package distribution

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"

	"github.com/odyssey-erp/odyssey-erp/internal/shared/testhelpers"
	"github.com/odyssey-erp/odyssey-erp/internal/shared/accountingmoney"
)

type DistributionTestSuite struct {
	suite.Suite
	db         *pgxpool.Pool
	repo       *DistributionRepository
	companyID  int64
	warehouseID int64
}

func (suite *DistributionTestSuite) SetupSuite() {
	db := testhelpers.SetupTestDB(suite.T())
	suite.db = db
	suite.repo = NewRepository(db)
	suite.companyID = 1
	suite.warehouseID = 1
}

func (suite *DistributionTestSuite) TearDownSuite() {
	suite.db.Close()
}

func (suite *DistributionTestSuite) TestPlanningHorizonLifecycle() {
	ctx := context.Background()
	startDate := time.Now().Add(24 * time.Hour)
	endDate := startDate.Add(30 * 24 * time.Hour)

	// Create planning horizon
	horizonID, err := suite.repo.CreatePlanningHorizon(ctx, CreatePlanningHorizonInput{
		CompanyID:         suite.companyID,
		WarehouseID:       suite.warehouseID,
		PlanningStartDate: startDate,
		PlanningEndDate:   endDate,
		CreatedBy:         1,
	})
	suite.NoError(err)
	suite.Greater(horizonID, int64(0))

	// Get planning horizon
	horizon, err := suite.repo.GetPlanningHorizon(ctx, horizonID)
	suite.NoError(err)
	suite.NotNil(horizon)
	suite.Equal(suite.companyID, horizon.CompanyID)

	// List planning horizons
	horizons, err := suite.repo.ListPlanningHorizons(ctx, suite.companyID)
	suite.NoError(err)
	suite.Greater(len(horizons), 0)

	// Update status
	err = suite.repo.UpdatePlanningHorizonStatus(ctx, horizonID, "CLOSED")
	suite.NoError(err)
}

func (suite *DistributionTestSuite) TestPlanningRuleManagement() {
	ctx := context.Background()

	// Create planning rule
	ruleID, err := suite.repo.CreatePlanningRule(ctx, CreatePlanningRuleInput{
		CompanyID:           suite.companyID,
		WarehouseID:         suite.warehouseID,
		RuleName:            "Max Weight 1000kg",
		RuleType:            "CAPACITY",
		MaxLoadWeightKg:     ptrInterface(1000),
		Priority:            1,
		CreatedBy:           1,
	})
	suite.NoError(err)
	suite.Greater(ruleID, int64(0))

	// List rules
	rules, err := suite.repo.ListPlanningRules(ctx, suite.warehouseID)
	suite.NoError(err)
	suite.GreaterOrEqual(len(rules), 1)

	// Update active status
	err = suite.repo.UpdateRuleActive(ctx, ruleID, false)
	suite.NoError(err)
}

func (suite *DistributionTestSuite) TestLoadConsolidation() {
	ctx := context.Background()

	// Create load
	loadID, err := suite.repo.CreateLoad(ctx, CreateLoadInput{
		CompanyID:             suite.companyID,
		OriginWarehouseID:     suite.warehouseID,
		DestinationAddress:    "123 Main St",
		DestinationCity:       "New York",
		DestinationCountry:    "USA",
		CreatedBy:             1,
	})
	suite.NoError(err)
	suite.Greater(loadID, int64(0))

	// Get load
	load, err := suite.repo.GetLoad(ctx, loadID)
	suite.NoError(err)
	suite.NotNil(load)
	suite.Equal(LoadStatusDraft, load.Status)

	// Add load items
	itemID, err := suite.repo.AddLoadItem(ctx, AddLoadItemInput{
		CompanyID: suite.companyID,
		LoadID:    loadID,
		ProductID: 1,
		Quantity:  ptrInterface(100),
	})
	suite.NoError(err)
	suite.Greater(itemID, int64(0))

	// Get load items
	items, err := suite.repo.GetLoadItems(ctx, loadID)
	suite.NoError(err)
	suite.Equal(1, len(items))

	// Update load status
	err = suite.repo.UpdateLoadStatus(ctx, loadID, LoadStatusReady)
	suite.NoError(err)

	// Dispatch load
	vehicleID := int64(1)
	driverID := int64(1)
	err = suite.repo.UpdateLoadDispatch(ctx, loadID, &vehicleID, &driverID, nil, nil)
	suite.NoError(err)

	// List loads
	loads, err := suite.repo.ListLoads(ctx, suite.companyID, nil)
	suite.NoError(err)
	suite.Greater(len(loads), 0)
}

