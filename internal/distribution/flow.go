package distribution

import (
	"context"
	"time"
)

// ShipmentGateway is the small logistics port used by distribution. Keeping
// this interface here prevents the planning module from depending on the
// logistics repository or its generated database types.
type ShipmentGateway interface {
	CreateShipment(context.Context, ShipmentCreateInput) (int64, error)
	AddShipmentLine(context.Context, ShipmentLineInput) error
	GetShipmentLines(context.Context, int64) ([]ShipmentLine, error)
	DispatchShipment(context.Context, int64, *int64, *int64, *int64, *string) error
	MarkShipmentInTransit(context.Context, int64) error
	MarkShipmentDelivered(context.Context, int64, time.Time) error
}

// InventoryGateway is the inventory boundary used when a delivered load
// leaves its origin warehouse. Implementations are responsible for invoking
// the normal inventory-to-GL integration hooks.
type InventoryGateway interface {
	PostAdjustment(context.Context, InventoryAdjustmentInput) error
}

type ShipmentCreateInput struct {
	CompanyID              int64
	ShipmentNumber         string
	ShipmentType           string
	OriginWarehouseID      *int64
	DestinationWarehouseID *int64
	DestinationAddress     string
	DestinationCity        string
	DestinationCountry     string
	PlannedDispatchAt      *time.Time
	PlannedDeliveryAt      *time.Time
	CreatedBy              int64
}

type ShipmentLineInput struct {
	CompanyID  int64
	ShipmentID int64
	ProductID  int64
	Quantity   float64
	WeightKg   *float64
	VolumeCbm  *float64
}

type ShipmentLine struct {
	ProductID int64
	Quantity  float64
}

type InventoryAdjustmentInput struct {
	Code        string
	WarehouseID int64
	ProductID   int64
	Quantity    float64
	Note        string
	ActorID     int64
	RefModule   string
	RefID       string
}

type Dependencies struct {
	Shipments ShipmentGateway
	Inventory InventoryGateway
}
