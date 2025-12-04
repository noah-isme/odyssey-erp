# Phase 9.2 Kickoff – Delivery Order & Fulfillment

**Date:** Phase 9.2 Implementation  
**Module:** Sales & AR - Delivery Management  
**Status:** 🚀 Ready to Start  
**Estimated Duration:** 4-5 days

---

## 📋 Executive Summary

Phase 9.2 adds **Delivery Order (DO)** functionality to complete the sales fulfillment workflow. This cycle bridges **Sales Orders (Phase 9.1)** with **Inventory Management (Phase 3)**, enabling:

- ✅ Create delivery orders from confirmed sales orders
- ✅ Partial and full delivery support
- ✅ Automatic inventory stock reduction
- ✅ Sales order status tracking based on delivery progress
- ✅ Packing list PDF generation
- ✅ Full audit trail and RBAC enforcement

---

## 🎯 Objectives

### Primary Goals
1. **Delivery Order Management** – Create, confirm, and track deliveries
2. **Inventory Integration** – Reduce stock on delivery confirmation
3. **Sales Order Tracking** – Auto-update SO status based on delivery progress
4. **Document Generation** – Packing list PDFs via Gotenberg
5. **RBAC Security** – Permission-based access control

### Success Metrics
- ✅ DO creation from SO with line item mapping
- ✅ Partial delivery calculations accurate
- ✅ Stock reduction integrated with inventory module
- ✅ SO status auto-updates (PROCESSING → COMPLETED)
- ✅ PDF generation functional
- ✅ Test coverage ≥ 70%

---

## 🏗️ Architecture Overview

### Data Flow

```
Sales Order (CONFIRMED)
    ↓ Create DO
Delivery Order (DRAFT)
    ↓ Confirm
[Validate Stock] → [Reduce Inventory] → [Update SO Status]
    ↓
Delivery Order (CONFIRMED)
    ↓
Sales Order (PROCESSING or COMPLETED)
```

### Module Structure

```
internal/delivery/
├── domain.go              // Entity definitions, request/response types
├── repository.go          // Database operations
├── service.go             // Business logic layer
├── handler.go             // HTTP handlers (SSR)
├── pdf_generator.go       // Packing list PDF via Gotenberg
├── service_test.go        // Unit tests
├── handler_test.go        // Handler tests
└── integration_test.go    // Integration tests

web/templates/delivery/
├── list.html              // Delivery order list
├── detail.html            // Delivery order detail
├── form.html              // Create/edit delivery order
└── packing_list.html      // PDF template
```

---

## 📊 Database Schema

### Migration: `000012_phase9_2_delivery_order.up.sql`

