package distribution

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// Service handles planning and distribution lifecycle transitions.
type Service struct {
	repo      Repository
	shipments ShipmentGateway
	inventory InventoryGateway
}

func NewService(db *pgxpool.Pool) *Service {
	return NewServiceWithDependencies(NewRepository(db), Dependencies{})
}

func NewServiceWithDependencies(repo Repository, dependencies Dependencies) *Service {
	return &Service{
		repo:      repo,
		shipments: dependencies.Shipments,
		inventory: dependencies.Inventory,
	}
}

func (s *Service) SetupPlanningHorizon(ctx context.Context, input CreatePlanningHorizonInput) (*PlanningHorizon, error) {
	if input.WarehouseID == 0 {
		return nil, fmt.Errorf("warehouse ID is required")
	}
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	if input.PlanningStartDate.IsZero() || input.PlanningEndDate.IsZero() {
		return nil, fmt.Errorf("planning start and end dates are required")
	}
	if input.PlanningEndDate.Before(input.PlanningStartDate) {
		return nil, fmt.Errorf("planning end date cannot be before start date")
	}

	horizonID, err := s.repo.CreatePlanningHorizon(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create planning horizon: %w", err)
	}
	return s.repo.GetPlanningHorizon(ctx, horizonID)
}

func (s *Service) ListPlanningHorizons(ctx context.Context, companyID int64) ([]*PlanningHorizon, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	return s.repo.ListPlanningHorizons(ctx, companyID)
}

func (s *Service) AddPlanningRule(ctx context.Context, input CreatePlanningRuleInput) (*PlanningRule, error) {
	if strings.TrimSpace(input.RuleName) == "" {
		return nil, fmt.Errorf("rule name is required")
	}
	if !validRuleType(input.RuleType) {
		return nil, fmt.Errorf("invalid rule type")
	}
	if input.WarehouseID == 0 {
		return nil, fmt.Errorf("warehouse ID is required")
	}
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	if input.MaxItemsPerLoad != nil && *input.MaxItemsPerLoad <= 0 {
		return nil, fmt.Errorf("max items per load must be positive")
	}

	ruleID, err := s.repo.CreatePlanningRule(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create planning rule: %w", err)
	}
	return s.repo.GetPlanningRule(ctx, ruleID)
}

func (s *Service) CreateLoad(ctx context.Context, input CreateLoadInput) (*Load, error) {
	if input.OriginWarehouseID == 0 {
		return nil, fmt.Errorf("origin warehouse is required")
	}
	if input.DestinationWarehouseID == nil && strings.TrimSpace(input.DestinationCity) == "" {
		return nil, fmt.Errorf("destination warehouse or city is required")
	}
	if input.DestinationWarehouseID != nil && *input.DestinationWarehouseID == input.OriginWarehouseID {
		return nil, fmt.Errorf("origin and destination warehouses cannot be the same")
	}
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	if input.PlannedPickupDate != nil && input.PlannedDeliveryDate != nil && input.PlannedDeliveryDate.Before(*input.PlannedPickupDate) {
		return nil, fmt.Errorf("planned delivery date cannot be before pickup date")
	}

	loadID, err := s.repo.CreateLoad(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create load: %w", err)
	}
	return s.repo.GetLoad(ctx, loadID)
}

func (s *Service) ListLoads(ctx context.Context, companyID int64, status *LoadStatus) ([]*Load, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	return s.repo.ListLoads(ctx, companyID, status)
}

func (s *Service) GetLoad(ctx context.Context, loadID int64) (*Load, []*LoadItem, error) {
	if loadID == 0 {
		return nil, nil, fmt.Errorf("load ID is required")
	}
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return nil, nil, fmt.Errorf("load not found: %w", err)
	}
	items, err := s.repo.GetLoadItems(ctx, loadID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load items: %w", err)
	}
	return load, items, nil
}

