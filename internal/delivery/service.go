package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InventoryService defines interface for inventory operations
type InventoryService interface {
	PostAdjustment(ctx context.Context, input InventoryAdjustmentInput) error
}

// InventoryAdjustmentInput represents inventory adjustment request
type InventoryAdjustmentInput struct {
	Code        string
	WarehouseID int64
	ProductID   int64
	Qty         float64
	UnitCost    float64
	Note        string
	ActorID     int64
	RefModule   string
	RefID       string
}

// Common errors
var (
	ErrDeliveryOrderNotFound = errors.New("delivery order not found")
	ErrCannotEdit            = errors.New("cannot edit delivery order in current status")
	ErrCannotConfirm         = errors.New("cannot confirm delivery order in current status")
	ErrCannotCancel          = errors.New("cannot cancel delivery order in current status")
)

// Service provides business logic for delivery operations.
type Service struct {
	repo      *Repository
	pool      *pgxpool.Pool
	inventory InventoryService
}

// NewService constructs a delivery service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		repo: NewRepository(pool),
		pool: pool,
	}
}

// SetInventoryService sets the inventory service for stock integration
func (s *Service) SetInventoryService(inv InventoryService) {
	s.inventory = inv
}

// ============================================================================
// DELIVERY ORDER OPERATIONS
// ============================================================================

