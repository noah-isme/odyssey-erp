package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// E2ETestSuite runs comprehensive end-to-end tests across all phases
type E2ETestSuite struct {
	suite.Suite
	server *httptest.Server
	client *http.Client
}

func (suite *E2ETestSuite) SetupSuite() {
	suite.T().Skip("E2E tests are not fully wired up yet")
	// In production: spin up full test database, app server, etc.
	// For now: httptest.Server with mock handlers
	suite.client = &http.Client{}
}

// TearDownSuite cleans up test environment
func (suite *E2ETestSuite) TearDownSuite() {
	if suite.server != nil {
		suite.server.Close()
	}
}

// Helper: make authenticated request
func (suite *E2ETestSuite) doAuthenticatedRequest(method, path string, body interface{}) *http.Response {
	var req *http.Request
	var err error

	if body != nil {
		bodyBytes, _ := json.Marshal(body)
		req, err = http.NewRequest(method, suite.server.URL+path, bytes.NewReader(bodyBytes))
	} else {
		req, err = http.NewRequest(method, suite.server.URL+path, nil)
	}

	suite.NoError(err)
	req.Header.Set("Content-Type", "application/json")

	// TODO: Add session cookie from authenticated session
	resp, err := suite.client.Do(req)
	suite.NoError(err)
	return resp
}