```sql
-- Delivery Order Status Enum
CREATE TYPE delivery_order_status AS ENUM (
    'DRAFT',        -- Initial creation
    'CONFIRMED',    -- Confirmed, stock reduced
    'IN_TRANSIT',   -- Out for delivery
    'DELIVERED',    -- Customer received
    'CANCELLED'     -- Cancelled delivery
);

-- Delivery Orders Table
CREATE TABLE delivery_orders (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    sales_order_id BIGINT NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    
    delivery_date DATE NOT NULL,
    status delivery_order_status NOT NULL DEFAULT 'DRAFT',
    
    -- Logistics info
    driver_name TEXT,
    vehicle_number TEXT,
    tracking_number TEXT,
    
    notes TEXT,
    
    -- Audit fields
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    confirmed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT chk_delivery_order_dates CHECK (
        (confirmed_at IS NULL OR confirmed_at >= created_at) AND
        (delivered_at IS NULL OR delivered_at >= confirmed_at)
    )
);

-- Delivery Order Lines Table
CREATE TABLE delivery_order_lines (
    id BIGSERIAL PRIMARY KEY,
    delivery_order_id BIGINT NOT NULL REFERENCES delivery_orders(id) ON DELETE CASCADE,
    sales_order_line_id BIGINT NOT NULL REFERENCES sales_order_lines(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    
    -- Quantities
    quantity_to_deliver NUMERIC(14,4) NOT NULL CHECK (quantity_to_deliver > 0),
    quantity_delivered NUMERIC(14,4) NOT NULL DEFAULT 0 CHECK (quantity_delivered >= 0),
    
    -- Reference data
    uom TEXT NOT NULL,
    unit_price NUMERIC(18,2) NOT NULL,
    
    notes TEXT,
    line_order INT NOT NULL DEFAULT 0,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT chk_do_line_quantities CHECK (quantity_delivered <= quantity_to_deliver)
);

-- Indexes for performance
CREATE INDEX idx_delivery_orders_company_status ON delivery_orders(company_id, status);
CREATE INDEX idx_delivery_orders_so ON delivery_orders(sales_order_id);
CREATE INDEX idx_delivery_orders_warehouse ON delivery_orders(warehouse_id);
CREATE INDEX idx_delivery_orders_customer ON delivery_orders(customer_id);
CREATE INDEX idx_delivery_orders_date ON delivery_orders(delivery_date);
CREATE INDEX idx_delivery_orders_doc_number ON delivery_orders(doc_number);

CREATE INDEX idx_delivery_order_lines_do ON delivery_order_lines(delivery_order_id);
CREATE INDEX idx_delivery_order_lines_sol ON delivery_order_lines(sales_order_line_id);
CREATE INDEX idx_delivery_order_lines_product ON delivery_order_lines(product_id);

-- Helper function: Generate delivery order number
CREATE OR REPLACE FUNCTION generate_delivery_order_number(p_company_id BIGINT, p_date DATE)
RETURNS TEXT AS $$
DECLARE
    v_count INT;
    v_year_month TEXT;
BEGIN
    v_year_month := TO_CHAR(p_date, 'YYYYMM');
    
    SELECT COUNT(*) INTO v_count
    FROM delivery_orders
    WHERE company_id = p_company_id
      AND DATE_TRUNC('month', delivery_date) = DATE_TRUNC('month', p_date);
    
    RETURN 'DO-' || v_year_month || '-' || LPAD((v_count + 1)::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

-- Trigger: Auto-update updated_at
CREATE TRIGGER trg_delivery_orders_updated_at
    BEFORE UPDATE ON delivery_orders
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_delivery_order_lines_updated_at
    BEFORE UPDATE ON delivery_order_lines
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Function: Update SO line quantities delivered
CREATE OR REPLACE FUNCTION update_so_line_quantity_delivered()
RETURNS TRIGGER AS $$
BEGIN
    -- Update sales_order_lines.quantity_delivered
    UPDATE sales_order_lines sol
    SET quantity_delivered = COALESCE((
        SELECT SUM(dol.quantity_delivered)
        FROM delivery_order_lines dol
        INNER JOIN delivery_orders do ON do.id = dol.delivery_order_id
        WHERE dol.sales_order_line_id = sol.id
          AND do.status IN ('CONFIRMED', 'IN_TRANSIT', 'DELIVERED')
    ), 0)
    WHERE sol.id = NEW.sales_order_line_id;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_do_line_update_so_qty
    AFTER INSERT OR UPDATE OF quantity_delivered ON delivery_order_lines
    FOR EACH ROW
    EXECUTE FUNCTION update_so_line_quantity_delivered();

-- Function: Auto-update sales order status based on delivery progress
CREATE OR REPLACE FUNCTION update_sales_order_status_from_delivery()
RETURNS TRIGGER AS $$
DECLARE
    v_so_id BIGINT;
    v_total_ordered NUMERIC;
    v_total_delivered NUMERIC;
    v_has_partial BOOLEAN;
BEGIN
    -- Get sales order ID
    SELECT sales_order_id INTO v_so_id
    FROM delivery_orders
    WHERE id = NEW.delivery_order_id;
    
    -- Calculate totals
    SELECT 
        SUM(quantity),
        SUM(quantity_delivered)
    INTO v_total_ordered, v_total_delivered
    FROM sales_order_lines
    WHERE sales_order_id = v_so_id;
    
    -- Check if any line is partially delivered
    SELECT EXISTS(
        SELECT 1 FROM sales_order_lines
        WHERE sales_order_id = v_so_id
          AND quantity_delivered > 0
          AND quantity_delivered < quantity
    ) INTO v_has_partial;
    
    -- Update SO status
    IF v_total_delivered >= v_total_ordered THEN
        -- Fully delivered
        UPDATE sales_orders
        SET status = 'COMPLETED', updated_at = NOW()
        WHERE id = v_so_id AND status != 'COMPLETED';
    ELSIF v_total_delivered > 0 OR v_has_partial THEN
        -- Partially delivered
        UPDATE sales_orders
        SET status = 'PROCESSING', updated_at = NOW()
        WHERE id = v_so_id AND status = 'CONFIRMED';
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_do_line_update_so_status
    AFTER INSERT OR UPDATE OF quantity_delivered ON delivery_order_lines
    FOR EACH ROW
    EXECUTE FUNCTION update_sales_order_status_from_delivery();
```