// AddShipmentToLoad records the relationship between an existing shipment
// line and a load. New flows should use CreateShipmentForLoad, which creates
// both the logistics line and the distribution relationship together.
func (s *Service) AddShipmentToLoad(ctx context.Context, loadID, shipmentID, productID int64, quantity accountingmoney.Money, weightKg, volumeCbm *accountingmoney.Money) error {
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return fmt.Errorf("load not found: %w", err)
	}
	if load.Status != LoadStatusDraft && load.Status != LoadStatusPlanned {
		return fmt.Errorf("can only add items to DRAFT or PLANNED loads, current status: %s", load.Status)
	}
	if shipmentID == 0 || productID == 0 {
		return fmt.Errorf("shipment and product IDs are required")
	}
	if moneyIsNonPositive(quantity) {
		return fmt.Errorf("quantity must be positive")
	}
	_, err = s.repo.AddLoadItem(ctx, AddLoadItemInput{
		CompanyID:  load.CompanyID,
		LoadID:     loadID,
		ShipmentID: &shipmentID,
		ProductID:  productID,
		Quantity:   quantity,
		WeightKg:   weightKg,
		VolumeCbm:  volumeCbm,
	})
	if err != nil {
		return fmt.Errorf("failed to add shipment to load: %w", err)
	}
	return nil
}

// CreateShipmentForLoad is the first complete distribution boundary: it
// creates a logistics shipment, its lines, and the load-item links.
func (s *Service) CreateShipmentForLoad(ctx context.Context, loadID int64, input ShipmentCreateInput, lines []ShipmentLineInput) (int64, error) {
	if s.shipments == nil {
		return 0, fmt.Errorf("distribution shipment gateway is not configured")
	}
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return 0, fmt.Errorf("load not found: %w", err)
	}
	if load.Status != LoadStatusDraft && load.Status != LoadStatusPlanned && load.Status != LoadStatusReady {
		return 0, fmt.Errorf("can only add shipments to DRAFT, PLANNED, or READY loads, current status: %s", load.Status)
	}
	if len(lines) == 0 {
		return 0, fmt.Errorf("at least one shipment line is required")
	}
	for _, line := range lines {
		if line.ProductID == 0 || line.Quantity <= 0 {
			return 0, fmt.Errorf("shipment product and positive quantity are required")
		}
	}
	if input.CompanyID == 0 {
		input.CompanyID = load.CompanyID
	}
	if input.CompanyID != load.CompanyID {
		return 0, fmt.Errorf("shipment company does not match load company")
	}
	if input.ShipmentNumber == "" {
		input.ShipmentNumber = fmt.Sprintf("SHIP-LOAD-%d-%d", loadID, time.Now().UTC().UnixNano())
	}
	if input.ShipmentType == "" {
		input.ShipmentType = "DELIVERY"
	}
	if input.OriginWarehouseID == nil {
		origin := load.OriginWarehouseID
		input.OriginWarehouseID = &origin
	}
	if input.DestinationWarehouseID == nil {
		input.DestinationWarehouseID = load.DestinationWarehouseID
	}
	if input.DestinationAddress == "" {
		input.DestinationAddress = load.DestinationAddress
	}
	if input.DestinationCity == "" {
		input.DestinationCity = load.DestinationCity
	}
	if input.DestinationCountry == "" {
		input.DestinationCountry = load.DestinationCountry
	}
	if input.CreatedBy == 0 {
		input.CreatedBy = load.CreatedBy
	}
	if input.DestinationWarehouseID == nil && input.DestinationCity == "" {
		return 0, fmt.Errorf("shipment destination warehouse or city is required")
	}

	shipmentID, err := s.shipments.CreateShipment(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to create shipment: %w", err)
	}
	for _, line := range lines {
		line.CompanyID = load.CompanyID
		line.ShipmentID = shipmentID
		if err := s.shipments.AddShipmentLine(ctx, line); err != nil {
			return 0, fmt.Errorf("failed to add shipment line: %w", err)
		}
		quantity := moneyFromFloat(line.Quantity, 4)
		weight := optionalMoneyFromFloat(line.WeightKg, 4)
		volume := optionalMoneyFromFloat(line.VolumeCbm, 4)
		if _, err := s.repo.AddLoadItem(ctx, AddLoadItemInput{
			CompanyID:  load.CompanyID,
			LoadID:     loadID,
			ShipmentID: &shipmentID,
			ProductID:  line.ProductID,
			Quantity:   quantity,
			WeightKg:   weight,
			VolumeCbm:  volume,
		}); err != nil {
			return 0, fmt.Errorf("failed to link shipment line to load: %w", err)
		}
	}
	return shipmentID, nil
}

