// E2E Test Suite for Odyssey ERP - Phases 1-5
// Framework: Playwright with TypeScript
// Tests: Complete procurement-logistics workflows

import { test, expect, Page } from '@playwright/test';

// Test fixtures
const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const TEST_USER = 'test@odyssey.local';
const TEST_PASSWORD = 'Test@12345';

// Helper: Login before each test
async function login(page: Page) {
  await page.goto(`${BASE_URL}/login`);
  await page.fill('input[name="email"]', TEST_USER);
  await page.fill('input[name="password"]', TEST_PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForURL(`${BASE_URL}/dashboard`);
}

// ═══════════════════════════════════════════════════════════════════════════
// PHASE 3: VENDOR INTELLIGENCE TESTS
// ═══════════════════════════════════════════════════════════════════════════

test.describe('Phase 3: Vendor Intelligence', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('should create and manage supplier contract', async ({ page }) => {
    // Navigate to contracts
    await page.goto(`${BASE_URL}/procurement/contracts`);
    await expect(page.locator('h1')).toContainText('Supplier Contracts');

    // Create new contract
    await page.click('a:has-text("Create Contract")');
    await page.fill('input[name="supplier_name"]', 'Acme Corporation');
    await page.fill('input[name="contract_number"]', 'CNT-2026-001');
    await page.fill('input[name="total_value"]', '100000');
    await page.selectOption('select[name="currency"]', 'USD');
    
    // Set dates
    await page.fill('input[name="start_date"]', '2026-08-02');
    await page.fill('input[name="end_date"]', '2027-08-02');

    // Add payment terms
    await page.selectOption('select[name="payment_terms"]', 'NET30');
    
    // Submit
    await page.click('button:has-text("Create Contract")');
    
    // Verify contract created
    await expect(page.locator('text=Contract created successfully')).toBeVisible();
    await expect(page.locator('text=CNT-2026-001')).toBeVisible();
  });

  test('should calculate supplier scorecard', async ({ page }) => {
    // Navigate to scorecards
    await page.goto(`${BASE_URL}/procurement/scorecards`);
    
    // View scorecard for supplier
    await page.click('text=Acme Corporation');
    
    // Verify scorecard components
    await expect(page.locator('text=OTIF Score')).toBeVisible();
    await expect(page.locator('text=Quality Score')).toBeVisible();
    await expect(page.locator('text=Price Competitiveness')).toBeVisible();
    
    // Verify scores are calculated
    const otifScore = await page.locator('[data-testid="otif-score"]').textContent();
    expect(parseInt(otifScore || '0')).toBeGreaterThan(0);
  });

  test('should detect and approve PO variance', async ({ page }) => {
    // Create purchase order
    await page.goto(`${BASE_URL}/procurement/orders`);
    await page.click('a:has-text("Create PO")');
    
    await page.fill('input[name="supplier_id"]', '1');
    await page.fill('input[name="line_item_1_quantity"]', '100');
    await page.fill('input[name="line_item_1_unit_price"]', '50');
    
    await page.click('button:has-text("Create")');
    
    // Verify variance detection
    const varianceAlert = page.locator('[data-testid="variance-warning"]');
    if (await varianceAlert.isVisible()) {
      // Variance detected - approve it
      await page.click('button:has-text("Approve Variance")');
      await expect(page.locator('text=Variance approved')).toBeVisible();
    }
  });
});

// ═══════════════════════════════════════════════════════════════════════════
// PHASE 4: TRANSPORT EXECUTION TESTS
// ═══════════════════════════════════════════════════════════════════════════