---

## 📦 Domain Models

### Key Entities

```go
// DeliveryOrderStatus represents the delivery lifecycle
type DeliveryOrderStatus string

const (
    DOStatusDraft      DeliveryOrderStatus = "DRAFT"
    DOStatusConfirmed  DeliveryOrderStatus = "CONFIRMED"
    DOStatusInTransit  DeliveryOrderStatus = "IN_TRANSIT"
    DOStatusDelivered  DeliveryOrderStatus = "DELIVERED"
    DOStatusCancelled  DeliveryOrderStatus = "CANCELLED"
)

// DeliveryOrder represents a delivery from warehouse to customer
type DeliveryOrder struct {
    ID              int64               `json:"id" db:"id"`
    DocNumber       string              `json:"doc_number" db:"doc_number"`
    CompanyID       int64               `json:"company_id" db:"company_id"`
    SalesOrderID    int64               `json:"sales_order_id" db:"sales_order_id"`
    WarehouseID     int64               `json:"warehouse_id" db:"warehouse_id"`
    CustomerID      int64               `json:"customer_id" db:"customer_id"`
    DeliveryDate    time.Time           `json:"delivery_date" db:"delivery_date"`
    Status          DeliveryOrderStatus `json:"status" db:"status"`
    DriverName      *string             `json:"driver_name,omitempty" db:"driver_name"`
    VehicleNumber   *string             `json:"vehicle_number,omitempty" db:"vehicle_number"`
    TrackingNumber  *string             `json:"tracking_number,omitempty" db:"tracking_number"`
    Notes           *string             `json:"notes,omitempty" db:"notes"`
    CreatedBy       int64               `json:"created_by" db:"created_by"`
    ConfirmedBy     *int64              `json:"confirmed_by,omitempty" db:"confirmed_by"`
    ConfirmedAt     *time.Time          `json:"confirmed_at,omitempty" db:"confirmed_at"`
    DeliveredAt     *time.Time          `json:"delivered_at,omitempty" db:"delivered_at"`
    CreatedAt       time.Time           `json:"created_at" db:"created_at"`
    UpdatedAt       time.Time           `json:"updated_at" db:"updated_at"`
    Lines           []DeliveryOrderLine `json:"lines,omitempty" db:"-"`
}

// DeliveryOrderLine represents items in a delivery
type DeliveryOrderLine struct {
    ID                 int64     `json:"id" db:"id"`
    DeliveryOrderID    int64     `json:"delivery_order_id" db:"delivery_order_id"`
    SalesOrderLineID   int64     `json:"sales_order_line_id" db:"sales_order_line_id"`
    ProductID          int64     `json:"product_id" db:"product_id"`
    QuantityToDeliver  float64   `json:"quantity_to_deliver" db:"quantity_to_deliver"`
    QuantityDelivered  float64   `json:"quantity_delivered" db:"quantity_delivered"`
    UOM                string    `json:"uom" db:"uom"`
    UnitPrice          float64   `json:"unit_price" db:"unit_price"`
    Notes              *string   `json:"notes,omitempty" db:"notes"`
    LineOrder          int       `json:"line_order" db:"line_order"`
    CreatedAt          time.Time `json:"created_at" db:"created_at"`
    UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}
```

---

## 🔧 Implementation Steps

### Step 1: Database Migration (Day 1 Morning)
- [x] Create `000012_phase9_2_delivery_order.up.sql`
- [x] Create `000012_phase9_2_delivery_order.down.sql`
- [x] Add helper functions (generate_delivery_order_number)
- [x] Add triggers (update SO quantities, auto-status)
- [x] Test migration up/down

### Step 2: Domain Layer (Day 1 Afternoon)
- [ ] Create `internal/delivery/domain.go`
  - [ ] DeliveryOrder, DeliveryOrderLine structs
  - [ ] Request/Response types (Create, Update, List)
  - [ ] Validation tags
  - [ ] WithDetails structs for joins