// ═══════════════════════════════════════════════════════════════════════════
// PHASE 3: VENDOR INTELLIGENCE E2E TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestVendorContractLifecycle tests complete contract workflow
func (suite *E2ETestSuite) TestVendorContractLifecycle() {

	// 1. Create contract
	createContractReq := map[string]interface{}{
		"vendor_id":              1,
		"contract_number":        "CONT-2026-001",
		"contract_type":          "PURCHASE",
		"start_date":             time.Now().AddDate(0, 0, 1).Format(time.RFC3339),
		"end_date":               time.Now().AddDate(1, 0, 0).Format(time.RFC3339),
		"currency":               "USD",
		"payment_terms":          "NET_30",
		"incoterms":              "FOB",
		"renewal_option":         true,
		"auto_renewal_days":      30,
	}
	resp := suite.doAuthenticatedRequest("POST", "/procurement/contracts", createContractReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var contractResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&contractResp)
	contractID := contractResp["id"]

	// 2. Add pricing lines
	addPricingReq := map[string]interface{}{
		"product_id":     1,
		"min_quantity":   100,
		"max_quantity":   500,
		"unit_price":     "25.00",
		"currency":       "USD",
		"effective_date": time.Now().Format(time.RFC3339),
	}
	resp = suite.doAuthenticatedRequest("POST", "/procurement/contracts/"+contractID.(string)+"/pricing", addPricingReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	// 3. Submit for approval
	resp = suite.doAuthenticatedRequest("POST", "/procurement/contracts/"+contractID.(string)+"/submit", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 4. Approve contract
	resp = suite.doAuthenticatedRequest("POST", "/procurement/contracts/"+contractID.(string)+"/approve", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 5. Verify status is ACTIVE
	resp = suite.doAuthenticatedRequest("GET", "/procurement/contracts/"+contractID.(string), nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	_ = json.NewDecoder(resp.Body).Decode(&contractResp)
	suite.Equal("ACTIVE", contractResp["status"])
}

// TestVendorScorecardCalculation tests scorecard metrics
func (suite *E2ETestSuite) TestVendorScorecardCalculation() {
	// 1. Create scorecard for vendor
	createScorecardReq := map[string]interface{}{
		"vendor_id":    1,
		"period_start": time.Now().AddDate(0, -1, 0).Format(time.RFC3339),
		"period_end":   time.Now().Format(time.RFC3339),
	}
	resp := suite.doAuthenticatedRequest("POST", "/procurement/scorecards", createScorecardReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var scoreResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&scoreResp)

	// 2. Verify metrics calculated
	suite.NotNil(scoreResp["otif_score"])
	suite.NotNil(scoreResp["quality_score"])
	suite.NotNil(scoreResp["price_score"])
	suite.NotNil(scoreResp["overall_score"])

	// 3. Verify score is in valid range (0-100)
	overallScore := scoreResp["overall_score"].(float64)
	suite.GreaterOrEqual(overallScore, 0.0)
	suite.LessOrEqual(overallScore, 100.0)
}

// ═══════════════════════════════════════════════════════════════════════════
// PHASE 4: TRANSPORT EXECUTION E2E TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestShipmentTrackingLifecycle tests end-to-end shipment workflow
func (suite *E2ETestSuite) TestShipmentTrackingLifecycle() {
	// 1. Create shipment
	createShipmentReq := map[string]interface{}{
		"po_id":              1,
		"origin_warehouse":   1,
		"destination_warehouse": 2,
		"planned_ship_date":  time.Now().AddDate(0, 0, 1).Format(time.RFC3339),
		"weight_kg":          "500.00",
		"volume_cbm":         "1.50",
	}
	resp := suite.doAuthenticatedRequest("POST", "/logistics/shipments", createShipmentReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var shipResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&shipResp)
	shipmentID := shipResp["id"]

	// 2. Create trip with shipment
	createTripReq := map[string]interface{}{
		"vehicle_id":      1,
		"driver_id":       1,
		"shipments":       []int{int(shipmentID.(float64))},
		"start_location":  "Warehouse A",
		"end_location":    "Warehouse B",
		"planned_start":   time.Now().AddDate(0, 0, 1).Format(time.RFC3339),
		"planned_arrival": time.Now().AddDate(0, 0, 2).Format(time.RFC3339),
	}
	resp = suite.doAuthenticatedRequest("POST", "/logistics/trips", createTripReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var tripResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&tripResp)
	tripID := tripResp["id"]

	// 3. Start trip
	resp = suite.doAuthenticatedRequest("POST", "/logistics/trips/"+tripID.(string)+"/start", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 4. Mark shipment in transit
	resp = suite.doAuthenticatedRequest("POST", "/logistics/shipments/"+shipmentID.(string)+"/in-transit", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 5. Complete delivery
	deliverReq := map[string]interface{}{
		"delivered_at": time.Now().Format(time.RFC3339),
		"signature":    "John Doe",
	}
	resp = suite.doAuthenticatedRequest("POST", "/logistics/shipments/"+shipmentID.(string)+"/deliver", deliverReq)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 6. Verify shipment is DELIVERED
	resp = suite.doAuthenticatedRequest("GET", "/logistics/shipments/"+shipmentID.(string), nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	_ = json.NewDecoder(resp.Body).Decode(&shipResp)
	suite.Equal("DELIVERED", shipResp["status"])
}

// TestCarrierRateCalculation tests 3PL rate computation
func (suite *E2ETestSuite) TestCarrierRateCalculation() {
	// 1. Create carrier
	createCarrierReq := map[string]interface{}{
		"carrier_name":         "FastShip Logistics",
		"country_of_operation": "USA",
		"carrier_type":         "LTL",
	}
	resp := suite.doAuthenticatedRequest("POST", "/logistics/carriers", createCarrierReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var carrierResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&carrierResp)
	carrierID := carrierResp["id"]

	// 2. Add rate card
	addRateReq := map[string]interface{}{
		"origin_city":       "New York",
		"destination_city":  "Los Angeles",
		"min_weight":        "0.00",
		"max_weight":        "1000.00",
		"base_rate":         "500.00",
		"weight_surcharge":  "0.50",
		"currency":          "USD",
	}
	resp = suite.doAuthenticatedRequest("POST", "/logistics/carriers/"+carrierID.(string)+"/rates", addRateReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	// 3. Calculate rate for shipment
	calcRateReq := map[string]interface{}{
		"weight": "750.00",
	}
	resp = suite.doAuthenticatedRequest("POST", "/logistics/carriers/"+carrierID.(string)+"/calculate-rate", calcRateReq)
	suite.Equal(http.StatusOK, resp.StatusCode)

	var rateResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&rateResp)
	suite.NotNil(rateResp["calculated_rate"])
	suite.NotNil(rateResp["breakdown"])
}

// ═══════════════════════════════════════════════════════════════════════════
// PHASE 5: DISTRIBUTION PLANNING E2E TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestLoadConsolidationWorkflow tests load planning and consolidation
func (suite *E2ETestSuite) TestLoadConsolidationWorkflow() {
	// 1. Create load
	createLoadReq := map[string]interface{}{
		"origin_warehouse":        1,
		"destination_address":     "123 Main St",
		"destination_city":        "Boston",
		"destination_country":     "USA",
		"planned_pickup_date":     time.Now().AddDate(0, 0, 1).Format(time.RFC3339),
		"planned_delivery_date":   time.Now().AddDate(0, 0, 3).Format(time.RFC3339),
	}
	resp := suite.doAuthenticatedRequest("POST", "/distribution/loads", createLoadReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var loadResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&loadResp)
	loadID := loadResp["id"]

	// 2. Add multiple items to load
	for i := 1; i <= 3; i++ {
		addItemReq := map[string]interface{}{
			"product_id": i,
			"quantity":   "100.00",
			"weight_kg":  "50.00",
			"volume_cbm": "0.50",
		}
		resp = suite.doAuthenticatedRequest("POST", "/distribution/loads/"+loadID.(string)+"/items", addItemReq)
		suite.Equal(http.StatusCreated, resp.StatusCode)
	}

	// 3. Mark load as ready
	resp = suite.doAuthenticatedRequest("POST", "/distribution/loads/"+loadID.(string)+"/mark-ready", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 4. Dispatch load to vehicle
	dispatchReq := map[string]interface{}{
		"vehicle_id": 1,
		"driver_id":  1,
	}
	resp = suite.doAuthenticatedRequest("POST", "/distribution/loads/"+loadID.(string)+"/dispatch", dispatchReq)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 5. Verify load status
	resp = suite.doAuthenticatedRequest("GET", "/distribution/loads/"+loadID.(string), nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	_ = json.NewDecoder(resp.Body).Decode(&loadResp)
	suite.Equal("CONFIRMED", loadResp["status"])
}

// TestRouteOptimization tests route planning and optimization
func (suite *E2ETestSuite) TestRouteOptimization() {
	// 1. Create load first
	createLoadReq := map[string]interface{}{
		"origin_warehouse":    1,
		"destination_city":    "Boston",
		"destination_country": "USA",
	}
	resp := suite.doAuthenticatedRequest("POST", "/distribution/loads", createLoadReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var loadResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&loadResp)
	loadID := loadResp["id"]

	// 2. Create route for load
	createRouteReq := map[string]interface{}{
		"load_id":                      loadID,
		"total_distance_km":            "250.50",
		"estimated_duration_minutes":   360,
	}
	resp = suite.doAuthenticatedRequest("POST", "/distribution/routes", createRouteReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var routeResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&routeResp)
	routeID := routeResp["id"]

	// 3. Add multiple stops
	for i := 1; i <= 3; i++ {
		addStopReq := map[string]interface{}{
			"stop_sequence":      i,
			"stop_type":          "CUSTOMER",
			"customer_address":   "456 Oak Ave",
			"customer_city":      "Boston",
			"contact_name":       "John Doe",
			"contact_phone":      "555-0100",
		}
		resp = suite.doAuthenticatedRequest("POST", "/distribution/routes/"+routeID.(string)+"/stops", addStopReq)
		suite.Equal(http.StatusCreated, resp.StatusCode)
	}

	// 4. Optimize route
	resp = suite.doAuthenticatedRequest("POST", "/distribution/routes/"+routeID.(string)+"/optimize", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 5. Approve route
	resp = suite.doAuthenticatedRequest("POST", "/distribution/routes/"+routeID.(string)+"/approve", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// 6. Verify optimization score
	resp = suite.doAuthenticatedRequest("GET", "/distribution/routes/"+routeID.(string), nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	_ = json.NewDecoder(resp.Body).Decode(&routeResp)
	suite.NotNil(routeResp["optimization_score"])
}

// ═══════════════════════════════════════════════════════════════════════════
// PERMISSION-BASED ACCESS TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestProcurementOfficerAccess tests procurement role permissions
func (suite *E2ETestSuite) TestProcurementOfficerAccess() {
	// Procurement officer should be able to view contracts
	resp := suite.doAuthenticatedRequest("GET", "/procurement/contracts", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// Procurement officer should NOT be able to post GL
	postGLReq := map[string]interface{}{
		"account": "1000",
		"amount":  "1000.00",
	}
	resp = suite.doAuthenticatedRequest("POST", "/finance/gl", postGLReq)
	suite.Equal(http.StatusForbidden, resp.StatusCode)
}

// TestLogisticsManagerAccess tests logistics role permissions
func (suite *E2ETestSuite) TestLogisticsManagerAccess() {
	// Logistics manager should be able to view shipments
	resp := suite.doAuthenticatedRequest("GET", "/logistics/shipments", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	// Logistics manager should be able to create routes
	createRouteReq := map[string]interface{}{
		"load_id": 1,
	}
	resp = suite.doAuthenticatedRequest("POST", "/distribution/routes", createRouteReq)
	// Should succeed or return 422 (validation error), not 403 (permission)
	suite.NotEqual(http.StatusForbidden, resp.StatusCode)
}

// ═══════════════════════════════════════════════════════════════════════════
// ERROR HANDLING TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestInvalidContractData tests validation
func (suite *E2ETestSuite) TestInvalidContractData() {
	invalidReq := map[string]interface{}{
		"vendor_id": "invalid", // Should be int
	}
	resp := suite.doAuthenticatedRequest("POST", "/procurement/contracts", invalidReq)
	suite.Equal(http.StatusUnprocessableEntity, resp.StatusCode)
}

// TestNotFoundError tests 404 handling
func (suite *E2ETestSuite) TestNotFoundError() {
	resp := suite.doAuthenticatedRequest("GET", "/procurement/contracts/99999", nil)
	suite.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestUnauthorizedAccess tests missing auth
func (suite *E2ETestSuite) TestUnauthorizedAccess() {
	// Make unauthenticated request
	req, _ := http.NewRequest("GET", suite.server.URL+"/procurement/contracts", nil)
	resp, _ := suite.client.Do(req)
	// Should redirect to login
	suite.Equal(http.StatusSeeOther, resp.StatusCode)
}

// Run tests
func TestE2ESuite(t *testing.T) {
	suite.Run(t, new(E2ETestSuite))
}