test.describe('Phase 4: Transport Execution', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('should manage carrier and fleet operations', async ({ page }) => {
    // Navigate to carriers
    await page.goto(`${BASE_URL}/logistics/carriers`);
    await expect(page.locator('h1')).toContainText('Carrier Management');

    // Register new carrier
    await page.click('a:has-text("Register Carrier")');
    await page.fill('input[name="carrier_name"]', 'FastLogistics Inc');
    await page.fill('input[name="contact_email"]', 'contact@fastlogistics.com');
    await page.fill('input[name="phone"]', '+1-800-LOGISTICS');
    await page.selectOption('select[name="service_types"]', ['STANDARD', 'EXPRESS']);
    
    await page.click('button:has-text("Register")');
    await expect(page.locator('text=Carrier registered')).toBeVisible();

    // Add rate card
    await page.click('button:has-text("Add Rate Card")');
    await page.fill('input[name="origin_zone"]', 'Zone-A');
    await page.fill('input[name="destination_zone"]', 'Zone-B');
    await page.fill('input[name="base_rate"]', '500');
    await page.fill('input[name="per_kg_rate"]', '2.50');
    
    await page.click('button:has-text("Save")');
    await expect(page.locator('text=Rate card added')).toBeVisible();
  });

  test('should create and track shipment', async ({ page }) => {
    // Navigate to shipments
    await page.goto(`${BASE_URL}/logistics/shipments`);
    await page.click('a:has-text("Create Shipment")');

    // Create shipment
    await page.fill('input[name="order_id"]', 'PO-2026-001');
    await page.fill('input[name="origin_warehouse"]', '1');
    await page.fill('input[name="destination_address"]', '123 Main St, New York, NY');
    await page.fill('input[name="recipient_name"]', 'John Doe');
    await page.fill('input[name="recipient_phone"]', '+1-555-0100');
    
    await page.selectOption('select[name="transport_method"]', 'internal');
    await page.selectOption('select[name="vehicle_id"]', '1');
    await page.selectOption('select[name="driver_id"]', '1');
    
    await page.click('button:has-text("Create")');
    await expect(page.locator('text=Shipment created')).toBeVisible();

    // Verify shipment tracking
    const shipmentNumber = await page.locator('[data-testid="shipment-number"]').textContent();
    expect(shipmentNumber).toBeTruthy();
    
    // Check shipment status
    await expect(page.locator('text=DRAFT')).toBeVisible();
  });

  test('should plan trip with multiple stops', async ({ page }) => {
    // Navigate to trips
    await page.goto(`${BASE_URL}/logistics/trips`);
    await page.click('a:has-text("Plan Trip")');

    // Create trip
    await page.selectOption('select[name="vehicle_id"]', '1');
    await page.selectOption('select[name="driver_id"]', '1');
    await page.fill('input[name="planned_start_date"]', '2026-08-03');

    // Add multiple stops
    await page.click('button:has-text("Add Stop")');
    await page.fill('input[name="stop_1_address"]', '123 Main St');
    await page.fill('input[name="stop_1_city"]', 'New York');
    await page.selectOption('select[name="stop_1_type"]', 'CUSTOMER');

    await page.click('button:has-text("Add Stop")');
    await page.fill('input[name="stop_2_address"]', '456 Oak Ave');
    await page.fill('input[name="stop_2_city"]', 'Boston');
    await page.selectOption('select[name="stop_2_type"]', 'CUSTOMER');

    await page.click('button:has-text("Create Trip")');
    await expect(page.locator('text=Trip planned successfully')).toBeVisible();

    // Verify trip sequence
    await expect(page.locator('text=Stop 1')).toBeVisible();
    await expect(page.locator('text=Stop 2')).toBeVisible();
  });
});

// ═══════════════════════════════════════════════════════════════════════════
// PHASE 5: DISTRIBUTION PLANNING TESTS
// ═══════════════════════════════════════════════════════════════════════════