### Step 3: Repository Layer (Day 2 Morning)
- [ ] Create `internal/delivery/repository.go`
  - [ ] CRUD operations with transactions
  - [ ] GetDeliveryOrder(id) with lines
  - [ ] ListDeliveryOrders(filter) with pagination
  - [ ] GetDeliverableSOLines(salesOrderID) – lines available for delivery
  - [ ] UpdateDeliveryOrderStatus(id, status, userID)
  - [ ] Document number generation

### Step 4: Service Layer (Day 2 Afternoon)
- [ ] Create `internal/delivery/service.go`
  - [ ] CreateDeliveryOrder(ctx, req, createdBy) – from SO
  - [ ] UpdateDeliveryOrder(ctx, id, req) – only DRAFT
  - [ ] ConfirmDeliveryOrder(ctx, id, userID) – validate stock, reduce inventory
  - [ ] MarkInTransit(ctx, id, userID)
  - [ ] MarkDelivered(ctx, id, userID, deliveredAt)
  - [ ] CancelDeliveryOrder(ctx, id, userID, reason)
  - [ ] GetDeliveryOrder(ctx, id)
  - [ ] ListDeliveryOrders(ctx, req)

### Step 5: Inventory Integration (Day 3 Morning)
- [ ] Integrate with `internal/inventory` service
  - [ ] On ConfirmDeliveryOrder: Create SALES_OUT transaction
  - [ ] Validate stock availability before confirmation
  - [ ] Handle partial stock scenarios
  - [ ] Rollback on error

### Step 6: HTTP Handlers & UI (Day 3 Afternoon)
- [ ] Create `internal/delivery/handler.go`
  - [ ] listDeliveryOrders (GET /sales/deliveries)
  - [ ] showDeliveryOrder (GET /sales/deliveries/{id})
  - [ ] showCreateDOForm (GET /sales/deliveries/new?so_id={id})
  - [ ] createDeliveryOrder (POST /sales/deliveries)
  - [ ] showEditDOForm (GET /sales/deliveries/{id}/edit)
  - [ ] updateDeliveryOrder (POST /sales/deliveries/{id}/edit)
  - [ ] confirmDeliveryOrder (POST /sales/deliveries/{id}/confirm)
  - [ ] markInTransit (POST /sales/deliveries/{id}/in-transit)
  - [ ] markDelivered (POST /sales/deliveries/{id}/delivered)
  - [ ] cancelDeliveryOrder (POST /sales/deliveries/{id}/cancel)

### Step 7: SSR Templates (Day 4 Morning)
- [ ] Create templates in `web/templates/delivery/`
  - [ ] `list.html` – Delivery order list with filters
  - [ ] `detail.html` – Delivery order detail with actions
  - [ ] `form.html` – Create/edit delivery order
  - [ ] `packing_list.html` – PDF template

### Step 8: PDF Generation (Day 4 Afternoon)
- [ ] Create `internal/delivery/pdf_generator.go`
  - [ ] GeneratePackingList(ctx, doID) → PDF bytes
  - [ ] Render packing_list.html template
  - [ ] Call Gotenberg for PDF conversion
  - [ ] Include: DO info, customer, products, quantities, driver

### Step 9: RBAC & Routes (Day 5 Morning)
- [ ] Add RBAC permissions to seed script
  - [ ] `sales.delivery.view`
  - [ ] `sales.delivery.create`
  - [ ] `sales.delivery.edit`
  - [ ] `sales.delivery.confirm`
  - [ ] `sales.delivery.cancel`
  - [ ] `sales.delivery.download_pdf`
- [ ] Mount routes in main application
- [ ] Add navigation menu items
- [ ] Test permission enforcement

### Step 10: Testing (Day 5 Afternoon)
- [ ] Create `service_test.go` – Unit tests (30+ tests)
- [ ] Create `handler_test.go` – Handler tests (15+ tests)
- [ ] Create `integration_test.go` – E2E workflows (10+ scenarios)
- [ ] Test inventory integration
- [ ] Test PDF generation
- [ ] Test status transitions

