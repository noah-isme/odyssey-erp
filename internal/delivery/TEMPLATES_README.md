# Delivery Order SSR Templates Documentation

This document describes the server-side rendered (SSR) templates for the Delivery Order module in Odyssey ERP.

## Overview

The Delivery Order templates provide a complete user interface for managing deliveries through server-side rendered HTML pages. All templates follow the PicoCSS framework conventions and integrate with HTMX for progressive enhancement.

## Template Structure

All templates are located in `web/templates/pages/delivery/` and follow this structure:

```
web/templates/pages/delivery/
├── orders_list.html          # List delivery orders with filtering
├── order_detail.html         # Single delivery order detail view
├── order_form.html           # Create new delivery order form
├── order_edit.html           # Edit existing delivery order form
└── orders_by_so.html         # List deliveries for a sales order
```

## Template Details

### 1. orders_list.html

**Purpose**: Display paginated list of delivery orders with filtering capabilities.

**Route**: `GET /delivery-orders`

**Data Structure**:
```go
{
    Data: {
        DeliveryOrders: []DeliveryOrderWithDetails,
        CurrentPage: int,
        TotalPages: int,
        TotalCount: int,
        Request: ListDeliveryOrdersRequest,
    },
    Flash: *FlashMessage,
    CSRFToken: string,
}
```

**Features**:
- Advanced filtering (status, search, date range, SO ID, warehouse ID)
- Pagination controls
- Status badges with color coding
- Clickable rows to detail page
- Flash message display
- Responsive table layout

**Filter Options**:
- **Status**: DRAFT, CONFIRMED, IN_TRANSIT, DELIVERED, CANCELLED
- **Search**: Document number, driver name
- **Date Range**: From/To delivery dates
- **Sales Order ID**: Filter by specific SO
- **Warehouse ID**: Filter by warehouse

**Actions**:
- Click row → View detail
- "New Delivery Order" button → Create form
- Filter form submission
- Clear filters

---

### 2. order_detail.html

**Purpose**: Display detailed view of a single delivery order with action buttons.

**Route**: `GET /delivery-orders/{id}`

**Data Structure**:
```go
{
    Data: {
        DeliveryOrder: *DeliveryOrder,
    },
    Flash: *FlashMessage,
    CSRFToken: string,
}
```

**Features**:
- Complete delivery order information
- Line items table with quantities
- Status-specific action buttons
- Modal dialogs for workflows
- Sales order link integration
- Audit trail (created by, confirmed by, etc.)

**Status-Based Actions**:

| Status | Available Actions |
|--------|-------------------|
| DRAFT | Edit, Confirm, Cancel |
| CONFIRMED | Ship, Cancel |
| IN_TRANSIT | Mark as Delivered, Cancel |
| DELIVERED | (View only) |
| CANCELLED | (View only) |

**Modal Dialogs**:
- **Ship Modal**: Mark as shipped with optional tracking number
- **Deliver Modal**: Mark as delivered with actual delivery date
- **Cancel Modal**: Cancel order with required reason

**Information Sections**:
1. **Delivery Information**: Doc number, SO link, customer, warehouse, dates
2. **Shipping Details**: Driver, vehicle, tracking number
3. **Audit Trail**: Created/confirmed/delivered/cancelled timestamps and users
4. **Line Items**: Products, quantities, SO line references

---

### 3. order_form.html

**Purpose**: Create a new delivery order from a sales order.

**Route**: `GET /delivery-orders/new`

**Data Structure**:
```go
{
    Data: {
        SalesOrderID: string (optional, pre-populate),
        FormData: url.Values (on validation error),
        Errors: map[string]string,
    },
    Flash: *FlashMessage,
    CSRFToken: string,
}
```

**Form Fields**:

**Header Section**:
- `sales_order_id` (number, required): Source sales order
- `warehouse_id` (number, required): Fulfillment warehouse
- `delivery_date` (date, required): Scheduled delivery date
- `driver_name` (text, optional): Driver name
- `vehicle_number` (text, optional): Vehicle registration
- `tracking_number` (text, optional): Shipment tracking
- `notes` (textarea, optional): Additional notes

**Line Items Section** (repeatable):
- `so_line_id[]` (number, required): Sales order line ID
- `product_id[]` (number, required): Product ID
- `quantity[]` (number, required): Quantity to deliver
- `line_notes[]` (text, optional): Line-specific notes

**Features**:
- Dynamic line item management (add/remove)
- Client-side validation
- Error summary display
- Form data persistence on validation errors
- Required field indicators (*)
- Helper text for each field