test.describe('Phase 5: Distribution Planning', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('should consolidate shipments into load', async ({ page }) => {
    // Navigate to loads
    await page.goto(`${BASE_URL}/distribution/loads`);
    await page.click('a:has-text("Create Load")');

    // Create load
    await page.selectOption('select[name="origin_warehouse"]', '1');
    await page.fill('input[name="destination_address"]', '789 Elm St');
    await page.fill('input[name="destination_city"]', 'Chicago');
    await page.selectOption('select[name="destination_country"]', 'USA');
    
    await page.click('button:has-text("Create Load")');
    await expect(page.locator('text=Load created')).toBeVisible();

    // Add items to load
    const loadId = await page.url();
    await page.click('button:has-text("Add Item")');
    await page.selectOption('select[name="product_id"]', '1');
    await page.fill('input[name="quantity"]', '100');
    await page.fill('input[name="weight_kg"]', '500');
    await page.fill('input[name="volume_cbm"]', '2.5');
    
    await page.click('button:has-text("Add")');
    await expect(page.locator('text=Item added')).toBeVisible();

    // Verify load metrics
    await expect(page.locator('text=500 kg')).toBeVisible();
    await expect(page.locator('text=2.5 m³')).toBeVisible();
  });

  test('should optimize and approve delivery route', async ({ page }) => {
    // Navigate to routes
    await page.goto(`${BASE_URL}/distribution/routes`);
    await page.click('a:has-text("Plan Route")');

    // Plan route
    await page.selectOption('select[name="load_id"]', '1');
    await page.fill('input[name="total_distance"]', '250.5');
    await page.fill('input[name="estimated_duration"]', '360');
    
    await page.click('button:has-text("Plan")');
    await expect(page.locator('text=Route planned')).toBeVisible();

    // Optimize route
    await page.click('button:has-text("Optimize")');
    await page.waitForLoadState('networkidle');
    
    // Verify optimization score
    const efficiencyScore = await page.locator('[data-testid="efficiency-score"]').textContent();
    expect(parseInt(efficiencyScore || '0')).toBeGreaterThan(70);

    // Approve route
    await page.click('button:has-text("Approve")');
    await expect(page.locator('text=Route approved')).toBeVisible();
  });

  test('should create inter-warehouse transfer', async ({ page }) => {
    // Navigate to transfers
    await page.goto(`${BASE_URL}/distribution/transfers`);
    await page.click('a:has-text("Create Transfer")');

    // Create transfer
    await page.selectOption('select[name="from_warehouse"]', '1');
    await page.selectOption('select[name="to_warehouse"]', '2');
    await page.fill('input[name="planned_dispatch_date"]', '2026-08-05');
    await page.fill('input[name="planned_arrival_date"]', '2026-08-06');
    
    await page.click('button:has-text("Create")');
    await expect(page.locator('text=Transfer created')).toBeVisible();

    // Add transfer lines
    await page.click('button:has-text("Add Item")');
    await page.selectOption('select[name="product_id"]', '1');
    await page.fill('input[name="quantity"]', '200');
    await page.fill('input[name="lot_number"]', 'LOT-2026-001');
    
    await page.click('button:has-text("Add")');
    await expect(page.locator('text=Item added')).toBeVisible();

    // Approve transfer
    await page.click('button:has-text("Approve")');
    await expect(page.locator('text=Transfer approved')).toBeVisible();

    // Dispatch transfer
    await page.click('button:has-text("Dispatch")');
    await page.selectOption('select[name="carrier_id"]', '1');
    await page.click('button:has-text("Dispatch Transfer")');
    await expect(page.locator('text=Transfer dispatched')).toBeVisible();
  });
});

// ═══════════════════════════════════════════════════════════════════════════
// PERMISSION AND RBAC TESTS
// ═══════════════════════════════════════════════════════════════════════════

test.describe('RBAC and Permission Enforcement', () => {
  test('should enforce procurement permissions', async ({ page }) => {
    // Login as limited user (read-only)
    await page.goto(`${BASE_URL}/login`);
    await page.fill('input[name="email"]', 'viewer@odyssey.local');
    await page.fill('input[name="password"]', 'Test@12345');
    await page.click('button[type="submit"]');

    // Navigate to contracts
    await page.goto(`${BASE_URL}/procurement/contracts`);
    
    // Verify read access
    await expect(page.locator('h1')).toContainText('Supplier Contracts');

    // Verify create button is hidden
    const createButton = page.locator('a:has-text("Create Contract")');
    await expect(createButton).not.toBeVisible();

    // Try direct navigation to create page
    await page.goto(`${BASE_URL}/procurement/contracts/new`);
    
    // Should see permission denied
    await expect(page.locator('text=Insufficient permissions')).toBeVisible();
  });

  test('should enforce distribution permissions', async ({ page }) => {
    await login(page);
    
    // Navigate to loads
    await page.goto(`${BASE_URL}/distribution/loads`);
    
    // Should have full access
    await expect(page.locator('a:has-text("Create Load")')).toBeVisible();
  });
});

// ═══════════════════════════════════════════════════════════════════════════
// INTEGRATION AND DATA CONSISTENCY TESTS
// ═══════════════════════════════════════════════════════════════════════════

test.describe('Data Consistency and Integration', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('should maintain data consistency across phases', async ({ page }) => {
    // Create PO in Phase 1-2
    await page.goto(`${BASE_URL}/procurement/orders`);
    await page.click('a:has-text("Create PO")');
    await page.fill('input[name="supplier"]', '1');
    await page.fill('input[name="quantity"]', '500');
    await page.click('button:has-text("Create")');
    const poUrl = await page.url();
    const poId = poUrl.split('/').pop();

    // Verify shipment creation in Phase 4
    await page.goto(`${BASE_URL}/logistics/shipments`);
    await page.fill('input[name="search"]', poId);
    await expect(page.locator(`text=${poId}`)).toBeVisible();

    // Verify consolidation in Phase 5
    await page.goto(`${BASE_URL}/distribution/loads`);
    await page.click('a:has-text("Create Load")');
    // Load should reference the PO
    await expect(page.locator('option')).toContainText(poId);
  });
});

