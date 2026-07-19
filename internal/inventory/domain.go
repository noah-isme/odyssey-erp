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
	LotID          int64
	SerialID       int64
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
	Code          string
	WarehouseID   int64
	ProductID     int64
	Qty           float64
	UnitCost      float64
	Note          string
	ActorID       int64
	RefModule     string
	RefID         string
	LotNumber     string
	ExpiryDate    *time.Time
	SerialNumbers []string
}

// ProductTraceability determines how a product must be received and valued.
type ProductTraceability struct {
	CostMethod  string
	TrackBatch  bool
	TrackSerial bool
}

// InventoryLot is a traceable received batch at a warehouse.
type InventoryLot struct {
	ID          int64
	ProductID   int64
	WarehouseID int64
	LotNumber   string
	ExpiryDate  *time.Time
	QtyOnHand   float64
	UnitCost    float64
}

// StockCardFilter filters card entries.
type StockCardFilter struct {
	WarehouseID int64
	ProductID   int64
	From        time.Time
	To          time.Time
	Limit       int
}

// StockAdjustmentStatus defines the state of an adjustment document.
type StockAdjustmentStatus string

const (
	StockAdjustmentStatusDraft     StockAdjustmentStatus = "DRAFT"
	StockAdjustmentStatusPosted    StockAdjustmentStatus = "POSTED"
	StockAdjustmentStatusCancelled StockAdjustmentStatus = "CANCELLED"
)

// StockAdjustment represents a manual inventory adjustment document.
type StockAdjustment struct {
	ID            int64
	UUID          string
	Number        string
	WarehouseID   int64
	Status        StockAdjustmentStatus
	Note          string
	AdjustmentAt  time.Time
	CreatedBy     int64
	PostedBy      int64
	PostedAt      time.Time
	CreatedAt     time.Time
	CreatorEmail  string
	WarehouseName string
	Lines         []StockAdjustmentLine
}

// StockAdjustmentLine represents a single item change in an adjustment.
type StockAdjustmentLine struct {
	ID           int64
	AdjustmentID int64
	ProductID    int64
	ProductName  string
	Qty          float64
	Note         string
}

// StockTakeStatus defines the state of a stock take session.
type StockTakeStatus string

const (
	StockTakeStatusDraft     StockTakeStatus = "DRAFT"
	StockTakeStatusPosted    StockTakeStatus = "POSTED"
	StockTakeStatusCancelled StockTakeStatus = "CANCELLED"
)

// StockTake represents a physical inventory count session.
type StockTake struct {
	ID            int64
	UUID          string
	Number        string
	WarehouseID   int64
	Status        StockTakeStatus
	Note          string
	TakenAt       time.Time
	CreatedBy     int64
	PostedBy      int64
	PostedAt      time.Time
	CreatedAt     time.Time
	CreatorEmail  string
	WarehouseName string
	Lines         []StockTakeLine
}

// StockTakeLine represents a count for a specific product.
type StockTakeLine struct {
	ID          int64
	StockTakeID int64
	ProductID   int64
	ProductName string
	SystemQty   float64
	PhysicalQty float64
	VarianceQty float64
	Note        string
}

// ValuationEntry for stock valuation reports.
type ValuationEntry struct {
	WarehouseID   int64
	WarehouseName string
	ProductID     int64
	ProductName   string
	SKU           string
	Qty           float64
	AvgCost       float64
	TotalValue    float64
}

// ReorderAlert models products below min stock.
type ReorderAlert struct {
	ProductID           int64
	ProductName         string
	SKU                 string
	MinStock            float64
	WarehouseID         int64
	WarehouseName       string
	CurrentQty          float64
	ReorderTarget       float64
	PreferredSupplierID int64
}

// ReorderRequest is the procurement-ready result of a low-stock alert.
type ReorderRequest struct {
	SupplierID  int64
	RequestedBy int64
	Note        string
	Lines       []ReorderRequestLine
}

type ReorderRequestLine struct {
	ProductID int64
	Qty       float64
	Note      string
}

// CreateStockTakeInput for new session.
type CreateStockTakeInput struct {
	WarehouseID int64
	Note        string
	TakenAt     time.Time
	CreatedBy   int64
}

// AddStockTakeLineInput for session items.
type AddStockTakeLineInput struct {
	StockTakeID int64
	ProductID   int64
	PhysicalQty float64
	Note        string
}

// ErrNegativeStock triggered when movement would result negative qty.
var ErrNegativeStock = errors.New("inventory: negative stock not allowed")

// ErrInvalidQuantity indicates invalid qty.
var ErrInvalidQuantity = errors.New("inventory: quantity must be non zero")

// ErrInvalidUnitCost indicates invalid cost value.
var ErrInvalidUnitCost = errors.New("inventory: unit cost must be >= 0")