func (s *Service) MarkLoadReady(ctx context.Context, loadID int64) ([]string, error) {
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return nil, fmt.Errorf("load not found: %w", err)
	}
	if load.Status != LoadStatusDraft && load.Status != LoadStatusPlanned {
		return nil, fmt.Errorf("can only ready DRAFT or PLANNED loads, current status: %s", load.Status)
	}
	items, err := s.repo.GetLoadItems(ctx, loadID)
	if err != nil {
		return nil, fmt.Errorf("failed to load items: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("a load must contain at least one shipment line")
	}
	valid, violations, err := s.ValidateLoadAgainstRules(ctx, loadID, load.OriginWarehouseID)
	if err != nil {
		return nil, err
	}
	if !valid {
		return violations, fmt.Errorf("load violates planning rules")
	}
	if err := s.repo.UpdateLoadStatus(ctx, loadID, LoadStatusReady); err != nil {
		return nil, fmt.Errorf("failed to ready load: %w", err)
	}
	return violations, nil
}

func (s *Service) ValidateLoadAgainstRules(ctx context.Context, loadID, warehouseID int64) (bool, []string, error) {
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return false, nil, fmt.Errorf("load not found: %w", err)
	}
	if warehouseID == 0 {
		warehouseID = load.OriginWarehouseID
	}
	items, err := s.repo.GetLoadItems(ctx, loadID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to load items: %w", err)
	}
	rules, err := s.repo.ListPlanningRules(ctx, warehouseID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to load planning rules: %w", err)
	}

	var quantity, weight, volume float64
	for _, item := range items {
		quantity += moneyFloat(item.Quantity)
		if item.WeightKg != nil {
			weight += moneyFloat(*item.WeightKg)
		}
		if item.VolumeCbm != nil {
			volume += moneyFloat(*item.VolumeCbm)
		}
	}
	violations := make([]string, 0)
	for _, rule := range rules {
		switch rule.RuleType {
		case RuleTypeCapacity:
			if rule.MaxItemsPerLoad != nil && quantity > float64(*rule.MaxItemsPerLoad) {
				violations = append(violations, fmt.Sprintf("%s: item quantity %.4g exceeds maximum %d", rule.RuleName, quantity, *rule.MaxItemsPerLoad))
			}
			if rule.MaxLoadWeightKg != nil && weight > moneyFloat(*rule.MaxLoadWeightKg) {
				violations = append(violations, fmt.Sprintf("%s: weight %.4g exceeds maximum %s", rule.RuleName, weight, rule.MaxLoadWeightKg.String()))
			}
			if rule.MaxLoadVolumeCbm != nil && volume > moneyFloat(*rule.MaxLoadVolumeCbm) {
				violations = append(violations, fmt.Sprintf("%s: volume %.4g exceeds maximum %s", rule.RuleName, volume, rule.MaxLoadVolumeCbm.String()))
			}
		case RuleTypeWeight:
			if rule.MaxLoadWeightKg != nil && weight > moneyFloat(*rule.MaxLoadWeightKg) {
				violations = append(violations, fmt.Sprintf("%s: weight %.4g exceeds maximum %s", rule.RuleName, weight, rule.MaxLoadWeightKg.String()))
			}
		case RuleTypeTimeWindow:
			if load.PlannedPickupDate == nil || rule.TimeWindowStart == nil || rule.TimeWindowEnd == nil {
				violations = append(violations, fmt.Sprintf("%s: pickup date and complete time window are required", rule.RuleName))
				continue
			}
			pickup := load.PlannedPickupDate.Hour()*60 + load.PlannedPickupDate.Minute()
			start := rule.TimeWindowStart.Hour()*60 + rule.TimeWindowStart.Minute()
			end := rule.TimeWindowEnd.Hour()*60 + rule.TimeWindowEnd.Minute()
			if pickup < start || pickup > end {
				violations = append(violations, fmt.Sprintf("%s: pickup time is outside the planning window", rule.RuleName))
			}
		case RuleTypeVehicleType:
			violations = append(violations, fmt.Sprintf("%s: vehicle type validation is required at dispatch", rule.RuleName))
		case RuleTypeCustom:
			violations = append(violations, fmt.Sprintf("%s: custom rule expressions are not supported yet", rule.RuleName))
		}
	}
	return len(violations) == 0, violations, nil
}

