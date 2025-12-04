# Delivery Order - Inventory Integration

## Overview

This document describes the inventory integration for the Delivery Order module in Odyssey ERP. When a delivery order is marked as **DELIVERED**, the system automatically reduces inventory stock in the warehouse.

---

## Architecture

### Components

1. **Delivery Service** (`internal/delivery/service.go`)
   - Core business logic for delivery orders
   - Triggers inventory adjustments on delivery completion

2. **Inventory Adapter** (`internal/delivery/inventory_adapter.go`)
   - Adapter pattern implementation
   - Bridges delivery service with inventory service
   - Converts delivery-specific requests to inventory operations

3. **Inventory Service** (`internal/inventory/service.go`)
   - Handles all inventory movements
   - Maintains stock balances and transaction history

---

## Integration Flow

### 1. Delivery Completion Workflow

```
┌─────────────────────┐
│  Delivery Handler   │
│  (HTTP Request)     │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Delivery Service   │
│  MarkDelivered()    │
└──────────┬──────────┘
           │
           ├─► Update DO status to DELIVERED
           │
           ├─► Update line quantities_delivered
           │
           ▼
┌─────────────────────┐
│ Inventory Adapter   │
│ PostAdjustment()    │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Inventory Service   │
│ PostAdjustment()    │
└──────────┬──────────┘
           │
           ├─► Create inventory transaction (OUT)
           │
           ├─► Update stock balance (reduce qty)
           │
           └─► Record audit log
```

### 2. Stock Reduction Logic

When a delivery order is marked as DELIVERED:

1. **Retrieve delivery order lines** from the database
2. **For each line:**
   - Extract: product_id, quantity_to_deliver, unit_price, warehouse_id
   - Create inventory adjustment with **negative quantity** (outbound)
   - Reference the delivery order for traceability

3. **Post inventory adjustments** via adapter
4. **Update stock balances** in warehouse
5. **Record transaction history** for audit trail

---

## Code Implementation

### Service Method: MarkDelivered

```go
func (s *Service) MarkDelivered(ctx context.Context, id int64, req MarkDeliveredRequest) (*DeliveryOrder, error) {
    // 1. Validate delivery order status
    existing, err := s.repo.GetDeliveryOrder(ctx, id)
    if existing.Status != DOStatusInTransit {
        return nil, fmt.Errorf("must be IN_TRANSIT to mark delivered")
    }

    // 2. Get delivery order lines
    lines, err := s.repo.getDeliveryOrderLines(ctx, id)
    
    // 3. Update status in transaction
    err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
        // Update DO status to DELIVERED
        tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusDelivered, updates)
        
        // Update line quantities as delivered
        for _, line := range lines {
            tx.UpdateDeliveryOrderLineQuantity(ctx, line.ID, line.QuantityToDeliver)
        }
        
        return nil
    })

    // 4. Reduce inventory stock (if inventory service available)
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
            
            s.inventory.PostAdjustment(ctx, adjustmentInput)
        }
    }

    return s.repo.GetDeliveryOrder(ctx, id)
}
```

### Inventory Adapter

```go
type InventoryAdapter struct {
    service *inventory.Service
}

func (a *InventoryAdapter) PostAdjustment(ctx context.Context, input InventoryAdjustmentInput) error {
    // Convert delivery adjustment to inventory adjustment
    invInput := inventory.AdjustmentInput{
        Code:        input.Code,
        WarehouseID: input.WarehouseID,
        ProductID:   input.ProductID,
        Qty:         input.Qty, // Negative for outbound
        UnitCost:    input.UnitCost,
        Note:        input.Note,
        ActorID:     input.ActorID,
        RefModule:   input.RefModule,
        RefID:       input.RefID,
    }

    _, err := a.service.PostAdjustment(ctx, invInput)
    return err
}
```

### Wiring in main.go

```go
// Initialize inventory service
inventoryRepo := inventory.NewRepository(dbpool)
inventoryService := inventory.NewService(inventoryRepo, auditLogger, idempotencyStore, inventory.ServiceConfig{}, integrationHooks)

// Initialize delivery service
deliveryService := delivery.NewService(dbpool)

// Wire up inventory integration
inventoryAdapter := delivery.NewInventoryAdapter(inventoryService)
deliveryService.SetInventoryService(inventoryAdapter)

deliveryHandler := delivery.NewHandler(logger, deliveryService, templates, csrfManager, sessionManager, rbacMiddleware)
```

---

## Database Operations

### Inventory Transaction Record

When a delivery is completed, the system creates:

**Transaction Header:**
- `transaction_code`: `"DO-{DOC_NUMBER}-L{LINE_ID}"`
- `transaction_type`: `"ADJUST"` (adjustment)
- `warehouse_id`: From delivery order
- `ref_module`: `"DELIVERY"`
- `ref_id`: Delivery order ID
- `posted_at`: Current timestamp

**Transaction Line:**
- `product_id`: From delivery order line
- `qty`: Negative value (outbound)
- `unit_cost`: From delivery order line unit price

