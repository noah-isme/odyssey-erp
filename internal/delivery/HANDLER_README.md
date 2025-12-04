# Delivery Order HTTP Handlers Documentation

This document describes the HTTP handlers for the Delivery Order module in Odyssey ERP.

## Overview

The Delivery Order handlers provide a complete REST-like interface for managing delivery orders through server-side rendered HTML pages. All handlers follow SSR (Server-Side Rendering) patterns with HTMX support for progressive enhancement.

## Handler Structure

```go
type Handler struct {
    logger    *slog.Logger
    service   *Service
    templates *view.Engine
    csrf      *shared.CSRFManager
    sessions  *shared.SessionManager
    rbac      rbac.Middleware
}
```

## Routes

All routes are mounted under the `/delivery-orders` prefix with appropriate RBAC permissions.

### List & View Routes

| Method | Path | Handler | Permission | Description |
|--------|------|---------|-----------|-------------|
| GET | `/delivery-orders` | `listDeliveryOrders` | `delivery.order.view` | List all delivery orders with filtering |
| GET | `/delivery-orders/{id}` | `showDeliveryOrder` | `delivery.order.view` | View single delivery order details |

### Create Routes

| Method | Path | Handler | Permission | Description |
|--------|------|---------|-----------|-------------|
| GET | `/delivery-orders/new` | `showDeliveryOrderForm` | `delivery.order.create` | Display create form |
| POST | `/delivery-orders` | `createDeliveryOrder` | `delivery.order.create` | Create new delivery order |

### Edit Routes

| Method | Path | Handler | Permission | Description |
|--------|------|---------|-----------|-------------|
| GET | `/delivery-orders/{id}/edit` | `showEditDeliveryOrderForm` | `delivery.order.edit` | Display edit form |
| POST | `/delivery-orders/{id}/edit` | `updateDeliveryOrder` | `delivery.order.edit` | Update delivery order |

### Status Transition Routes

| Method | Path | Handler | Permission | Description |
|--------|------|---------|-----------|-------------|
| POST | `/delivery-orders/{id}/confirm` | `confirmDeliveryOrder` | `delivery.order.confirm` | Confirm delivery order (DRAFT → CONFIRMED) |
| POST | `/delivery-orders/{id}/ship` | `shipDeliveryOrder` | `delivery.order.ship` | Mark as shipped (CONFIRMED → IN_TRANSIT) |
| POST | `/delivery-orders/{id}/complete` | `completeDeliveryOrder` | `delivery.order.complete` | Mark as delivered (IN_TRANSIT → DELIVERED) |
| POST | `/delivery-orders/{id}/cancel` | `cancelDeliveryOrder` | `delivery.order.cancel` | Cancel delivery order |

### Integration Routes

| Method | Path | Handler | Permission | Description |
|--------|------|---------|-----------|-------------|
| GET | `/sales-orders/{id}/delivery-orders` | `listDeliveryOrdersBySalesOrder` | `delivery.order.view` or `sales.order.view` | List deliveries for a sales order |

## Handler Details

### 1. List Delivery Orders (`listDeliveryOrders`)

**Purpose**: Display paginated list of delivery orders with filtering capabilities.

**Query Parameters**:
- `page` (int): Page number (default: 1)
- `status` (string): Filter by status (draft, confirmed, in_transit, delivered, cancelled)
- `sales_order_id` (int64): Filter by sales order
- `warehouse_id` (int64): Filter by warehouse
- `search` (string): Search in document number, driver name
- `date_from` (date): Filter deliveries from date (format: YYYY-MM-DD)
- `date_to` (date): Filter deliveries to date (format: YYYY-MM-DD)

**Response Template Data**:
```go
{
    "DeliveryOrders": []DeliveryOrderWithDetails,
    "CurrentPage": int,
    "TotalPages": int,
    "TotalCount": int,
    "Request": ListDeliveryOrdersRequest,
}
```

**Template**: `delivery/list`

**Example**:
```
GET /delivery-orders?status=confirmed&page=1&warehouse_id=1
```

---

### 2. Show Delivery Order (`showDeliveryOrder`)

**Purpose**: Display detailed view of a single delivery order.

**URL Parameters**:
- `id` (int64): Delivery order ID

**Response Template Data**:
```go
{
    "DeliveryOrder": *DeliveryOrder,
}
```

**Template**: `delivery/detail`

**Error Responses**:
- 400 Bad Request: Invalid ID format
- 404 Not Found: Delivery order not found
- 500 Internal Server Error: Database error