func (s *Service) DispatchLoad(ctx context.Context, loadID int64, vehicleID, driverID, carrierID *int64, carrierService *string) error {
	if s.shipments == nil {
		return fmt.Errorf("distribution shipment gateway is not configured")
	}
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return fmt.Errorf("load not found: %w", err)
	}
	if load.Status != LoadStatusReady {
		return fmt.Errorf("can only dispatch READY loads, current status: %s", load.Status)
	}
	hasOwnTransport := vehicleID != nil && driverID != nil
	hasCarrierService := carrierID != nil && carrierService != nil && strings.TrimSpace(*carrierService) != ""
	if !hasOwnTransport && !hasCarrierService {
		return fmt.Errorf("must assign either vehicle+driver or carrier+service")
	}
	if hasOwnTransport && hasCarrierService {
		return fmt.Errorf("cannot assign both vehicle+driver and carrier+service")
	}

	items, err := s.repo.GetLoadItems(ctx, loadID)
	if err != nil {
		return fmt.Errorf("failed to load shipment lines: %w", err)
	}
	shipmentIDs := uniqueShipmentIDs(items)
	if len(shipmentIDs) == 0 {
		return fmt.Errorf("load has no linked shipments")
	}
	if err := s.repo.UpdateLoadDispatch(ctx, loadID, vehicleID, driverID, carrierID, carrierService); err != nil {
		return fmt.Errorf("failed to assign load transport: %w", err)
	}
	if err := s.repo.UpdateLoadStatus(ctx, loadID, LoadStatusDispatched); err != nil {
		return fmt.Errorf("failed to mark load dispatched: %w", err)
	}
	for _, shipmentID := range shipmentIDs {
		if err := s.shipments.DispatchShipment(ctx, shipmentID, vehicleID, driverID, carrierID, carrierService); err != nil {
			return fmt.Errorf("failed to dispatch shipment %d: %w", shipmentID, err)
		}
		if err := s.shipments.MarkShipmentInTransit(ctx, shipmentID); err != nil {
			return fmt.Errorf("failed to start shipment %d: %w", shipmentID, err)
		}
	}
	if err := s.repo.UpdateLoadStatus(ctx, loadID, LoadStatusInTransit); err != nil {
		return fmt.Errorf("failed to mark load in transit: %w", err)
	}
	return nil
}

// DeliverLoad posts the physical inventory movement before closing each
// shipment. Inventory's normal integration hook then creates the GL entry.
func (s *Service) DeliverLoad(ctx context.Context, loadID, actorID int64, deliveredAt time.Time) error {
	if s.shipments == nil {
		return fmt.Errorf("distribution shipment gateway is not configured")
	}
	if s.inventory == nil {
		return fmt.Errorf("distribution inventory gateway is not configured")
	}
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return fmt.Errorf("load not found: %w", err)
	}
	if load.Status != LoadStatusInTransit {
		return fmt.Errorf("can only deliver IN_TRANSIT loads, current status: %s", load.Status)
	}
	if deliveredAt.IsZero() {
		deliveredAt = time.Now().UTC()
	}
	items, err := s.repo.GetLoadItems(ctx, loadID)
	if err != nil {
		return fmt.Errorf("failed to load shipment lines: %w", err)
	}
	shipmentIDs := uniqueShipmentIDs(items)
	if len(shipmentIDs) == 0 {
		return fmt.Errorf("load has no linked shipments")
	}
	for _, item := range items {
		if item.ShipmentID == nil {
			return fmt.Errorf("load item %d has no shipment", item.ID)
		}
		if err := s.inventory.PostAdjustment(ctx, InventoryAdjustmentInput{
			Code:        fmt.Sprintf("DIST-LOAD-%d-ITEM-%d", loadID, item.ID),
			WarehouseID: load.OriginWarehouseID,
			ProductID:   item.ProductID,
			Quantity:    -moneyFloat(item.Quantity),
			Note:        fmt.Sprintf("Distribution load %s delivered", load.LoadNumber),
			ActorID:     actorID,
			RefModule:   "DISTRIBUTION",
			RefID:       strconv.FormatInt(loadID, 10),
		}); err != nil {
			return fmt.Errorf("failed to post inventory for load item %d: %w", item.ID, err)
		}
	}
	for _, shipmentID := range shipmentIDs {
		if err := s.shipments.MarkShipmentDelivered(ctx, shipmentID, deliveredAt); err != nil {
			return fmt.Errorf("failed to deliver shipment %d: %w", shipmentID, err)
		}
	}
	if err := s.repo.UpdateLoadStatus(ctx, loadID, LoadStatusDelivered); err != nil {
		return fmt.Errorf("failed to mark load delivered: %w", err)
	}
	return nil
}

