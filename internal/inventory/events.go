package inventory

import "time"

// AdjustmentPostedEvent represents an inventory adjustment ready for ledger posting.
type AdjustmentPostedEvent struct {
	Code        string
	WarehouseID int64
	ProductID   int64
	Qty         float64
	UnitCost    float64
	PostedAt    time.Time
}
