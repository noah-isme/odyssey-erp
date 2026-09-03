package app

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/distribution"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/logistics"
)

// logisticsDistributionGateway adapts the logistics service to the narrow
// distribution port. The distribution package never sees logistics' SQLC
// models or repository types.
type logisticsDistributionGateway struct {
	service *logistics.Service
}

// NewDistributionHandler wires distribution to the application-owned
// logistics and inventory services. The adapter is kept in app so neither
// domain package needs to import the other.
func NewDistributionHandler(db *pgxpool.Pool, logisticsService *logistics.Service, inventoryService *inventory.Service) *distribution.Handler {
	service := distribution.NewServiceWithDependencies(distribution.NewRepository(db), distribution.Dependencies{
		Shipments: logisticsDistributionGateway{service: logisticsService},
		Inventory: inventoryDistributionGateway{service: inventoryService},
	})
	return distribution.NewHandler(service)
}

func (g logisticsDistributionGateway) CreateShipment(ctx context.Context, input distribution.ShipmentCreateInput) (int64, error) {
	shipment, err := g.service.CreateShipment(ctx, logistics.CreateShipmentInput{
		CompanyID:              input.CompanyID,
		ShipmentNumber:         input.ShipmentNumber,
		ShipmentType:           logistics.ShipmentType(input.ShipmentType),
		OriginWarehouseID:      input.OriginWarehouseID,
		DestinationWarehouseID: input.DestinationWarehouseID,
		DestinationAddress:     input.DestinationAddress,
		DestinationCity:        input.DestinationCity,
		DestinationCountry:     input.DestinationCountry,
		PlannedDispatchAt:      input.PlannedDispatchAt,
		PlannedDeliveryAt:      input.PlannedDeliveryAt,
		CreatedBy:              input.CreatedBy,
	})
	if err != nil {
		return 0, err
	}
	return shipment.ID, nil
}

func (g logisticsDistributionGateway) AddShipmentLine(ctx context.Context, input distribution.ShipmentLineInput) error {
	_, err := g.service.AddItemToShipment(ctx, logistics.AddShipmentLineInput{
		CompanyID:  input.CompanyID,
		ShipmentID: input.ShipmentID,
		ProductID:  input.ProductID,
		Quantity:   input.Quantity,
		WeightKg:   input.WeightKg,
		VolumeCbm:  input.VolumeCbm,
	})
	return err
}

func (g logisticsDistributionGateway) GetShipmentLines(ctx context.Context, shipmentID int64) ([]distribution.ShipmentLine, error) {
	lines, err := g.service.GetShipmentLines(ctx, shipmentID)
	if err != nil {
		return nil, err
	}
	result := make([]distribution.ShipmentLine, 0, len(lines))
	for _, line := range lines {
		result = append(result, distribution.ShipmentLine{ProductID: line.ProductID, Quantity: moneyFloat(line.Quantity.String())})
	}
	return result, nil
}

func (g logisticsDistributionGateway) DispatchShipment(ctx context.Context, shipmentID int64, vehicleID, driverID, carrierID *int64, carrierService *string) error {
	var serviceType *logistics.CarrierServiceType
	if carrierService != nil {
		value := logistics.CarrierServiceType(*carrierService)
		serviceType = &value
	}
	return g.service.DispatchShipment(ctx, shipmentID, vehicleID, driverID, carrierID, serviceType)
}

func (g logisticsDistributionGateway) MarkShipmentInTransit(ctx context.Context, shipmentID int64) error {
	return g.service.MarkShipmentInTransit(ctx, shipmentID)
}

func (g logisticsDistributionGateway) MarkShipmentDelivered(ctx context.Context, shipmentID int64, deliveredAt time.Time) error {
	return g.service.MarkShipmentDelivered(ctx, shipmentID, deliveredAt)
}

type inventoryDistributionGateway struct {
	service *inventory.Service
}

func (g inventoryDistributionGateway) PostAdjustment(ctx context.Context, input distribution.InventoryAdjustmentInput) error {
	_, err := g.service.PostAdjustment(ctx, inventory.AdjustmentInput{
		Code:        input.Code,
		WarehouseID: input.WarehouseID,
		ProductID:   input.ProductID,
		Qty:         input.Quantity,
		Note:        input.Note,
		ActorID:     input.ActorID,
		RefModule:   input.RefModule,
		RefID:       input.RefID,
	})
	return err
}

func moneyFloat(value string) float64 {
	result, _ := strconv.ParseFloat(value, 64)
	return result
}