func (s *Service) PlanDeliveryRoute(ctx context.Context, input CreateRouteInput) (*DeliveryRoute, error) {
	if input.LoadID == 0 {
		return nil, fmt.Errorf("load ID is required")
	}
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	load, err := s.repo.GetLoad(ctx, input.LoadID)
	if err != nil {
		return nil, fmt.Errorf("load not found: %w", err)
	}
	if load.Status != LoadStatusReady && load.Status != LoadStatusPlanned {
		return nil, fmt.Errorf("can only create routes for PLANNED or READY loads, current status: %s", load.Status)
	}
	if input.CompanyID != load.CompanyID {
		return nil, fmt.Errorf("route company does not match load company")
	}
	routeID, err := s.repo.CreateRoute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create route: %w", err)
	}
	return s.repo.GetRoute(ctx, routeID)
}

func (s *Service) AddDeliveryStop(ctx context.Context, input AddRouteStopInput) (*RouteStop, error) {
	route, err := s.repo.GetRoute(ctx, input.RouteID)
	if err != nil {
		return nil, fmt.Errorf("route not found: %w", err)
	}
	if route.Status != RouteStatusDraft && route.Status != RouteStatusOptimized {
		return nil, fmt.Errorf("can only add stops to DRAFT or OPTIMIZED routes, current status: %s", route.Status)
	}
	if input.CompanyID != route.CompanyID {
		return nil, fmt.Errorf("stop company does not match route company")
	}
	if input.StopSequence <= 0 {
		return nil, fmt.Errorf("stop sequence must be positive")
	}
	if input.StopType != StopTypeWarehouse && input.StopType != StopTypeCustomer && input.StopType != StopTypeDeliveryPoint {
		return nil, fmt.Errorf("invalid stop type")
	}
	existingStops, err := s.repo.GetRouteStops(ctx, input.RouteID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch route stops: %w", err)
	}
	if input.StopSequence > len(existingStops)+1 {
		return nil, fmt.Errorf("stop sequence must not skip a sequence number")
	}
	stopID, err := s.repo.AddRouteStop(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to add route stop: %w", err)
	}
	return s.repo.GetRouteStop(ctx, stopID)
}

func (s *Service) OptimizeRoute(ctx context.Context, routeID int64) error {
	route, err := s.repo.GetRoute(ctx, routeID)
	if err != nil {
		return fmt.Errorf("route not found: %w", err)
	}
	if route.Status != RouteStatusDraft {
		return fmt.Errorf("can only optimize DRAFT routes, current status: %s", route.Status)
	}
	stops, err := s.repo.GetRouteStops(ctx, routeID)
	if err != nil {
		return fmt.Errorf("failed to load route stops: %w", err)
	}
	if len(stops) == 0 {
		return fmt.Errorf("cannot optimize a route without stops")
	}
	if err := s.repo.UpdateRouteStatus(ctx, routeID, RouteStatusOptimized); err != nil {
		return fmt.Errorf("failed to update route status: %w", err)
	}
	return nil
}

func (s *Service) ApproveRoute(ctx context.Context, routeID int64) error {
	route, err := s.repo.GetRoute(ctx, routeID)
	if err != nil {
		return fmt.Errorf("route not found: %w", err)
	}
	if route.Status != RouteStatusOptimized {
		return fmt.Errorf("can only approve OPTIMIZED routes, current status: %s", route.Status)
	}
	if err := s.repo.UpdateRouteStatus(ctx, routeID, RouteStatusApproved); err != nil {
		return fmt.Errorf("failed to approve route: %w", err)
	}
	return nil
}