**JavaScript Functions**:
- `addLineItem()`: Add new line item row
- `removeLine(button)`: Remove line item (min 1 required)
- `loadSalesOrderLines()`: Placeholder for AJAX SO line loading
- Form validation on submit

---

### 4. order_edit.html

**Purpose**: Edit an existing delivery order (DRAFT only).

**Route**: `GET /delivery-orders/{id}/edit`

**Data Structure**:
```go
{
    Data: {
        DeliveryOrder: *DeliveryOrder,
        FormData: url.Values (on validation error),
        Errors: map[string]string,
    },
    Flash: *FlashMessage,
    CSRFToken: string,
}
```

**Editable Fields**:
- Delivery date
- Driver name
- Vehicle number
- Tracking number
- Notes
- Line item quantities
- Add/remove line items

**Non-Editable Fields** (display only):
- Sales order (immutable)
- Warehouse (immutable)
- Document number (system-generated)

**Features**:
- Pre-populated form with existing data
- Same dynamic line management as create form
- Validation and error handling
- Cancel returns to detail page

**Business Rules**:
- Only DRAFT orders can be edited
- Sales order and warehouse cannot be changed
- Line items can be modified or removed
- New line items can be added (must be from same SO)

---

### 5. orders_by_so.html

**Purpose**: Display all delivery orders for a specific sales order.

**Route**: `GET /sales-orders/{id}/delivery-orders`

**Data Structure**:
```go
{
    Data: {
        DeliveryOrders: []DeliveryOrderWithDetails,
        SalesOrderID: int64,
    },
    Flash: *FlashMessage,
    CSRFToken: string,
}
```

**Features**:
- Filtered list of deliveries for one SO
- Summary statistics by status
- Quick navigation to SO detail
- Create new delivery from this SO
- Status breakdown counts

**Summary Metrics**:
- Total deliveries
- Count by status (Draft, Confirmed, In Transit, Delivered, Cancelled)

**Actions**:
- "Back to Sales Order" → Returns to SO detail
- "New Delivery" → Create form pre-populated with SO ID
- Click row → View delivery detail

---

## Common Template Elements

### Status Badges

All templates use consistent status badge styling:

```html
{{ if eq .Status "DRAFT" }}<span class="badge badge-secondary">Draft</span>{{ end }}
{{ if eq .Status "CONFIRMED" }}<span class="badge badge-info">Confirmed</span>{{ end }}
{{ if eq .Status "IN_TRANSIT" }}<span class="badge badge-warning">In Transit</span>{{ end }}
{{ if eq .Status "DELIVERED" }}<span class="badge badge-success">Delivered</span>{{ end }}
{{ if eq .Status "CANCELLED" }}<span class="badge badge-danger">Cancelled</span>{{ end }}
```

**Badge Colors**:
- **Draft**: Gray (`#6c757d`)
- **Confirmed**: Cyan (`#0dcaf0`)
- **In Transit**: Yellow (`#ffc107`)
- **Delivered**: Green (`#198754`)
- **Cancelled**: Red (`#dc3545`)

### Flash Messages

Consistent flash message display across all templates:

```html
{{ if .Flash }}
<article class="flash-message flash-{{ .Flash.Kind }}">
    {{ .Flash.Message }}
</article>
{{ end }}
```

**Message Types**:
- `success`: Green background, success messages
- `error`: Red background, error messages
- `info`: Blue background, informational messages
- `warning`: Yellow background, warnings

### Modal Dialogs

Standard modal pattern using native `<dialog>` element:

```html
<dialog id="modalId">
    <article>
        <header>
            <button aria-label="Close" rel="prev" onclick="closeModal()"></button>
            <h3>Modal Title</h3>
        </header>
        <form method="post" action="/endpoint">
            <!-- Form content -->
            <footer>
                <button type="button" class="secondary" onclick="closeModal()">Cancel</button>
                <button type="submit">Confirm</button>
            </footer>
        </form>
    </article>
</dialog>
```

### CSRF Protection

All POST forms include CSRF token:

```html
<input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
```

### Date Formatting

Consistent date format across templates:

```html
{{ .Date.Format "2006-01-02" }}           <!-- Short date: 2024-01-15 -->
{{ .DateTime.Format "2006-01-02 15:04" }} <!-- With time: 2024-01-15 15:30 -->
```

## Styling Guidelines

### CSS Classes

All templates use PicoCSS as the base framework with custom additions:

**Layout**:
- `.grid`: Two-column responsive grid
- `.delivery-orders-wrapper`, etc.: Page-specific wrappers
- `section`: Content sections with automatic spacing

