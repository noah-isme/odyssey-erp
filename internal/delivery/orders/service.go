package orders

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// InventoryItem represents an item for inventory reduction.
type InventoryItem struct {
	WarehouseID int64
	ProductID   int64
	Quantity    float64
	UnitCost    float64
	Code        string
	Note        string
	ActorID     int64
	RefModule   string
	RefID       string
}

// InventoryClient provides inventory operations.
type InventoryClient interface {
	Reduce(ctx context.Context, items []InventoryItem) error
}

// Service provides business logic for delivery orders.
type Service struct {
	repo      Repository
	inventory InventoryClient
}

// NewService creates a new service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SetInventory sets the inventory client for stock operations.
func (s *Service) SetInventory(inv InventoryClient) {
	s.inventory = inv
}

// Create creates a new delivery order from a sales order.
func (s *Service) Create(ctx context.Context, req CreateRequest, createdBy int64) (*DeliveryOrder, error) {
	// Validate SO exists and is in correct status
	soDetails, err := s.repo.GetSalesOrderDetails(ctx, req.SalesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	if soDetails.Status != "CONFIRMED" && soDetails.Status != "PROCESSING" {
		return nil, fmt.Errorf("sales order must be CONFIRMED or PROCESSING, got: %s", soDetails.Status)
	}

	if soDetails.CompanyID != req.CompanyID {
		return nil, fmt.Errorf("sales order belongs to different company")
	}

	// Validate warehouse
	exists, err := s.repo.CheckWarehouseExists(ctx, req.WarehouseID)
	if err != nil {
		return nil, fmt.Errorf("check warehouse: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("warehouse not found")
	}

	// Validate lines against deliverable quantities
	deliverableLines, err := s.repo.GetDeliverableSOLines(ctx, req.SalesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get deliverable lines: %w", err)
	}

	if len(deliverableLines) == 0 {
		return nil, fmt.Errorf("no deliverable lines found")
	}

	deliverableMap := make(map[int64]*DeliverableSOLine)
	for i := range deliverableLines {
		deliverableMap[deliverableLines[i].SalesOrderLineID] = &deliverableLines[i]
	}

	for _, reqLine := range req.Lines {
		deliverable, exists := deliverableMap[reqLine.SalesOrderLineID]
		if !exists {
			return nil, fmt.Errorf("SO line %d not found or fully delivered", reqLine.SalesOrderLineID)
		}
		if reqLine.QuantityToDeliver > deliverable.RemainingQuantity {
			return nil, fmt.Errorf("qty %.2f exceeds remaining %.2f for line %d",
				reqLine.QuantityToDeliver, deliverable.RemainingQuantity, reqLine.SalesOrderLineID)
		}
		if reqLine.ProductID != deliverable.ProductID {
			return nil, fmt.Errorf("product ID mismatch for line %d", reqLine.SalesOrderLineID)
		}
	}

	// Generate document number
	docNumber, err := s.repo.GenerateDocNumber(ctx, req.CompanyID, req.DeliveryDate)
	if err != nil {
		return nil, fmt.Errorf("generate doc number: %w", err)
	}

	// Create in transaction
	var doID int64
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		do := DeliveryOrder{
			DocNumber:      docNumber,
			CompanyID:      req.CompanyID,
			SalesOrderID:   req.SalesOrderID,
			WarehouseID:    req.WarehouseID,
			CustomerID:     soDetails.CustomerID,
			DeliveryDate:   req.DeliveryDate,
			Status:         StatusDraft,
			DriverName:     req.DriverName,
			VehicleNumber:  req.VehicleNumber,
			TrackingNumber: req.TrackingNumber,
			Notes:          req.Notes,
			CreatedBy:      createdBy,
		}

		id, err := tx.CreateDeliveryOrder(ctx, do)
		if err != nil {
			return fmt.Errorf("create delivery order: %w", err)
		}
		doID = id

		for _, reqLine := range req.Lines {
			deliverable := deliverableMap[reqLine.SalesOrderLineID]
			line := Line{
				DeliveryOrderID:   doID,
				SalesOrderLineID:  reqLine.SalesOrderLineID,
				ProductID:         reqLine.ProductID,
				QuantityToDeliver: reqLine.QuantityToDeliver,
				QuantityDelivered: 0,
				UOM:               deliverable.UOM,
				UnitPrice:         deliverable.UnitPrice,
				Notes:             reqLine.Notes,
				LineOrder:         reqLine.LineOrder,
			}
			if _, err := tx.InsertLine(ctx, line); err != nil {
				return fmt.Errorf("insert line: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, doID)
}

// Update updates a DRAFT delivery order.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*DeliveryOrder, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if !existing.Status.CanEdit() {
		return nil, fmt.Errorf("%w: %s", ErrCannotEdit, existing.Status)
	}

	updates := make(map[string]interface{})
	if req.DeliveryDate != nil {
		updates["delivery_date"] = *req.DeliveryDate
	}
	if req.DriverName != nil {
		updates["driver_name"] = req.DriverName
	}
	if req.VehicleNumber != nil {
		updates["vehicle_number"] = req.VehicleNumber
	}
	if req.TrackingNumber != nil {
		updates["tracking_number"] = req.TrackingNumber
	}
	if req.Notes != nil {
		updates["notes"] = req.Notes
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if len(updates) > 0 {
			if err := tx.UpdateDeliveryOrder(ctx, id, updates); err != nil {
				return fmt.Errorf("update: %w", err)
			}
		}

		if req.Lines != nil {
			if err := tx.DeleteLines(ctx, id); err != nil {
				return fmt.Errorf("delete lines: %w", err)
			}

			deliverableLines, err := s.repo.GetDeliverableSOLines(ctx, existing.SalesOrderID)
			if err != nil {
				return fmt.Errorf("get deliverable lines: %w", err)
			}

			deliverableMap := make(map[int64]*DeliverableSOLine)
			for i := range deliverableLines {
				deliverableMap[deliverableLines[i].SalesOrderLineID] = &deliverableLines[i]
			}

			for _, reqLine := range *req.Lines {
				deliverable := deliverableMap[reqLine.SalesOrderLineID]
				line := Line{
					DeliveryOrderID:   id,
					SalesOrderLineID:  reqLine.SalesOrderLineID,
					ProductID:         reqLine.ProductID,
					QuantityToDeliver: reqLine.QuantityToDeliver,
					QuantityDelivered: 0,
					UOM:               deliverable.UOM,
					UnitPrice:         deliverable.UnitPrice,
					Notes:             reqLine.Notes,
					LineOrder:         reqLine.LineOrder,
				}
				if _, err := tx.InsertLine(ctx, line); err != nil {
					return fmt.Errorf("insert line: %w", err)
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

// Confirm confirms a delivery order.
func (s *Service) Confirm(ctx context.Context, id int64, confirmedBy int64) (*DeliveryOrder, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if !existing.Status.CanConfirm() {
		return nil, fmt.Errorf("%w: %s", ErrCannotConfirm, existing.Status)
	}

	if len(existing.Lines) == 0 {
		return nil, fmt.Errorf("cannot confirm without lines")
	}

	confirmedAt := time.Now()

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		updates := map[string]interface{}{
			"confirmed_by": confirmedBy,
			"confirmed_at": confirmedAt,
		}
		if err := tx.UpdateStatus(ctx, id, StatusConfirmed, updates); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

// MarkInTransit marks a delivery order as in transit.
func (s *Service) MarkInTransit(ctx context.Context, id int64, req MarkInTransitRequest) (*DeliveryOrder, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if existing.Status != StatusConfirmed {
		return nil, fmt.Errorf("must be CONFIRMED to mark in transit, got: %s", existing.Status)
	}

	updates := map[string]interface{}{}
	if req.TrackingNumber != nil {
		updates["tracking_number"] = req.TrackingNumber
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateStatus(ctx, id, StatusInTransit, updates)
	})

	if err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

// MarkDelivered marks a delivery order as delivered.
func (s *Service) MarkDelivered(ctx context.Context, id int64, req MarkDeliveredRequest) (*DeliveryOrder, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if existing.Status != StatusInTransit {
		return nil, fmt.Errorf("must be IN_TRANSIT to mark delivered, got: %s", existing.Status)
	}

	updates := map[string]interface{}{
		"delivered_at": req.DeliveredAt,
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if err := tx.UpdateStatus(ctx, id, StatusDelivered, updates); err != nil {
			return err
		}

		for _, line := range existing.Lines {
			if err := tx.UpdateLineQuantity(ctx, line.ID, line.QuantityToDeliver); err != nil {
				return fmt.Errorf("update line %d: %w", line.ID, err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Inventory reduction
	if s.inventory != nil {
		items := make([]InventoryItem, 0, len(existing.Lines))
		for _, line := range existing.Lines {
			items = append(items, InventoryItem{
				WarehouseID: existing.WarehouseID,
				ProductID:   line.ProductID,
				Quantity:    line.QuantityToDeliver,
				UnitCost:    line.UnitPrice,
				Code:        fmt.Sprintf("DO-%s-L%d", existing.DocNumber, line.ID),
				Note:        fmt.Sprintf("Delivery %s Line %d", existing.DocNumber, line.LineOrder),
				ActorID:     req.UpdatedBy,
				RefModule:   "DELIVERY",
				RefID:       fmt.Sprintf("%d", id),
			})
		}
		if err := s.inventory.Reduce(ctx, items); err != nil {
			return nil, fmt.Errorf("reduce inventory: %w", err)
		}
	}

	return s.repo.GetByID(ctx, id)
}

// Cancel cancels a delivery order.
func (s *Service) Cancel(ctx context.Context, id int64, req CancelRequest) (*DeliveryOrder, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if !existing.Status.CanCancel() {
		return nil, fmt.Errorf("%w: %s", ErrCannotCancel, existing.Status)
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if existing.Status == StatusConfirmed {
			for _, line := range existing.Lines {
				if err := tx.UpdateLineQuantity(ctx, line.ID, 0); err != nil {
					return fmt.Errorf("reset line %d: %w", line.ID, err)
				}
			}
		}

		updates := map[string]interface{}{
			"notes": req.Reason,
		}
		return tx.UpdateStatus(ctx, id, StatusCancelled, updates)
	})

	if err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

// GetByID retrieves a delivery order by ID.
func (s *Service) GetByID(ctx context.Context, id int64) (*DeliveryOrder, error) {
	do, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return do, nil
}

// GetWithDetails retrieves a delivery order with enriched details.
func (s *Service) GetWithDetails(ctx context.Context, id int64) (*WithDetails, error) {
	return s.repo.GetWithDetails(ctx, id)
}

// GetLinesWithDetails retrieves lines with product details.
func (s *Service) GetLinesWithDetails(ctx context.Context, doID int64) ([]LineWithDetails, error) {
	return s.repo.GetLinesWithDetails(ctx, doID)
}

// List returns a paginated list of delivery orders.
func (s *Service) List(ctx context.Context, req ListRequest) ([]WithDetails, int, error) {
	return s.repo.List(ctx, req)
}

// GetDeliverableSOLines retrieves SO lines that can still be delivered.
func (s *Service) GetDeliverableSOLines(ctx context.Context, salesOrderID int64) ([]DeliverableSOLine, error) {
	soDetails, err := s.repo.GetSalesOrderDetails(ctx, salesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	if soDetails.Status != "CONFIRMED" && soDetails.Status != "PROCESSING" {
		return nil, fmt.Errorf("SO must be CONFIRMED or PROCESSING, got: %s", soDetails.Status)
	}

	return s.repo.GetDeliverableSOLines(ctx, salesOrderID)
}