func (s *Service) CreateTransferOrder(ctx context.Context, input CreateTransferOrderInput) (*TransferOrder, error) {
	if input.FromWarehouseID == 0 || input.ToWarehouseID == 0 {
		return nil, fmt.Errorf("from and to warehouses are required")
	}
	if input.FromWarehouseID == input.ToWarehouseID {
		return nil, fmt.Errorf("from and to warehouses cannot be the same")
	}
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	if input.PlannedDispatchDate != nil && input.PlannedArrivalDate != nil && input.PlannedArrivalDate.Before(*input.PlannedDispatchDate) {
		return nil, fmt.Errorf("planned arrival date cannot be before dispatch date")
	}
	transferID, err := s.repo.CreateTransferOrder(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create transfer order: %w", err)
	}
	return s.repo.GetTransferOrder(ctx, transferID)
}

func (s *Service) ListTransfers(ctx context.Context, companyID int64, status *TransferStatus) ([]*TransferOrder, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	return s.repo.ListTransferOrders(ctx, companyID, status)
}

func (s *Service) GetTransfer(ctx context.Context, transferID int64) (*TransferOrder, []*TransferOrderLine, error) {
	if transferID == 0 {
		return nil, nil, fmt.Errorf("transfer ID is required")
	}
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return nil, nil, fmt.Errorf("transfer order not found: %w", err)
	}
	lines, err := s.repo.GetTransferLines(ctx, transferID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load transfer lines: %w", err)
	}
	return transfer, lines, nil
}

func (s *Service) AddTransferItem(ctx context.Context, transferID, productID int64, quantity accountingmoney.Money, lotNumber string, serialNumbers []string) error {
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return fmt.Errorf("transfer order not found: %w", err)
	}
	if transfer.Status != TransferStatusDraft {
		return fmt.Errorf("can only add items to DRAFT transfers, current status: %s", transfer.Status)
	}
	if productID == 0 || moneyIsNonPositive(quantity) {
		return fmt.Errorf("product and positive quantity are required")
	}
	_, err = s.repo.AddTransferLine(ctx, AddTransferLineInput{
		CompanyID:         transfer.CompanyID,
		TransferOrderID:   transferID,
		ProductID:         productID,
		QuantityRequested: quantity,
		LotNumber:         lotNumber,
		SerialNumbers:     serialNumbers,
	})
	if err != nil {
		return fmt.Errorf("failed to add transfer item: %w", err)
	}
	return nil
}

func (s *Service) ApproveTransfer(ctx context.Context, transferID int64) error {
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return fmt.Errorf("transfer order not found: %w", err)
	}
	if transfer.Status != TransferStatusDraft {
		return fmt.Errorf("can only approve DRAFT transfers, current status: %s", transfer.Status)
	}
	lines, err := s.repo.GetTransferLines(ctx, transferID)
	if err != nil {
		return fmt.Errorf("failed to load transfer lines: %w", err)
	}
	if len(lines) == 0 {
		return fmt.Errorf("a transfer must contain at least one line")
	}
	for _, line := range lines {
		if moneyIsNonPositive(line.QuantityRequested) {
			return fmt.Errorf("transfer line %d must have a positive quantity", line.ID)
		}
	}
	if err := s.repo.UpdateTransferStatus(ctx, transferID, TransferStatusApproved); err != nil {
		return fmt.Errorf("failed to approve transfer: %w", err)
	}
	return nil
}