---

### 3. Show Create Form (`showDeliveryOrderForm`)

**Purpose**: Display form to create a new delivery order.

**Query Parameters** (optional):
- `sales_order_id` (int64): Pre-populate from sales order

**Response Template Data**:
```go
{
    "SalesOrderID": string,
    "CSRFToken": string,
}
```

**Template**: `delivery/form`

---

### 4. Create Delivery Order (`createDeliveryOrder`)

**Purpose**: Process form submission to create a new delivery order.

**Form Fields**:
- `sales_order_id` (int64, required): Source sales order
- `warehouse_id` (int64, required): Fulfillment warehouse
- `delivery_date` (date, required): Scheduled delivery date (YYYY-MM-DD)
- `driver_name` (string, optional): Driver name
- `vehicle_number` (string, optional): Vehicle registration
- `tracking_number` (string, optional): Shipping tracking number
- `notes` (string, optional): Additional notes
- `so_line_id[]` ([]int64, required): Sales order line IDs
- `product_id[]` ([]int64, required): Product IDs (must match SO lines)
- `quantity[]` ([]float64, required): Quantities to deliver
- `line_notes[]` ([]string, optional): Line-level notes

**Success Response**:
- 303 See Other: Redirects to delivery order detail page
- Flash message: "Delivery order created successfully"

**Error Response**:
- 200 OK: Re-renders form with errors

**Validation Rules**:
- Sales order must exist and be in CONFIRMED or PROCESSING status
- Warehouse must exist
- All line items must belong to the sales order
- Quantities must not exceed remaining deliverable quantities
- At least one line item is required

---

### 5. Show Edit Form (`showEditDeliveryOrderForm`)

**Purpose**: Display form to edit an existing delivery order.

**URL Parameters**:
- `id` (int64): Delivery order ID

**Response Template Data**:
```go
{
    "DeliveryOrder": *DeliveryOrder,
    "CSRFToken": string,
}
```

**Template**: `delivery/edit`

**Business Rules**:
- Only DRAFT orders can be edited
- Returns 404 if order not found

---

### 6. Update Delivery Order (`updateDeliveryOrder`)

**Purpose**: Process form submission to update a delivery order.

**URL Parameters**:
- `id` (int64): Delivery order ID

**Form Fields** (all optional):
- `delivery_date` (date): New delivery date
- `driver_name` (string): Driver name
- `vehicle_number` (string): Vehicle registration
- `tracking_number` (string): Tracking number
- `notes` (string): Notes
- `so_line_id[]` ([]int64): Updated line items
- `product_id[]` ([]int64): Product IDs
- `quantity[]` ([]float64): Quantities

**Success Response**:
- 303 See Other: Redirects to delivery order detail page
- Flash message: "Delivery order updated successfully"

**Business Rules**:
- Only DRAFT orders can be updated
- If lines are provided, they replace existing lines
- Quantities must still respect SO line limits

---

### 7. Confirm Delivery Order (`confirmDeliveryOrder`)

**Purpose**: Transition delivery order from DRAFT to CONFIRMED status.

**URL Parameters**:
- `id` (int64): Delivery order ID

**Success Response**:
- 303 See Other: Redirects to delivery order detail page
- Flash message: "Delivery order confirmed successfully"

**Error Response**:
- 303 See Other: Redirects to detail page with error message

**Business Rules**:
- Only DRAFT orders can be confirmed
- Validates stock availability (if inventory integration enabled)
- Updates sales order quantities
- Records confirming user

---

### 8. Ship Delivery Order (`shipDeliveryOrder`)

**Purpose**: Mark delivery order as shipped (IN_TRANSIT).

**URL Parameters**:
- `id` (int64): Delivery order ID

**Form Fields** (optional):
- `tracking_number` (string): Shipping tracking number

**Success Response**:
- 303 See Other: Redirects to delivery order detail page
- Flash message: "Delivery order shipped successfully"

**Business Rules**:
- Only CONFIRMED orders can be shipped
- Optionally updates tracking number
- Reduces inventory stock (if integration enabled)

---

### 9. Complete Delivery Order (`completeDeliveryOrder`)

**Purpose**: Mark delivery order as delivered (DELIVERED).

**URL Parameters**:
- `id` (int64): Delivery order ID

**Form Fields** (optional):
- `delivered_at` (date): Actual delivery date (defaults to current date)

