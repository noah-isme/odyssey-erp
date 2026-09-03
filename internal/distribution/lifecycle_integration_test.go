//go:build integration

package distribution

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/logistics"
)

// TestDatabaseLoadLifecycle exercises the persisted planning -> shipment ->
// dispatch -> delivery path. It is opt-in because it needs a migrated
// PostgreSQL database and deliberately uses the real logistics and inventory
// services at the two module boundaries.
func TestDatabaseLoadLifecycle(t *testing.T) {
	dsn := os.Getenv("DISTRIBUTION_TEST_DSN")
	if dsn == "" {
		dsn = os.Getenv("PG_DSN")
	}
	if dsn == "" {
		t.Skip("DISTRIBUTION_TEST_DSN or PG_DSN is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	userID := insertID(t, pool, `INSERT INTO users(email, password_hash) VALUES($1, 'integration') RETURNING id`, "distribution-"+suffix+"@example.test")
	companyID := insertID(t, pool, `INSERT INTO companies(code, name) VALUES($1, 'Distribution integration') RETURNING id`, "DIST-"+suffix)
	branchID := insertID(t, pool, `INSERT INTO branches(company_id, code, name) VALUES($1, $2, 'Distribution branch') RETURNING id`, companyID, "DIST-B-"+suffix)
	warehouseID := insertID(t, pool, `INSERT INTO warehouses(branch_id, code, name) VALUES($1, $2, 'Distribution warehouse') RETURNING id`, branchID, "DIST-W-"+suffix)
	unitID := insertID(t, pool, `INSERT INTO units(code, name) VALUES($1, 'Each') RETURNING id`, "DIST-U-"+suffix)
	categoryID := insertID(t, pool, `INSERT INTO categories(code, name) VALUES($1, 'Distribution category') RETURNING id`, "DIST-C-"+suffix)
	productID := insertID(t, pool, `INSERT INTO products(sku, name, category_id, unit_id, price) VALUES($1, 'Distribution product', $2, $3, 10) RETURNING id`, "DIST-P-"+suffix, categoryID, unitID)
	carrierID := insertID(t, pool, `INSERT INTO carriers(company_id, carrier_name, carrier_code, created_by, updated_by) VALUES($1, 'Integration carrier', $2, $3, $3) RETURNING id`, companyID, "DIST-CAR-"+suffix, userID)

	t.Cleanup(func() {
		cleanupDistributionFixture(ctx, pool, userID, companyID, branchID, warehouseID, unitID, categoryID, productID, carrierID)
	})

	stock := inventory.NewService(inventory.NewRepository(pool), nil, nil, inventory.ServiceConfig{}, nil)
	if _, err := stock.PostAdjustment(ctx, inventory.AdjustmentInput{Code: "DIST-IN-" + suffix, WarehouseID: warehouseID, ProductID: productID, Qty: 5, UnitCost: 10, ActorID: userID, RefModule: "DISTRIBUTION_TEST"}); err != nil {
		t.Fatal(err)
	}

	logisticsService := logistics.NewService(pool)
	distributionService := NewServiceWithDependencies(NewRepository(pool), Dependencies{
		Shipments: integrationShipmentGateway{service: logisticsService},
		Inventory: integrationInventoryGateway{service: stock},
	})
	start := time.Now().UTC().Truncate(24 * time.Hour)
	if _, err := distributionService.SetupPlanningHorizon(ctx, CreatePlanningHorizonInput{CompanyID: companyID, WarehouseID: warehouseID, PlanningStartDate: start, PlanningEndDate: start.AddDate(0, 0, 7), CreatedBy: userID}); err != nil {
		t.Fatal(err)
	}
	load, err := distributionService.CreateLoad(ctx, CreateLoadInput{CompanyID: companyID, OriginWarehouseID: warehouseID, DestinationCity: "Jakarta", DestinationCountry: "ID", CreatedBy: userID})
	if err != nil {
		t.Fatal(err)
	}
	shipmentID, err := distributionService.CreateShipmentForLoad(ctx, load.ID, ShipmentCreateInput{ShipmentNumber: "DIST-SHIP-" + suffix, ShipmentType: "DELIVERY", CreatedBy: userID}, []ShipmentLineInput{{ProductID: productID, Quantity: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := distributionService.MarkLoadReady(ctx, load.ID); err != nil {
		t.Fatal(err)
	}
	carrierService := "STANDARD"
	if err := distributionService.DispatchLoad(ctx, load.ID, nil, nil, &carrierID, &carrierService); err != nil {
		t.Fatal(err)
	}
	if err := distributionService.DeliverLoad(ctx, load.ID, userID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	finalLoad, _, err := distributionService.GetLoad(ctx, load.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalLoad.Status != LoadStatusDelivered {
		t.Fatalf("load status=%s", finalLoad.Status)
	}
	var shipmentStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM shipments WHERE id=$1`, shipmentID).Scan(&shipmentStatus); err != nil {
		t.Fatal(err)
	}
	if shipmentStatus != "DELIVERED" {
		t.Fatalf("shipment status=%s", shipmentStatus)
	}
	var balance float64
	if err := pool.QueryRow(ctx, `SELECT qty FROM inventory_balances WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, productID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 3 {
		t.Fatalf("inventory balance=%v want 3", balance)
	}
}

type integrationShipmentGateway struct {
	service *logistics.Service
}

func (g integrationShipmentGateway) CreateShipment(ctx context.Context, input ShipmentCreateInput) (int64, error) {
	shipment, err := g.service.CreateShipment(ctx, logistics.CreateShipmentInput{CompanyID: input.CompanyID, ShipmentNumber: input.ShipmentNumber, ShipmentType: logistics.ShipmentType(input.ShipmentType), OriginWarehouseID: input.OriginWarehouseID, DestinationWarehouseID: input.DestinationWarehouseID, DestinationAddress: input.DestinationAddress, DestinationCity: input.DestinationCity, DestinationCountry: input.DestinationCountry, PlannedDispatchAt: input.PlannedDispatchAt, PlannedDeliveryAt: input.PlannedDeliveryAt, CreatedBy: input.CreatedBy})
	if err != nil {
		return 0, err
	}
	return shipment.ID, nil
}

func (g integrationShipmentGateway) AddShipmentLine(ctx context.Context, input ShipmentLineInput) error {
	_, err := g.service.AddItemToShipment(ctx, logistics.AddShipmentLineInput{CompanyID: input.CompanyID, ShipmentID: input.ShipmentID, ProductID: input.ProductID, Quantity: input.Quantity, WeightKg: input.WeightKg, VolumeCbm: input.VolumeCbm})
	return err
}

func (g integrationShipmentGateway) GetShipmentLines(ctx context.Context, shipmentID int64) ([]ShipmentLine, error) {
	lines, err := g.service.GetShipmentLines(ctx, shipmentID)
	if err != nil {
		return nil, err
	}
	result := make([]ShipmentLine, 0, len(lines))
	for _, line := range lines {
		quantity, parseErr := strconv.ParseFloat(line.Quantity.String(), 64)
		if parseErr != nil {
			return nil, parseErr
		}
		result = append(result, ShipmentLine{ProductID: line.ProductID, Quantity: quantity})
	}
	return result, nil
}

func (g integrationShipmentGateway) DispatchShipment(ctx context.Context, shipmentID int64, vehicleID, driverID, carrierID *int64, carrierService *string) error {
	var serviceType *logistics.CarrierServiceType
	if carrierService != nil {
		value := logistics.CarrierServiceType(*carrierService)
		serviceType = &value
	}
	return g.service.DispatchShipment(ctx, shipmentID, vehicleID, driverID, carrierID, serviceType)
}

func (g integrationShipmentGateway) MarkShipmentInTransit(ctx context.Context, shipmentID int64) error {
	return g.service.MarkShipmentInTransit(ctx, shipmentID)
}

func (g integrationShipmentGateway) MarkShipmentDelivered(ctx context.Context, shipmentID int64, deliveredAt time.Time) error {
	return g.service.MarkShipmentDelivered(ctx, shipmentID, deliveredAt)
}

type integrationInventoryGateway struct {
	service *inventory.Service
}

func (g integrationInventoryGateway) PostAdjustment(ctx context.Context, input InventoryAdjustmentInput) error {
	_, err := g.service.PostAdjustment(ctx, inventory.AdjustmentInput{Code: input.Code, WarehouseID: input.WarehouseID, ProductID: input.ProductID, Qty: input.Quantity, ActorID: input.ActorID, RefModule: input.RefModule, RefID: input.RefID, Note: input.Note})
	return err
}

func insertID(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func cleanupDistributionFixture(ctx context.Context, pool *pgxpool.Pool, userID, companyID, branchID, warehouseID, unitID, categoryID, productID, carrierID int64) {
	_, _ = pool.Exec(ctx, `DELETE FROM inventory_cards WHERE warehouse_id=$1`, warehouseID)
	_, _ = pool.Exec(ctx, `DELETE FROM inventory_tx_lines WHERE tx_id IN (SELECT id FROM inventory_tx WHERE warehouse_id=$1)`, warehouseID)
	_, _ = pool.Exec(ctx, `DELETE FROM inventory_tx WHERE warehouse_id=$1`, warehouseID)
	_, _ = pool.Exec(ctx, `DELETE FROM inventory_balances WHERE warehouse_id=$1`, warehouseID)
	_, _ = pool.Exec(ctx, `DELETE FROM load_items WHERE company_id=$1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM loads WHERE company_id=$1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM shipment_lines WHERE company_id=$1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM shipments WHERE company_id=$1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM planning_rules WHERE company_id=$1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM planning_horizons WHERE company_id=$1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM carriers WHERE id=$1`, carrierID)
	_, _ = pool.Exec(ctx, `DELETE FROM products WHERE id=$1`, productID)
	_, _ = pool.Exec(ctx, `DELETE FROM categories WHERE id=$1`, categoryID)
	_, _ = pool.Exec(ctx, `DELETE FROM units WHERE id=$1`, unitID)
	_, _ = pool.Exec(ctx, `DELETE FROM warehouses WHERE id=$1`, warehouseID)
	_, _ = pool.Exec(ctx, `DELETE FROM branches WHERE id=$1`, branchID)
	_, _ = pool.Exec(ctx, `DELETE FROM companies WHERE id=$1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
}