func (s *Service) DispatchTransfer(ctx context.Context, transferID int64, vehicleID, driverID, carrierID *int64) error {
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return fmt.Errorf("transfer order not found: %w", err)
	}
	if transfer.Status != TransferStatusApproved {
		return fmt.Errorf("can only dispatch APPROVED transfers, current status: %s", transfer.Status)
	}
	hasOwnTransport := vehicleID != nil && driverID != nil
	hasCarrier := carrierID != nil
	if !hasOwnTransport && !hasCarrier {
		return fmt.Errorf("must assign either vehicle+driver or carrier")
	}
	if hasOwnTransport && hasCarrier {
		return fmt.Errorf("cannot assign both vehicle+driver and carrier")
	}
	if err := s.repo.UpdateTransferDispatch(ctx, transferID, vehicleID, driverID, carrierID); err != nil {
		return fmt.Errorf("failed to dispatch transfer: %w", err)
	}
	if err := s.repo.UpdateTransferStatus(ctx, transferID, TransferStatusDispatched); err != nil {
		return fmt.Errorf("failed to mark transfer dispatched: %w", err)
	}
	if err := s.repo.UpdateTransferStatus(ctx, transferID, TransferStatusInTransit); err != nil {
		return fmt.Errorf("failed to mark transfer in transit: %w", err)
	}
	return nil
}

func (s *Service) ReceiveTransfer(ctx context.Context, transferID int64, receivedAt time.Time) error {
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return fmt.Errorf("transfer order not found: %w", err)
	}
	if transfer.Status != TransferStatusInTransit {
		return fmt.Errorf("can only receive IN_TRANSIT transfers, current status: %s", transfer.Status)
	}
	lines, err := s.repo.GetTransferLines(ctx, transferID)
	if err != nil {
		return fmt.Errorf("failed to load transfer lines: %w", err)
	}
	for _, line := range lines {
		if err := s.repo.UpdateTransferLineReceipt(ctx, line.ID, line.QuantityRequested); err != nil {
			return fmt.Errorf("failed to receive transfer line %d: %w", line.ID, err)
		}
	}
	if err := s.repo.UpdateTransferStatus(ctx, transferID, TransferStatusReceived); err != nil {
		return fmt.Errorf("failed to mark transfer received: %w", err)
	}
	_ = receivedAt // the repository records the receipt transition timestamp.
	return nil
}

func (s *Service) GetLoadUtilization(ctx context.Context, loadID int64) (*LoadUtilization, error) {
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return nil, fmt.Errorf("load not found: %w", err)
	}
	items, err := s.repo.GetLoadItems(ctx, loadID)
	if err != nil {
		return nil, fmt.Errorf("failed to load items: %w", err)
	}
	rules, err := s.repo.ListPlanningRules(ctx, load.OriginWarehouseID)
	if err != nil {
		return nil, fmt.Errorf("failed to load planning rules: %w", err)
	}
	var weight, volume, quantity float64
	for _, item := range items {
		quantity += moneyFloat(item.Quantity)
		if item.WeightKg != nil {
			weight += moneyFloat(*item.WeightKg)
		}
		if item.VolumeCbm != nil {
			volume += moneyFloat(*item.VolumeCbm)
		}
	}
	result := &LoadUtilization{
		LoadID:        loadID,
		CurrentWeight: optionalMoneyFromFloat(&weight, 4),
		CurrentVolume: optionalMoneyFromFloat(&volume, 4),
		CurrentItems:  intFromFloat(quantity),
	}
	for _, rule := range rules {
		if result.MaxWeight == nil && rule.MaxLoadWeightKg != nil {
			result.MaxWeight = rule.MaxLoadWeightKg
		}
		if result.MaxVolume == nil && rule.MaxLoadVolumeCbm != nil {
			result.MaxVolume = rule.MaxLoadVolumeCbm
		}
		if result.MaxItems == nil && rule.MaxItemsPerLoad != nil {
			result.MaxItems = rule.MaxItemsPerLoad
		}
	}
	result.WeightUtilization = percentage(weight, moneyFloatPtr(result.MaxWeight))
	result.VolumeUtilization = percentage(volume, moneyFloatPtr(result.MaxVolume))
	if result.MaxItems != nil && *result.MaxItems > 0 {
		result.CapacityUtilization = percentage(quantity, float64Ptr(float64(*result.MaxItems)))
	}
	return result, nil
}

type LoadUtilization struct {
	LoadID              int64
	CurrentWeight       *accountingmoney.Money
	CurrentVolume       *accountingmoney.Money
	CurrentItems        *int
	MaxWeight           *accountingmoney.Money
	MaxVolume           *accountingmoney.Money
	MaxItems            *int
	CapacityUtilization float64
	WeightUtilization   float64
	VolumeUtilization   float64
}