**Success Response**:
- 303 See Other: Redirects to delivery order detail page
- Flash message: "Delivery order completed successfully"

**Business Rules**:
- Only IN_TRANSIT orders can be marked delivered
- Records actual delivery timestamp
- Updates sales order status if fully delivered

---

### 10. Cancel Delivery Order (`cancelDeliveryOrder`)

**Purpose**: Cancel a delivery order.

**URL Parameters**:
- `id` (int64): Delivery order ID

**Form Fields**:
- `cancellation_reason` (string, required): Reason for cancellation

**Success Response**:
- 303 See Other: Redirects to delivery order detail page
- Flash message: "Delivery order cancelled successfully"

**Business Rules**:
- DRAFT, CONFIRMED, and IN_TRANSIT orders can be cancelled
- DELIVERED orders cannot be cancelled
- Cancellation restores quantities to sales order
- Reverses inventory transactions (if integration enabled)

---

### 11. List by Sales Order (`listDeliveryOrdersBySalesOrder`)

**Purpose**: Display all delivery orders for a specific sales order.

**URL Parameters**:
- `id` (int64): Sales order ID

**Response Template Data**:
```go
{
    "DeliveryOrders": []DeliveryOrderWithDetails,
    "SalesOrderID": int64,
}
```

**Template**: `delivery/list_by_sales_order`

## Helper Functions

### `parseDeliveryOrderLines(r *http.Request)`

Parses delivery order line items from form data.

**Returns**: `[]CreateDeliveryOrderLineReq, formErrors`

**Validation**:
- At least one line required
- Valid sales order line IDs
- Valid product IDs
- Positive quantities
- Proper array alignment

### `render(w, r, template, data)`

Renders template with common data injection:
- CSRF token
- Flash messages
- Session data

### `redirectWithFlash(w, r, url, message)`

Redirects with a success flash message.

### `getCurrentUserID(r)`

Extracts user ID from request context.

### `getCurrentCompanyID(r)`

Extracts company ID from request context.

## Error Handling

All handlers follow consistent error handling patterns:

1. **Validation Errors**: Re-render form with error messages
2. **Not Found**: 404 HTTP status
3. **Business Logic Errors**: Flash message with redirect
4. **System Errors**: 500 HTTP status with logged details

## CSRF Protection

All POST/PUT/DELETE handlers require valid CSRF token:
- Token generated via `csrf.EnsureToken(ctx, session)`
- Token validated by middleware
- Token included in all forms

## Session Management

Flash messages are stored in session:
- Success messages: Green notification
- Error messages: Red notification
- Messages auto-expire after display

## Template Integration

Expected template structure:

```
views/
  delivery/
    list.html              # Delivery order list
    detail.html            # Single delivery order
    form.html              # Create form
    edit.html              # Edit form
    list_by_sales_order.html  # Sales order deliveries
```

## Security Considerations

1. **RBAC**: All routes protected by permission checks
2. **Company Isolation**: All queries scoped to user's company
3. **CSRF**: All mutations protected
4. **Input Validation**: Server-side validation on all inputs
5. **SQL Injection**: Protected via parameterized queries

## Testing

See `handler_test.go` for comprehensive test coverage:
- Unit tests for each handler
- Mock service layer
- Request/response validation
- Error case coverage
- Permission testing

## Integration Points

### Sales Order Integration
- Creates delivery from confirmed sales orders
- Updates sales order quantities
- Triggers status transitions

### Inventory Integration
- Validates stock availability
- Records inventory transactions
- Handles stock reductions/restorations

### Audit Trail
- All mutations logged with user ID
- Status transitions tracked
- Timestamps recorded

## Performance Considerations

1. **Pagination**: Default 20 items per page
2. **Filtering**: Indexed database queries
3. **Session**: Redis-backed for scalability
4. **Templates**: Cached compiled templates

## Future Enhancements

- [ ] Bulk operations (multi-select actions)
- [ ] PDF packing list generation
- [ ] Email notifications
- [ ] Mobile-optimized UI
- [ ] Real-time status updates via WebSocket
- [ ] Barcode scanning integration
- [ ] Photo upload for proof of delivery

## Related Documentation

- [Domain Model](./domain.go) - Data structures
- [Service Layer](./service.go) - Business logic
- [Repository Layer](./repository.go) - Database operations
- [Database Schema](../../migrations/) - Table definitions

---

**Last Updated**: 2024
**Module Version**: Phase 9.2
**Status**: ✅ Complete