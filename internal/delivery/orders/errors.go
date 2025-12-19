package orders

import "errors"

// Domain errors for delivery orders.
var (
	// ErrNotFound indicates the requested delivery order was not found.
	ErrNotFound = errors.New("delivery order not found")
	// ErrOrderNotFound is an alias for ErrNotFound (backward compat).
	ErrOrderNotFound = ErrNotFound

	// Status transition errors.
	ErrCannotEdit    = errors.New("cannot edit delivery order in current status")
	ErrCannotConfirm = errors.New("cannot confirm delivery order in current status")
	ErrCannotShip    = errors.New("cannot ship delivery order in current status")
	ErrCannotDeliver = errors.New("cannot deliver order in current status")
	ErrCannotCancel  = errors.New("cannot cancel delivery order in current status")

	// Validation errors.
	ErrEmptyLines          = errors.New("at least one line is required")
	ErrInvalidQuantity     = errors.New("quantity must be greater than zero")
	ErrInvalidDeliveryDate = errors.New("delivery date cannot be in the past")
	ErrReasonTooShort      = errors.New("cancellation reason must be at least 10 characters")
	ErrSOLineNotFound      = errors.New("sales order line not found or fully delivered")
	ErrQuantityExceeds     = errors.New("requested quantity exceeds remaining")
	ErrProductMismatch     = errors.New("product ID mismatch")

	// Business rule errors.
	ErrNoDeliverableLines = errors.New("no deliverable lines found for sales order")
	ErrSONotDeliverable   = errors.New("sales order must be CONFIRMED or PROCESSING")
	ErrCompanyMismatch    = errors.New("sales order belongs to different company")
	ErrWarehouseNotFound  = errors.New("warehouse not found")
	ErrNoLines            = errors.New("cannot confirm without lines")

	// External service errors.
	ErrInventoryFailed = errors.New("inventory service operation failed")
)