func (s *Service) ListPendingTransfers(ctx context.Context, companyID int64) ([]*TransferOrder, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	draft := TransferStatusDraft
	approved := TransferStatusApproved
	drafts, err := s.repo.ListTransferOrders(ctx, companyID, &draft)
	if err != nil {
		return nil, err
	}
	approvedTransfers, err := s.repo.ListTransferOrders(ctx, companyID, &approved)
	if err != nil {
		return nil, err
	}
	result := append(drafts, approvedTransfers...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *Service) GetRouteMetrics(ctx context.Context, routeID int64) (*RouteMetrics, error) {
	route, err := s.repo.GetRoute(ctx, routeID)
	if err != nil {
		return nil, fmt.Errorf("route not found: %w", err)
	}
	stops, err := s.repo.GetRouteStops(ctx, routeID)
	if err != nil {
		return nil, fmt.Errorf("failed to load route stops: %w", err)
	}
	result := &RouteMetrics{
		RouteID:           routeID,
		TotalStops:        len(stops),
		TotalDistance:     route.TotalDistanceKm,
		EstimatedDuration: route.EstimatedDurationMinutes,
		OptimizationScore: route.OptimizationScore,
	}
	var first, last *time.Time
	for _, stop := range stops {
		if stop.ActualArrivalAt != nil || stop.ActualDepartureAt != nil {
			result.CompletedStops++
		}
		for _, event := range []*time.Time{stop.ActualArrivalAt, stop.ActualDepartureAt} {
			if event == nil {
				continue
			}
			if first == nil || event.Before(*first) {
				first = event
			}
			if last == nil || event.After(*last) {
				last = event
			}
		}
	}
	if first != nil && last != nil && !last.Before(*first) {
		minutes := int(last.Sub(*first).Minutes())
		result.ActualDuration = &minutes
	}
	if result.TotalStops > 0 {
		result.EfficiencyRating = float64(result.CompletedStops) / float64(result.TotalStops) * 100
	}
	return result, nil
}

type RouteMetrics struct {
	RouteID           int64
	TotalStops        int
	CompletedStops    int
	TotalDistance     *float64
	EstimatedDuration *int
	ActualDuration    *int
	OptimizationScore *accountingmoney.Money
	EfficiencyRating  float64
}

func validRuleType(ruleType RuleType) bool {
	switch ruleType {
	case RuleTypeCapacity, RuleTypeWeight, RuleTypeTimeWindow, RuleTypeVehicleType, RuleTypeCustom:
		return true
	default:
		return false
	}
}

func uniqueShipmentIDs(items []*LoadItem) []int64 {
	seen := make(map[int64]struct{})
	result := make([]int64, 0, len(items))
	for _, item := range items {
		if item.ShipmentID == nil || *item.ShipmentID == 0 {
			continue
		}
		if _, ok := seen[*item.ShipmentID]; ok {
			continue
		}
		seen[*item.ShipmentID] = struct{}{}
		result = append(result, *item.ShipmentID)
	}
	return result
}

func moneyIsNonPositive(value accountingmoney.Money) bool {
	return moneyFloat(value) <= 0
}

func moneyFloat(value accountingmoney.Money) float64 {
	result, _ := strconv.ParseFloat(value.String(), 64)
	return result
}

func moneyFloatPtr(value *accountingmoney.Money) *float64 {
	if value == nil {
		return nil
	}
	result := moneyFloat(*value)
	return &result
}

func moneyFromFloat(value float64, scale int) accountingmoney.Money {
	result, _ := accountingmoney.Parse(strconv.FormatFloat(value, 'f', -1, 64), scale)
	return result
}

func optionalMoneyFromFloat(value *float64, scale int) *accountingmoney.Money {
	if value == nil {
		return nil
	}
	result := moneyFromFloat(*value, scale)
	return &result
}

func intFromFloat(value float64) *int {
	result := int(value)
	return &result
}

func float64Ptr(value float64) *float64 {
	return &value
}

func percentage(value float64, maximum *float64) float64 {
	if maximum == nil || *maximum <= 0 {
		return 0
	}
	return value / *maximum * 100
}