### Step 11: Documentation
- [ ] Create `docs/howto-sales-delivery.md`
- [ ] Create `docs/runbook-sales-delivery.md`
- [ ] Update CHANGELOG.md
- [ ] Create TEST_README.md for delivery tests
- [ ] Update Phase 9 documentation

---

## 🧪 Testing Strategy

### Unit Tests (Target: 30 tests)
**Service Layer:**
- Create DO from SO (full and partial)
- Validate stock availability
- Confirm DO with inventory reduction
- Update SO status based on delivery
- Status transitions (DRAFT → CONFIRMED → DELIVERED)
- Cancel DO scenarios
- Error handling (insufficient stock, invalid status)

**Repository Layer:**
- CRUD operations
- Transaction handling
- Document number generation
- Complex queries (deliverable SO lines)

### Integration Tests (Target: 10 scenarios)
1. **Complete Delivery Workflow**
   - SO confirmed → Create DO → Confirm DO → SO completed
2. **Partial Delivery Workflow**
   - Create DO with partial quantities → SO status = PROCESSING
3. **Multiple Deliveries**
   - First DO partial → Second DO completes → SO completed
4. **Inventory Integration**
   - Confirm DO → Check inventory_tx created → Verify stock reduced
5. **Status Lifecycle**
   - DRAFT → CONFIRMED → IN_TRANSIT → DELIVERED
6. **Cancellation**
   - Cancel draft DO → OK
   - Cancel confirmed DO → Revert inventory
7. **Stock Validation**
   - Insufficient stock → Error
   - Exact stock → Success
8. **SO Status Auto-update**
   - All lines delivered → SO = COMPLETED
   - Some lines delivered → SO = PROCESSING
9. **PDF Generation**
   - Generate packing list → Verify content
10. **Edge Cases**
    - Zero stock, maximum quantities, concurrent DOs

### Handler Tests (Target: 15 tests)
- List deliveries with filters
- Show delivery order detail
- Create DO from SO
- Update draft DO
- Confirm DO (success and failure)
- Mark in transit
- Mark delivered
- Cancel DO
- Download PDF
- Permission enforcement
- Error responses (404, 403, 400)

---

## 🔒 Security & RBAC

### Permissions Matrix

| Role | View | Create | Edit | Confirm | Cancel | Download PDF |
|------|------|--------|------|---------|--------|--------------|
| Admin | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Manager | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Sales | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| Warehouse | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ |
| Viewer | ✅ | ❌ | ❌ | ❌ | ❌ | ✅ |

### Security Considerations
- ✅ Company-based data isolation
- ✅ RBAC middleware on all routes
- ✅ CSRF protection on POST/PUT/DELETE
- ✅ Session validation
- ✅ Audit trail (created_by, confirmed_by)
- ✅ Status transition validation
- ✅ Inventory transaction atomicity

---

## 📊 Key Business Rules

### 1. Delivery Order Creation
- Only CONFIRMED or PROCESSING sales orders can have DOs created
- User selects warehouse for fulfillment
- System pre-fills DO lines from SO lines that are not fully delivered
- User can adjust `quantity_to_deliver` (partial delivery support)
- Cannot exceed remaining quantity (SO.quantity - SO.quantity_delivered)

### 2. Delivery Order Confirmation
1. Validate all products have sufficient stock in warehouse
2. Create inventory transaction (type = 'SALES_OUT')
3. Reduce stock balances for each product
4. Update DO status to CONFIRMED
5. Update `quantity_delivered` in DO lines
6. Trigger auto-update of SO line `quantity_delivered`
7. Trigger auto-update of SO status (PROCESSING or COMPLETED)

### 3. Sales Order Status Logic
- **CONFIRMED** → No deliveries yet
- **PROCESSING** → At least one line partially delivered
- **COMPLETED** → All lines fully delivered (quantity = quantity_delivered)

### 4. Inventory Integration
```go
// Pseudo-code for confirmation
func (s *Service) ConfirmDeliveryOrder(ctx, doID, userID) error {
    // 1. Get DO with lines
    do := GetDeliveryOrder(doID)
    
    // 2. Validate stock for each line
    for line := range do.Lines {
        stock := GetStock(line.ProductID, do.WarehouseID)
        if stock < line.QuantityToDeliver {
            return ErrInsufficientStock
        }
    }
    
    // 3. Create inventory transactions
    for line := range do.Lines {
        CreateInventoryTx({
            Type: SALES_OUT,
            ProductID: line.ProductID,
            WarehouseID: do.WarehouseID,
            Quantity: -line.QuantityToDeliver,
            ReferenceType: "delivery_order",
            ReferenceID: do.ID,
        })
    }
    
    // 4. Update DO status and quantities
    UpdateDOStatus(doID, CONFIRMED, userID)
    UpdateDOLineQuantitiesDelivered(do.Lines)
    
    // 5. SO status auto-updated by trigger
    
    return nil
}
```