**Components**:
- `.badge`: Status indicators
- `.flash-message`: Notification banners
- `.error-summary`: Form validation errors
- `.line-item`: Repeatable form rows
- `.pagination`: Page navigation
- `.summary`: Summary statistics box

**Utilities**:
- `.small`: Smaller button/text size
- `.required`: Red asterisk for required fields
- `.secondary`: Secondary button style
- `.danger`: Red danger button
- `.success`: Green success button

### Responsive Design

All templates are mobile-responsive:
- Tables scroll horizontally on small screens
- Grids stack vertically on mobile
- Action buttons wrap appropriately
- Modals are mobile-friendly

## JavaScript Functionality

### Form Management

**Line Item Management** (order_form.html, order_edit.html):
```javascript
let lineIndex = 0;

function addLineItem() {
    // Add new line item row
}

function removeLine(button) {
    // Remove line item (min 1 required)
}
```

**Form Validation**:
```javascript
form.addEventListener('submit', function(e) {
    // Validate line items exist
    // Prevent submission if invalid
});
```

### Modal Controls

**Show/Hide Modals**:
```javascript
function showModal() {
    document.getElementById('modalId').showModal();
}

function closeModal() {
    document.getElementById('modalId').close();
}
```

## Integration Points

### Sales Order Integration

Links to sales orders:
```html
<a href="/sales/orders/{{ .SalesOrderID }}">{{ .SalesOrderNumber }}</a>
```

Pre-populate from SO:
```html
<a href="/delivery-orders/new?sales_order_id={{ .SalesOrderID }}">New Delivery</a>
```

### Inventory Integration (Future)

Placeholders for future features:
- Stock availability indicators
- Real-time inventory checks
- Product information lookup

### Document Generation (Future)

Planned features:
- PDF packing list generation
- Print-friendly views
- Email delivery notifications

## Accessibility

All templates follow accessibility best practices:

- Semantic HTML5 elements
- ARIA labels on interactive elements
- Keyboard navigation support
- Form labels associated with inputs
- Color contrast compliance
- Focus indicators
- Screen reader friendly

## Browser Compatibility

Templates are tested and compatible with:
- Chrome/Edge (latest)
- Firefox (latest)
- Safari (latest)
- Mobile browsers (iOS Safari, Chrome Mobile)

## Performance Considerations

- Minimal JavaScript for progressive enhancement
- CSS inlined in templates for faster initial render
- No external dependencies beyond PicoCSS
- Efficient template rendering
- Pagination to limit DOM size

## Future Enhancements

Planned improvements:

1. **HTMX Integration**:
   - Inline editing
   - Real-time status updates
   - Partial page updates
   - Optimistic UI updates

2. **Enhanced Forms**:
   - Auto-complete for products
   - AJAX sales order line loading
   - Real-time stock availability
   - Barcode scanning input

3. **Rich Features**:
   - Photo upload for proof of delivery
   - Digital signature capture
   - GPS location tracking
   - Print/export options

4. **Workflow Improvements**:
   - Bulk operations
   - Quick actions from list
   - Keyboard shortcuts
   - Mobile-optimized scanning

## Testing

Template testing checklist:

- [ ] All forms submit correctly
- [ ] CSRF tokens present and valid
- [ ] Validation errors display properly
- [ ] Flash messages appear/disappear
- [ ] Modals open/close correctly
- [ ] Line items add/remove works
- [ ] Pagination navigates correctly
- [ ] Status badges render properly
- [ ] Links navigate correctly
- [ ] Responsive layout works
- [ ] Accessibility compliance
- [ ] Browser compatibility

## Troubleshooting

### Common Issues

**Templates not rendering**:
- Check template path matches handler
- Verify data structure matches template expectations
- Check for template syntax errors

**Flash messages not appearing**:
- Ensure session middleware is active
- Verify flash message set before redirect
- Check session is properly saved

**CSRF validation failing**:
- Verify token included in form
- Check middleware order
- Ensure session is valid

**Line items not working**:
- Check JavaScript console for errors
- Verify form array naming (`name[]`)
- Test add/remove functions

**Modals not opening**:
- Check JavaScript function names match
- Verify dialog element ID
- Test showModal() browser support

## Related Documentation

- [Handler Documentation](./HANDLER_README.md) - HTTP endpoints
- [Service Layer](./service.go) - Business logic
- [Domain Models](./domain.go) - Data structures
- [Repository Layer](./repository.go) - Database operations

---

**Last Updated**: 2024
**Module Version**: Phase 9.2
**Status**: ✅ Complete