package inventory

import (
	"errors"
	"time"
)

// TransactionType enumerates supported inventory movements.
type TransactionType string

const (
	// TransactionTypeIn represents an inbound movement.
	TransactionTypeIn TransactionType = "IN"
	// TransactionTypeOut represents an outbound movement.
	TransactionTypeOut TransactionType = "OUT"
	// TransactionTypeTransfer used for transfer meta records.
	TransactionTypeTransfer TransactionType = "TRANSFER"
	// TransactionTypeAdjust indicates manual adjustments.
	TransactionTypeAdjust TransactionType = "ADJUST"
)

// Transaction models the header of inventory transaction.
type Transaction struct {
	ID          int64
	Code        string
	Type        TransactionType
	WarehouseID int64
	RefModule   string
	RefID       string
	Note        string
	PostedAt    time.Time
	CreatedBy   int64
	CreatedAt   time.Time
}

// TransactionLine models each product movement line.
type TransactionLine struct {
	ID             int64
	TransactionID  int64
	ProductID      int64
	Qty            float64
	UnitCost       float64
	SrcWarehouseID int64
	DstWarehouseID int64
}

// Balance summarises stock in warehouse per product.
type Balance struct {
	WarehouseID int64
	ProductID   int64
	Qty         float64
	AvgCost     float64
	UpdatedAt   time.Time
}

// StockCardEntry describes inventory card entry for reports.
type StockCardEntry struct {
	TxCode      string
	TxType      TransactionType
	PostedAt    time.Time
	QtyIn       float64
	QtyOut      float64
	BalanceQty  float64
	UnitCost    float64
	BalanceCost float64
	Note        string
}

// AdjustmentInput describes request to adjust stock.
type AdjustmentInput struct {
	Code        string
	WarehouseID int64
	ProductID   int64
	Qty         float64
	UnitCost    float64
	Note        string
	ActorID     int64
	RefModule   string
	RefID       string
}

// TransferInput describes transfer request between warehouses.
type TransferInput struct {
	Code         string
	ProductID    int64
	Qty          float64
	SrcWarehouse int64
	DstWarehouse int64
	UnitCost     float64
	Note         string
	ActorID      int64
	RefModule    string
	RefID        string
}

// InboundInput is used for GRN posting.
type InboundInput struct {
	Code        string
	WarehouseID int64
	ProductID   int64
	Qty         float64
	UnitCost    float64
	Note        string
	ActorID     int64
	RefModule   string
	RefID       string
}

// StockCardFilter filters card entries.
type StockCardFilter struct {
	WarehouseID int64
	ProductID   int64
	From        time.Time
	To          time.Time
	Limit       int
}

// ErrNegativeStock triggered when movement would result negative qty.
var ErrNegativeStock = errors.New("inventory: negative stock not allowed")

// ErrInvalidQuantity indicates invalid qty.
var ErrInvalidQuantity = errors.New("inventory: quantity must be non zero")

// ErrInvalidUnitCost indicates invalid cost value.
var ErrInvalidUnitCost = errors.New("inventory: unit cost must be >= 0")