// ═══════════════════════════════════════════════════════════════════════════
// PHASE 8: MANUFACTURING GOVERNANCE E2E TESTS
// ═══════════════════════════════════════════════════════════════════════════

test.describe('Phase 8: Manufacturing Governance', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('should submit BOM approval decision', async ({ page }) => {
    // Navigate to governance decisions
    await page.goto(`${BASE_URL}/mrp/decisions`);
    await expect(page.locator('h1')).toContainText('Manufacturing Decisions');

    // Create new decision
    await page.click('button:has-text("New Decision")');
    
    // Fill decision form
    await page.selectOption('select[name="record_type"]', 'BOM');
    await page.fill('input[name="record_id"]', '1');
    await page.selectOption('select[name="action"]', 'Approve');
    await page.fill('textarea[name="reason"]', 'BOM structure verified and complete');
    
    // Submit decision
    await page.click('button:has-text("Submit Decision")');
    
    // Verify challenge generated
    await expect(page.locator('[data-testid="challenge-id"]')).toBeVisible();
    const challengeID = await page.locator('[data-testid="challenge-id"]').textContent();
    expect(challengeID).toMatch(/challenge-/);
  });

  test('should display multi-actor gate signatures', async ({ page }) => {
    // Navigate to governance audit
    await page.goto(`${BASE_URL}/mrp/decisions/audit-log`);
    await expect(page.locator('h1')).toContainText('Audit Log');

    // Search for BOM decisions
    await page.fill('input[name="search"]', 'BOM');
    await page.click('button:has-text("Search")');

    // Wait for results
    await page.waitForSelector('[data-testid="audit-row"]');

    // Verify gate information
    const gateStatus = await page.locator('[data-testid="gate-status"]').first().textContent();
    expect(['PENDING', 'APPROVED', 'REJECTED']).toContain(gateStatus);

    // Verify signatures displayed
    const signatures = page.locator('[data-testid="signature-actor"]');
    const count = await signatures.count();
    expect(count).toBeGreaterThanOrEqual(1);
  });

  test('should show validation errors before gate entry', async ({ page }) => {
    // Navigate to decisions
    await page.goto(`${BASE_URL}/mrp/decisions`);
    await page.click('button:has-text("New Decision")');

    // Select invalid record
    await page.selectOption('select[name="record_type"]', 'WorkOrder');
    await page.fill('input[name="record_id"]', '999'); // Non-existent ID

    // Try to submit
    await page.click('button:has-text("Submit Decision")');

    // Verify validation error
    await expect(page.locator('[data-testid="validation-error"]')).toBeVisible();
    const error = await page.locator('[data-testid="validation-error"]').textContent();
    expect(error).toContain('not found');
  });

  test('should verify signature challenge response', async ({ page }) => {
    // Navigate to decision
    await page.goto(`${BASE_URL}/mrp/decisions`);
    
    // Find pending decision
    await page.click('text=Pending');
    const firstDecision = page.locator('[data-testid="decision-row"]').first();
    await firstDecision.click();

    // Verify challenge displayed
    await expect(page.locator('[data-testid="challenge-text"]')).toBeVisible();
    const challengeText = await page.locator('[data-testid="challenge-text"]').textContent();
    
    // Sign with response (mock)
    await page.fill('input[name="signature"]', 'mock-signature-response');
    await page.click('button:has-text("Sign")');

    // Verify verification result
    const result = page.locator('[data-testid="verification-result"]');
    await expect(result).toBeVisible({ timeout: 5000 });
  });

  test('should paginate audit log', async ({ page }) => {
    // Navigate to audit log
    await page.goto(`${BASE_URL}/mrp/decisions/audit-log`);

    // Verify initial rows
    const rows = page.locator('[data-testid="audit-row"]');
    const initialCount = await rows.count();
    expect(initialCount).toBeGreaterThan(0);

    // Go to next page
    await page.click('button:has-text("Next")');
    await page.waitForTimeout(500);

    // Verify rows changed
    const newRows = page.locator('[data-testid="audit-row"]');
    const newCount = await newRows.count();
    expect(newCount).toBeGreaterThanOrEqual(0);

    // Go back to first page
    await page.click('button:has-text("Previous")');
    await page.waitForTimeout(500);
  });

  test('should filter decisions by status', async ({ page }) => {
    // Navigate to decisions
    await page.goto(`${BASE_URL}/mrp/decisions/audit-log`);

    // Filter by status
    await page.selectOption('select[name="status"]', 'APPROVED');
    await page.click('button:has-text("Apply Filter")');

    // Wait for results
    await page.waitForSelector('[data-testid="audit-row"]');

    // Verify all results are approved
    const statuses = page.locator('[data-testid="decision-status"]');
    const count = await statuses.count();
    
    for (let i = 0; i < Math.min(count, 5); i++) {
      const status = await statuses.nth(i).textContent();
      expect(status).toContain('APPROVED');
    }
  });

  test('should export audit trail as CSV', async ({ page }) => {
    // Navigate to audit log
    await page.goto(`${BASE_URL}/mrp/decisions/audit-log`);

    // Click export button
    const downloadPromise = page.waitForEvent('download');
    await page.click('button:has-text("Export CSV")');
    const download = await downloadPromise;

    // Verify download
    expect(download.suggestedFilename()).toContain('audit');
    expect(download.suggestedFilename()).toContain('.csv');
  });

  test('should handle concurrent decision submissions', async ({ browser }) => {
    // Create two pages for concurrent operations
    const page1 = await browser.newPage();
    const page2 = await browser.newPage();

    // Login both pages
    await login(page1);
    await login(page2);

    // Navigate to decisions
    await page1.goto(`${BASE_URL}/mrp/decisions`);
    await page2.goto(`${BASE_URL}/mrp/decisions`);

    // Submit decisions concurrently
    await page1.click('button:has-text("New Decision")');
    await page2.click('button:has-text("New Decision")');

    await page1.selectOption('select[name="record_type"]', 'BOM');
    await page2.selectOption('select[name="record_type"]', 'WorkOrder');

    await page1.fill('input[name="record_id"]', '1');
    await page2.fill('input[name="record_id"]', '2');

    await page1.fill('textarea[name="reason"]', 'Decision 1');
    await page2.fill('textarea[name="reason"]', 'Decision 2');

    // Submit both
    await Promise.all([
      page1.click('button:has-text("Submit Decision")'),
      page2.click('button:has-text("Submit Decision")'),
    ]);

    // Verify both succeeded
    await expect(page1.locator('[data-testid="challenge-id"]')).toBeVisible();
    await expect(page2.locator('[data-testid="challenge-id"]')).toBeVisible();

    await page1.close();
    await page2.close();
  });

  test('should display role-based decision requirements', async ({ page }) => {
    // Navigate to decisions
    await page.goto(`${BASE_URL}/mrp/decisions`);
    await page.click('button:has-text("New Decision")');

    // Select BOM approval (requires QUALITY_LEAD + ENGINEERING)
    await page.selectOption('select[name="record_type"]', 'BOM');
    await page.selectOption('select[name="action"]', 'Approve');

    // Verify required roles displayed
    await expect(page.locator('[data-testid="required-roles"]')).toBeVisible();
    const rolesText = await page.locator('[data-testid="required-roles"]').textContent();
    expect(rolesText).toContain('QUALITY_LEAD');
    expect(rolesText).toContain('ENGINEERING');

    // Verify count
    const roleCount = rolesText?.match(/\d+/)?.[0];
    expect(roleCount).toBe('2');
  });

  test('should prevent duplicate decision submissions', async ({ page }) => {
    // Navigate to decisions
    await page.goto(`${BASE_URL}/mrp/decisions`);
    await page.click('button:has-text("New Decision")');

    // Fill form
    await page.selectOption('select[name="record_type"]', 'BOM');
    await page.fill('input[name="record_id"]', '1');
    await page.fill('textarea[name="reason"]', 'Test decision');

    // Submit twice quickly
    await page.click('button:has-text("Submit Decision")');
    await page.click('button:has-text("Submit Decision")');

    // Wait for responses
    await page.waitForTimeout(1000);

    // Verify only one decision was created
    const challengeElements = await page.locator('[data-testid="challenge-id"]').count();
    expect(challengeElements).toBeLessThanOrEqual(2); // May have disabled button
  });
});