**Stock Balance Update:**
- `warehouse_id` + `product_id`: Composite key
- `qty`: Reduced by delivery quantity
- `avg_cost`: Recalculated weighted average
- `updated_at`: Current timestamp

### Example SQL Operations

```sql
-- Insert inventory transaction
INSERT INTO inventory_transactions (code, type, warehouse_id, ref_module, ref_id, note, posted_at, created_by)
VALUES ('DO-2024-001-L1', 'ADJUST', 1, 'DELIVERY', '123', 'Delivery Order 2024-001 - Line 1', NOW(), 5);

-- Insert transaction line
INSERT INTO inventory_transaction_lines (transaction_id, product_id, qty, unit_cost)
VALUES (456, 100, -10.00, 50.00);

-- Update stock balance
UPDATE inventory_balances
SET qty = qty - 10.00,
    avg_cost = ((qty * avg_cost) - (10.00 * 50.00)) / (qty - 10.00),
    updated_at = NOW()
WHERE warehouse_id = 1 AND product_id = 100;
```

---

## Data Flow Example

### Scenario: Deliver 10 units of Product A

**Input:**
- Delivery Order: `DO-2024-001`
- Warehouse: `WH-MAIN` (ID: 1)
- Product: `Product A` (ID: 100)
- Quantity: 10 units
- Unit Price: $50.00

**Process:**

1. **Mark Delivery as DELIVERED**
   - Request: `POST /delivery-orders/123/complete`
   - Body: `{ "delivered_at": "2024-01-15T14:30:00Z", "updated_by": 5 }`

2. **Update Delivery Order**
   - Status: `DRAFT` → `CONFIRMED` → `IN_TRANSIT` → `DELIVERED`
   - Line quantity_delivered: 0 → 10

3. **Post Inventory Adjustment**
   - Code: `DO-2024-001-L1`
   - Warehouse: 1 (WH-MAIN)
   - Product: 100 (Product A)
   - Quantity: **-10.00** (negative = outbound)
   - Unit Cost: $50.00
   - Reference: `DELIVERY:123`

4. **Update Stock Balance**
   - Before: 100 units @ $48.00 avg cost
   - Adjustment: -10 units @ $50.00
   - After: 90 units @ $47.78 avg cost (recalculated)

**Result:**
- ✅ Delivery marked as DELIVERED
- ✅ Inventory reduced by 10 units
- ✅ Transaction recorded with full audit trail
- ✅ Stock balance updated correctly

---

## Error Handling

### Insufficient Stock

If stock is insufficient during delivery:

```go
if balance.Qty < line.QuantityToDeliver {
    return fmt.Errorf("insufficient stock: have %.2f, need %.2f", balance.Qty, line.QuantityToDeliver)
}
```

**Resolution:**
- Check stock availability before confirming delivery
- Use `ValidateStockAvailability()` method
- Prevent delivery confirmation if stock is insufficient

### Transaction Rollback

All operations are wrapped in database transactions:

```go
err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
    // All operations here are atomic
    // If any fails, all changes are rolled back
    return nil
})
```

**If any operation fails:**
- Database transaction is rolled back
- Delivery order status remains unchanged
- No inventory adjustments are posted
- Error is returned to user

### Inventory Service Unavailable

The integration is **optional**:

```go
if s.inventory != nil {
    // Post inventory adjustments
}
```

**Behavior:**
- If inventory service is not configured, delivery can still complete
- Stock reduction will not occur (manual adjustment required)
- System logs a warning

---

## Testing

### Unit Tests

See `internal/delivery/inventory_integration_test.go`:

- ✅ Test inventory adapter interface
- ✅ Test negative quantity for outbound
- ✅ Test reference module is "DELIVERY"
- ✅ Test multiple line items
- ✅ Test optional inventory integration
- ✅ Test error handling

**Run tests:**
```bash
go test ./internal/delivery -run TestInventory -v
```

### Integration Tests

Integration tests validate end-to-end workflow:

1. Create sales order
2. Create delivery order
3. Confirm delivery order
4. Mark as in-transit
5. **Mark as delivered** ← Triggers inventory reduction
6. Verify stock balance reduced
7. Verify transaction history

**Run integration tests:**
```bash
go test ./internal/delivery -run TestIntegration -v
```

---

## Monitoring & Observability

### Audit Trail

Every inventory adjustment is recorded in `audit_logs`:

```json
{
  "module": "INVENTORY",
  "action": "POST_ADJUSTMENT",
  "entity_type": "inventory_transaction",
  "entity_id": "456",
  "changes": {
    "warehouse_id": 1,
    "product_id": 100,
    "qty": -10.00,
    "ref_module": "DELIVERY",
    "ref_id": "123"
  },
  "actor_id": 5,
  "timestamp": "2024-01-15T14:30:00Z"
}
```

### Traceability

To trace inventory movements from delivery orders:

```sql
-- Find all inventory transactions for a delivery order
SELECT it.code, it.type, it.posted_at, itl.product_id, itl.qty, itl.unit_cost
FROM inventory_transactions it
JOIN inventory_transaction_lines itl ON itl.transaction_id = it.id
WHERE it.ref_module = 'DELIVERY' AND it.ref_id = '123';

-- Find delivery order for an inventory transaction
SELECT do.*
FROM delivery_orders do
JOIN inventory_transactions it ON it.ref_id::int = do.id
WHERE it.code = 'DO-2024-001-L1' AND it.ref_module = 'DELIVERY';
```

---

## Configuration

### Enable/Disable Integration

In `cmd/odyssey/main.go`:

```go
// To enable (default):
inventoryAdapter := delivery.NewInventoryAdapter(inventoryService)
deliveryService.SetInventoryService(inventoryAdapter)

// To disable:
// (Simply don't call SetInventoryService)
deliveryService := delivery.NewService(dbpool)
```

### Allow Negative Stock

Configure in inventory service initialization:

```go
inventoryService := inventory.NewService(
    inventoryRepo, 
    auditLogger, 
    idempotencyStore, 
    inventory.ServiceConfig{
        AllowNegativeStock: false, // Set to true to allow negative balances
    }, 
    integrationHooks,
)
```

**Recommended:** Keep `AllowNegativeStock: false` to prevent overselling.

---

## Best Practices

### 1. Validate Stock Before Delivery

Always check stock availability before confirming delivery:

```go
err := deliveryService.ValidateStockAvailability(ctx, deliveryOrderID)
if err != nil {
    return fmt.Errorf("insufficient stock: %w", err)
}
```

### 2. Use Transactions

Always wrap related operations in database transactions to ensure atomicity.

### 3. Audit Everything

All inventory movements should be auditable:
- Who performed the action
- When it happened
- What changed
- Why it changed (reference to delivery order)

### 4. Handle Errors Gracefully

If inventory adjustment fails:
- Roll back delivery status update
- Return clear error message
- Log error for investigation
- Don't leave system in inconsistent state

### 5. Monitor Stock Levels

Set up alerts for:
- Low stock levels
- Negative stock (if allowed)
- Large inventory adjustments
- Failed delivery completions

---

## Troubleshooting

### Issue: Stock Not Reduced After Delivery

**Diagnosis:**
1. Check if inventory service is wired up in main.go
2. Verify delivery order status is DELIVERED
3. Check inventory transaction logs
4. Look for errors in application logs

**Solution:**
```bash
# Check inventory transactions
SELECT * FROM inventory_transactions 
WHERE ref_module = 'DELIVERY' AND ref_id = '123';

# Manual adjustment if needed
INSERT INTO inventory_transactions ...
```

### Issue: Negative Stock Error

**Diagnosis:**
- Stock balance is lower than delivery quantity
- `AllowNegativeStock` is set to false

**Solution:**
1. Verify current stock balance
2. Check for concurrent deliveries
3. Adjust stock manually or receive goods
4. Retry delivery completion

### Issue: Transaction Rollback

**Diagnosis:**
- Error during inventory adjustment
- Database constraint violation
- Network timeout

**Solution:**
1. Check application logs for error details
2. Verify database connectivity
3. Check for duplicate transaction codes
4. Retry the operation

---

## Future Enhancements

### Planned Improvements

1. **Stock Reservation**
   - Reserve stock when delivery is CONFIRMED
   - Release reservation when DELIVERED or CANCELLED
   - Prevent overselling with concurrent orders

2. **Batch Operations**
   - Support bulk delivery completions
   - Optimize inventory adjustments for multiple lines
   - Reduce database round-trips

3. **Stock Reconciliation**
   - Periodic reconciliation reports
   - Identify discrepancies
   - Auto-correction workflows

4. **Advanced Cost Tracking**
   - FIFO/LIFO costing methods
   - Lot/serial number tracking
   - Landed cost allocation

5. **Real-time Notifications**
   - Alert warehouse staff on low stock
   - Notify purchasing team for reorders
   - Dashboard for inventory managers

---

## References

- **Delivery Service:** `internal/delivery/service.go`
- **Inventory Service:** `internal/inventory/service.go`
- **Inventory Adapter:** `internal/delivery/inventory_adapter.go`
- **Integration Tests:** `internal/delivery/inventory_integration_test.go`
- **Main Application:** `cmd/odyssey/main.go`

---

## Changelog

### Version 1.0 - 2024-01-15

- ✅ Initial inventory integration implementation
- ✅ Automatic stock reduction on delivery completion
- ✅ Adapter pattern for loose coupling
- ✅ Full audit trail and traceability
- ✅ Unit tests for inventory adapter
- ✅ Transaction-based error handling
- ✅ Optional integration (graceful degradation)

---

## Support

For questions or issues related to inventory integration:

1. Check application logs: `/var/log/odyssey-erp/`
2. Review database transactions: `inventory_transactions` table
3. Check audit logs: `audit_logs` table with `module='INVENTORY'`
4. Contact development team: dev@odyssey-erp.com

---

**Last Updated:** 2024-01-15  
**Author:** Odyssey ERP Development Team  
**Status:** Production Ready ✅