func (suite *DistributionTestSuite) TestRouteOptimization() {
	ctx := context.Background()

	// Create load first
	loadID, _ := suite.repo.CreateLoad(ctx, CreateLoadInput{
		CompanyID:         suite.companyID,
		OriginWarehouseID: suite.warehouseID,
		DestinationCity:   "Boston",
		DestinationCountry: "USA",
		CreatedBy:         1,
	})

	// Create route
	routeID, err := suite.repo.CreateRoute(ctx, CreateRouteInput{
		CompanyID:                suite.companyID,
		LoadID:                   loadID,
		TotalDistanceKm:          ptrFloat(250.5),
		EstimatedDurationMinutes: ptrInt(360),
		CreatedBy:                1,
	})
	suite.NoError(err)
	suite.Greater(routeID, int64(0))

	// Get route
	route, err := suite.repo.GetRoute(ctx, routeID)
	suite.NoError(err)
	suite.NotNil(route)

	// Add route stop
	stopID, err := suite.repo.AddRouteStop(ctx, AddRouteStopInput{
		CompanyID:       suite.companyID,
		RouteID:         routeID,
		StopSequence:    1,
		StopType:        "CUSTOMER",
		CustomerAddress: "456 Oak Ave",
		CustomerCity:    "Boston",
		ContactName:     "John Doe",
		ContactPhone:    "555-0100",
	})
	suite.NoError(err)
	suite.Greater(stopID, int64(0))

	// Get route stops
	stops, err := suite.repo.GetRouteStops(ctx, routeID)
	suite.NoError(err)
	suite.Equal(1, len(stops))

	// Update stop times
	now := time.Now()
	err = suite.repo.UpdateStopActualTimes(ctx, stopID, &now, nil)
	suite.NoError(err)

	// List routes
	routes, err := suite.repo.ListRoutes(ctx, suite.companyID, nil)
	suite.NoError(err)
	suite.Greater(len(routes), 0)

	// Update route status
	err = suite.repo.UpdateRouteStatus(ctx, routeID, "OPTIMIZED")
	suite.NoError(err)
}

func (suite *DistributionTestSuite) TestTransferOrders() {
	ctx := context.Background()

	toWarehouseID := int64(2)

	// Create transfer order
	transferID, err := suite.repo.CreateTransferOrder(ctx, CreateTransferOrderInput{
		CompanyID:       suite.companyID,
		FromWarehouseID: suite.warehouseID,
		ToWarehouseID:   toWarehouseID,
		CreatedBy:       1,
	})
	suite.NoError(err)
	suite.Greater(transferID, int64(0))

	// Get transfer
	transfer, err := suite.repo.GetTransferOrder(ctx, transferID)
	suite.NoError(err)
	suite.NotNil(transfer)
	suite.Equal(TransferStatusDraft, transfer.Status)

	// Add transfer line
	lineID, err := suite.repo.AddTransferLine(ctx, AddTransferLineInput{
		CompanyID:       suite.companyID,
		TransferOrderID: transferID,
		ProductID:       1,
		QuantityRequested: ptrInterface(50),
		LotNumber:       "LOT-001",
	})
	suite.NoError(err)
	suite.Greater(lineID, int64(0))

	// Get transfer lines
	lines, err := suite.repo.GetTransferLines(ctx, transferID)
	suite.NoError(err)
	suite.Equal(1, len(lines))

	// Update transfer status
	err = suite.repo.UpdateTransferStatus(ctx, transferID, "APPROVED")
	suite.NoError(err)

	// Dispatch transfer
	carrierID := int64(1)
	err = suite.repo.UpdateTransferDispatch(ctx, transferID, nil, nil, &carrierID)
	suite.NoError(err)

	// Update line receipt
	err = suite.repo.UpdateTransferLineReceipt(ctx, lineID, ptrInterface(50))
	suite.NoError(err)

	// List transfers
	transfers, err := suite.repo.ListTransferOrders(ctx, suite.companyID, nil)
	suite.NoError(err)
	suite.Greater(len(transfers), 0)
}

func (suite *DistributionTestSuite) TestServiceLayerValidation() {
	ctx := context.Background()
	service := NewService(suite.repo)

	// Test planning horizon validation
	_, err := service.SetupPlanningHorizon(ctx, 1, 1, time.Now(), time.Now().Add(-24*time.Hour))
	suite.Error(err) // End date before start date

	// Test load validation
	_, err = service.CreateLoad(ctx, 1, 1, nil, "Test Address", "City", "Country")
	suite.NoError(err) // Should succeed
}

// Helper functions
func ptrInterface(v interface{}) *interface{} {
	return &v
}

func ptrFloat(v float64) *float64 {
	return &v
}

func ptrInt(v int) *int {
	return &v
}

func TestDistributionSuite(t *testing.T) {
	suite.Run(t, new(DistributionTestSuite))
}