// CreateDeliveryOrder creates a new delivery order from a sales order.
func (s *Service) CreateDeliveryOrder(ctx context.Context, req CreateDeliveryOrderRequest, createdBy int64) (*DeliveryOrder, error) {
	// Validate sales order exists and is in correct status
	soDetails, err := s.repo.GetSalesOrderDetails(ctx, req.SalesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	if soDetails.Status != "CONFIRMED" && soDetails.Status != "PROCESSING" {
		return nil, fmt.Errorf("sales order must be CONFIRMED or PROCESSING to create delivery order, got: %s", soDetails.Status)
	}

	// Validate company ID matches
	if soDetails.CompanyID != req.CompanyID {
		return nil, fmt.Errorf("sales order belongs to different company")
	}

	// Validate warehouse exists
	warehouseExists, err := s.repo.CheckWarehouseExists(ctx, req.WarehouseID)
	if err != nil {
		return nil, fmt.Errorf("check warehouse: %w", err)
	}
	if !warehouseExists {
		return nil, fmt.Errorf("warehouse not found")
	}

	// Get deliverable lines to validate requested quantities
	deliverableLines, err := s.repo.GetDeliverableSOLines(ctx, req.SalesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get deliverable lines: %w", err)
	}

	if len(deliverableLines) == 0 {
		return nil, fmt.Errorf("no deliverable lines found for sales order")
	}

	// Build map of deliverable quantities for validation
	deliverableMap := make(map[int64]*DeliverableSOLine)
	for i := range deliverableLines {
		deliverableMap[deliverableLines[i].SalesOrderLineID] = &deliverableLines[i]
	}

	// Validate each requested line
	for _, reqLine := range req.Lines {
		deliverable, exists := deliverableMap[reqLine.SalesOrderLineID]
		if !exists {
			return nil, fmt.Errorf("sales order line %d not found or fully delivered", reqLine.SalesOrderLineID)
		}

		if reqLine.QuantityToDeliver > deliverable.RemainingQuantity {
			return nil, fmt.Errorf("requested quantity %.2f exceeds remaining quantity %.2f for line %d",
				reqLine.QuantityToDeliver, deliverable.RemainingQuantity, reqLine.SalesOrderLineID)
		}

		if reqLine.QuantityToDeliver <= 0 {
			return nil, fmt.Errorf("quantity to deliver must be positive for line %d", reqLine.SalesOrderLineID)
		}

		// Validate product ID matches
		if reqLine.ProductID != deliverable.ProductID {
			return nil, fmt.Errorf("product ID mismatch for line %d", reqLine.SalesOrderLineID)
		}
	}

	// Generate document number
	docNumber, err := s.repo.GenerateDeliveryOrderNumber(ctx, req.CompanyID, req.DeliveryDate)
	if err != nil {
		return nil, fmt.Errorf("generate doc number: %w", err)
	}

	// Create delivery order in transaction
	var doID int64
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		do := DeliveryOrder{
			DocNumber:      docNumber,
			CompanyID:      req.CompanyID,
			SalesOrderID:   req.SalesOrderID,
			WarehouseID:    req.WarehouseID,
			CustomerID:     soDetails.CustomerID,
			DeliveryDate:   req.DeliveryDate,
			Status:         DOStatusDraft,
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

		// Insert lines
		for _, reqLine := range req.Lines {
			deliverable := deliverableMap[reqLine.SalesOrderLineID]
			line := DeliveryOrderLine{
				DeliveryOrderID:   doID,
				SalesOrderLineID:  reqLine.SalesOrderLineID,
				ProductID:         reqLine.ProductID,
				QuantityToDeliver: reqLine.QuantityToDeliver,
				QuantityDelivered: 0, // Will be set on confirmation
				UOM:               deliverable.UOM,
				UnitPrice:         deliverable.UnitPrice,
				Notes:             reqLine.Notes,
				LineOrder:         reqLine.LineOrder,
			}

			_, err := tx.InsertDeliveryOrderLine(ctx, line)
			if err != nil {
				return fmt.Errorf("insert line: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.repo.GetDeliveryOrder(ctx, doID)
}

// UpdateDeliveryOrder updates a DRAFT delivery order.
func (s *Service) UpdateDeliveryOrder(ctx context.Context, id int64, req UpdateDeliveryOrderRequest) (*DeliveryOrder, error) {
	// Get existing DO
	existing, err := s.repo.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	// Only DRAFT can be edited
	if !existing.Status.CanEdit() {
		return nil, fmt.Errorf("cannot edit delivery order in status: %s", existing.Status)
	}

	// Build updates
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

	// Update lines if provided
	if req.Lines != nil {
		// Validate lines similar to create
		deliverableLines, err := s.repo.GetDeliverableSOLines(ctx, existing.SalesOrderID)
		if err != nil {
			return nil, fmt.Errorf("get deliverable lines: %w", err)
		}

		deliverableMap := make(map[int64]*DeliverableSOLine)
		for i := range deliverableLines {
			deliverableMap[deliverableLines[i].SalesOrderLineID] = &deliverableLines[i]
		}

		for _, reqLine := range *req.Lines {
			deliverable, exists := deliverableMap[reqLine.SalesOrderLineID]
			if !exists {
				return nil, fmt.Errorf("sales order line %d not found or fully delivered", reqLine.SalesOrderLineID)
			}

			if reqLine.QuantityToDeliver > deliverable.RemainingQuantity {
				return nil, fmt.Errorf("requested quantity %.2f exceeds remaining quantity %.2f for line %d",
					reqLine.QuantityToDeliver, deliverable.RemainingQuantity, reqLine.SalesOrderLineID)
			}
		}
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		// Update header
		if len(updates) > 0 {
			if err := tx.UpdateDeliveryOrder(ctx, id, updates); err != nil {
				return fmt.Errorf("update delivery order: %w", err)
			}
		}

		// Update lines if provided
		if req.Lines != nil {
			// Delete existing lines
			if err := tx.DeleteDeliveryOrderLines(ctx, id); err != nil {
				return fmt.Errorf("delete lines: %w", err)
			}

			// Insert new lines
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
				line := DeliveryOrderLine{
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

				if _, err := tx.InsertDeliveryOrderLine(ctx, line); err != nil {
					return fmt.Errorf("insert line: %w", err)
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.repo.GetDeliveryOrder(ctx, id)
}

// ConfirmDeliveryOrder confirms a delivery order and reduces inventory.
func (s *Service) ConfirmDeliveryOrder(ctx context.Context, id int64, confirmedBy int64) (*DeliveryOrder, error) {
	// Get existing DO
	existing, err := s.repo.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	// Validate can confirm
	if !existing.Status.CanConfirm() {
		return nil, fmt.Errorf("cannot confirm delivery order in status: %s", existing.Status)
	}

	// Validate has lines
	if len(existing.Lines) == 0 {
		return nil, fmt.Errorf("cannot confirm delivery order without lines")
	}

	// TODO: Validate inventory availability for each line
	// This would call inventory service to check stock levels
	// For now, we'll assume inventory check is done

	confirmedAt := time.Now()

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		// Update status to CONFIRMED
		updates := map[string]interface{}{
			"confirmed_by": confirmedBy,
			"confirmed_at": confirmedAt,
		}

		if err := tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusConfirmed, updates); err != nil {
			return fmt.Errorf("update status: %w", err)
		}

		// Set quantity_delivered = quantity_to_deliver for all lines
		// This triggers the database triggers that update SO quantities
		for _, line := range existing.Lines {
			if err := tx.UpdateDeliveryOrderLineQuantity(ctx, line.ID, line.QuantityToDeliver); err != nil {
				return fmt.Errorf("update line quantity: %w", err)
			}
		}

		// TODO: Create inventory transactions for stock reduction
		// This would call inventory service to reduce stock
		// for _, line := range existing.Lines {
		//     inventoryReq := InventoryTransactionRequest{
		//         TransactionType: "SALES_OUT",
		//         CompanyID:       existing.CompanyID,
		//         WarehouseID:     existing.WarehouseID,
		//         ProductID:       line.ProductID,
		//         Quantity:        -line.QuantityToDeliver, // Negative for outbound
		//         ReferenceType:   "delivery_order",
		//         ReferenceID:     existing.ID,
		//         TransactionDate: confirmedAt,
		//         PostedBy:        confirmedBy,
		//     }
		//     if err := inventoryService.CreateTransaction(ctx, inventoryReq); err != nil {
		//         return fmt.Errorf("create inventory transaction: %w", err)
		//     }
		// }

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.repo.GetDeliveryOrder(ctx, id)
}

// MarkInTransit marks a delivery order as in transit.
func (s *Service) MarkInTransit(ctx context.Context, id int64, req MarkInTransitRequest) (*DeliveryOrder, error) {
	// Get existing DO
	existing, err := s.repo.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	// Must be CONFIRMED to mark in transit
	if existing.Status != DOStatusConfirmed {
		return nil, fmt.Errorf("delivery order must be CONFIRMED to mark in transit, got: %s", existing.Status)
	}

	updates := map[string]interface{}{}
	if req.TrackingNumber != nil {
		updates["tracking_number"] = req.TrackingNumber
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusInTransit, updates)
	})

	if err != nil {
		return nil, fmt.Errorf("mark in transit: %w", err)
	}

	return s.repo.GetDeliveryOrder(ctx, id)
}

// MarkDelivered marks a delivery order as delivered.
func (s *Service) MarkDelivered(ctx context.Context, id int64, req MarkDeliveredRequest) (*DeliveryOrder, error) {
	// Get existing DO
	existing, err := s.repo.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	// Must be IN_TRANSIT to mark delivered
	if existing.Status != DOStatusInTransit {
		return nil, fmt.Errorf("delivery order must be IN_TRANSIT to mark delivered, got: %s", existing.Status)
	}

	// Get delivery order lines for inventory reduction
	lines, err := s.repo.getDeliveryOrderLines(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order lines: %w", err)
	}

	updates := map[string]interface{}{
		"delivered_at": req.DeliveredAt,
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		// Update status to DELIVERED
		if err := tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusDelivered, updates); err != nil {
			return err
		}

		// Update lines to mark as delivered
		for _, line := range lines {
			if err := tx.UpdateDeliveryOrderLineQuantity(ctx, line.ID, line.QuantityToDeliver); err != nil {
				return fmt.Errorf("update line %d: %w", line.ID, err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("mark delivered: %w", err)
	}

	// Reduce inventory stock if inventory service is available
	if s.inventory != nil {
		for _, line := range lines {
			adjustmentInput := InventoryAdjustmentInput{
				Code:        fmt.Sprintf("DO-%s-L%d", existing.DocNumber, line.ID),
				WarehouseID: existing.WarehouseID,
				ProductID:   line.ProductID,
				Qty:         -line.QuantityToDeliver, // Negative for outbound
				UnitCost:    line.UnitPrice,
				Note:        fmt.Sprintf("Delivery Order %s - Line %d", existing.DocNumber, line.LineOrder),
				ActorID:     req.UpdatedBy,
				RefModule:   "DELIVERY",
				RefID:       fmt.Sprintf("%d", id),
			}

			if err := s.inventory.PostAdjustment(ctx, adjustmentInput); err != nil {
				return nil, fmt.Errorf("reduce stock for product %d: %w", line.ProductID, err)
			}
		}
	}

	return s.repo.GetDeliveryOrder(ctx, id)
}

// CancelDeliveryOrder cancels a delivery order.
func (s *Service) CancelDeliveryOrder(ctx context.Context, id int64, req CancelDeliveryOrderRequest) (*DeliveryOrder, error) {
	// Get existing DO
	existing, err := s.repo.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	// Validate can cancel
	if !existing.Status.CanCancel() {
		return nil, fmt.Errorf("cannot cancel delivery order in status: %s", existing.Status)
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		// If was CONFIRMED, need to reverse inventory transactions
		if existing.Status == DOStatusConfirmed {
			// TODO: Reverse inventory transactions
			// This would restore the stock that was reduced
			// for _, line := range existing.Lines {
			//     inventoryReq := InventoryTransactionRequest{
			//         TransactionType: "SALES_RETURN",
			//         CompanyID:       existing.CompanyID,
			//         WarehouseID:     existing.WarehouseID,
			//         ProductID:       line.ProductID,
			//         Quantity:        line.QuantityDelivered, // Positive for inbound
			//         ReferenceType:   "delivery_order_cancel",
			//         ReferenceID:     existing.ID,
			//         TransactionDate: time.Now(),
			//         PostedBy:        req.CancelledBy,
			//         Notes:           req.Reason,
			//     }
			//     if err := inventoryService.CreateTransaction(ctx, inventoryReq); err != nil {
			//         return fmt.Errorf("reverse inventory: %w", err)
			//     }
			// }

			// Reset line quantities to 0 (triggers SO quantity recalculation)
			for _, line := range existing.Lines {
				if err := tx.UpdateDeliveryOrderLineQuantity(ctx, line.ID, 0); err != nil {
					return fmt.Errorf("reset line quantity: %w", err)
				}
			}
		}

		// Update status to CANCELLED
		updates := map[string]interface{}{
			"notes": req.Reason, // Store cancellation reason in notes
		}

		return tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusCancelled, updates)
	})

	if err != nil {
		return nil, fmt.Errorf("cancel delivery order: %w", err)
	}

	return s.repo.GetDeliveryOrder(ctx, id)
}

// ============================================================================
// QUERY OPERATIONS
// ============================================================================

// GetDeliveryOrder retrieves a delivery order by ID.
func (s *Service) GetDeliveryOrder(ctx context.Context, id int64) (*DeliveryOrder, error) {
	do, err := s.repo.GetDeliveryOrder(ctx, id)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, ErrDeliveryOrderNotFound
		}
		return nil, err
	}
	return do, nil
}

// GetDeliveryOrderByDocNumber retrieves a delivery order by document number.
func (s *Service) GetDeliveryOrderByDocNumber(ctx context.Context, companyID int64, docNumber string) (*DeliveryOrder, error) {
	return s.repo.GetDeliveryOrderByDocNumber(ctx, companyID, docNumber)
}

// GetDeliveryOrderWithDetails retrieves a delivery order with enriched details.
func (s *Service) GetDeliveryOrderWithDetails(ctx context.Context, id int64) (*DeliveryOrderWithDetails, error) {
	return s.repo.GetDeliveryOrderWithDetails(ctx, id)
}

// GetDeliveryOrderLinesWithDetails retrieves lines with product details.
func (s *Service) GetDeliveryOrderLinesWithDetails(ctx context.Context, deliveryOrderID int64) ([]DeliveryOrderLineWithDetails, error) {
	return s.repo.GetDeliveryOrderLinesWithDetails(ctx, deliveryOrderID)
}

// ListDeliveryOrders returns a paginated list of delivery orders with filters.
func (s *Service) ListDeliveryOrders(ctx context.Context, req ListDeliveryOrdersRequest) ([]DeliveryOrderWithDetails, int, error) {
	return s.repo.ListDeliveryOrders(ctx, req)
}

// GetDeliverableSOLines retrieves sales order lines that can be delivered.
func (s *Service) GetDeliverableSOLines(ctx context.Context, salesOrderID int64) ([]DeliverableSOLine, error) {
	// Validate sales order exists
	soDetails, err := s.repo.GetSalesOrderDetails(ctx, salesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	// Only CONFIRMED or PROCESSING orders can be delivered
	if soDetails.Status != "CONFIRMED" && soDetails.Status != "PROCESSING" {
		return nil, fmt.Errorf("sales order must be CONFIRMED or PROCESSING to deliver, got: %s", soDetails.Status)
	}

	return s.repo.GetDeliverableSOLines(ctx, salesOrderID)
}

// ============================================================================
// VALIDATION HELPERS
// ============================================================================

// ValidateStockAvailability checks if sufficient stock is available for delivery.
// TODO: Implement when inventory module is available
func (s *Service) ValidateStockAvailability(ctx context.Context, warehouseID int64, lines []CreateDeliveryOrderLineReq) error {
	// This would call inventory service to validate stock
	// for _, line := range lines {
	//     req := StockValidationRequest{
	//         WarehouseID: warehouseID,
	//         ProductID:   line.ProductID,
	//         Quantity:    line.QuantityToDeliver,
	//     }
	//     result, err := inventoryService.ValidateStock(ctx, req)
	//     if err != nil {
	//         return fmt.Errorf("validate stock for product %d: %w", line.ProductID, err)
	//     }
	//     if !result.Available {
	//         return fmt.Errorf("insufficient stock for product %d: need %.2f, have %.2f",
	//             line.ProductID, result.RequestedQty, result.CurrentStock)
	//     }
	// }
	return nil
}