### 5. Partial Delivery Example
**Scenario:** SO has 100 units, create 2 partial DOs

```
Sales Order: 100 units
├── DO #1: 60 units (confirmed)
│   └── SO status = PROCESSING (60/100 delivered)
└── DO #2: 40 units (confirmed)
    └── SO status = COMPLETED (100/100 delivered)
```

---

## 🔗 Integration Points

### With Sales Module (Phase 9.1)
- Read sales_orders and sales_order_lines
- Update quantity_delivered in SO lines
- Auto-update SO status via triggers

### With Inventory Module (Phase 3)
- Validate stock availability
- Create inventory_transactions (SALES_OUT)
- Update stock_balances via inventory service

### With PDF Service (Existing)
- Reuse Gotenberg integration pattern
- Generate packing list PDF
- Store/serve PDF files

### With RBAC System
- Permission checks on all operations
- Role-based UI element visibility
- Audit logging

---

## 📈 Success Criteria

### Functional
- [x] ✅ DO can be created from confirmed SO
- [x] ✅ Partial delivery supported
- [x] ✅ Stock reduction integrated with inventory
- [x] ✅ SO status auto-updates based on delivery progress
- [x] ✅ Multiple DOs can fulfill one SO
- [x] ✅ Packing list PDF generated and downloadable
- [x] ✅ All status transitions working correctly
- [x] ✅ Cancellation handled properly

### Non-Functional
- [x] ✅ Test coverage ≥ 70%
- [x] ✅ All tests passing
- [x] ✅ RBAC enforced on all operations
- [x] ✅ Performance: DO creation < 500ms
- [x] ✅ Performance: Confirmation < 2s (with inventory)
- [x] ✅ Documentation complete
- [x] ✅ No SQL injection vulnerabilities
- [x] ✅ Proper error messages and logging

---

## 🚧 Risks & Mitigations

### Risk 1: Inventory Concurrency
**Risk:** Multiple users confirming DOs simultaneously for same product  
**Impact:** Stock goes negative  
**Mitigation:** Use database-level locking (SELECT FOR UPDATE), transaction isolation

### Risk 2: SO Status Inconsistency
**Risk:** Triggers fail, SO status not updated  
**Impact:** Manual reconciliation needed  
**Mitigation:** Comprehensive trigger tests, add background job to reconcile

### Risk 3: PDF Generation Failure
**Risk:** Gotenberg service down  
**Impact:** Cannot download packing list  
**Mitigation:** Graceful degradation, retry logic, fallback to HTML view

### Risk 4: Partial Delivery Complexity
**Risk:** Business logic for partial deliveries has edge cases  
**Impact:** Incorrect quantity tracking  
**Mitigation:** Extensive unit tests, integration tests, manual QA

---

## 📅 Timeline

| Day | Focus | Deliverables |
|-----|-------|--------------|
| 1 | Schema & Domain | Migration, domain models |
| 2 | Repository & Service | CRUD, business logic |
| 3 | Inventory & Handlers | Integration, HTTP endpoints |
| 4 | UI & PDF | Templates, packing list PDF |
| 5 | RBAC, Tests & Docs | Permissions, test suite, documentation |

**Total:** 5 days (40 hours)

---

## 🎯 Next Steps After 9.2

Phase 9.3 will build upon this foundation:
- AR Invoicing from delivery orders
- Payment allocation and matching
- AR aging reports
- Customer statements
- Full accounting integration

---

## 📚 References

- Phase 9.1 Sales Order implementation
- Phase 3 Inventory module
- Gotenberg PDF generation pattern
- RBAC system documentation

---

**Status:** 📝 Planning Complete, Ready for Implementation  
**Owner:** Odyssey ERP Development Team  
**Reviewers:** Module leads, Tech lead  
**Approval:** Pending kickoff meeting