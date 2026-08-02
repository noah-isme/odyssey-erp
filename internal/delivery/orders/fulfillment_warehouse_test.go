package orders

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fulfillmentWarehouseRepoFake struct {
	returnRepoFake
	salesOrder *SalesOrderInfo
	lines      []DeliverableSOLine
}

func (r *fulfillmentWarehouseRepoFake) GetSalesOrderDetails(context.Context, int64) (*SalesOrderInfo, error) {
	return r.salesOrder, nil
}

func (r *fulfillmentWarehouseRepoFake) GetDeliverableSOLines(context.Context, int64) ([]DeliverableSOLine, error) {
	return r.lines, nil
}

func TestCreateRejectsSalesLineAssignedToAnotherFulfillmentWarehouse(t *testing.T) {
	lineWarehouseID := int64(11)
	repo := &fulfillmentWarehouseRepoFake{
		salesOrder: &SalesOrderInfo{ID: 1, CompanyID: 1, CustomerID: 7, Status: "CONFIRMED"},
		lines: []DeliverableSOLine{{
			SalesOrderLineID: 1, ProductID: 3, Quantity: 2, RemainingQuantity: 2,
			FulfillmentWarehouseID: &lineWarehouseID,
		}},
	}

	_, err := NewService(repo).Create(context.Background(), CreateRequest{
		CompanyID: 1, SalesOrderID: 1, WarehouseID: 12, DeliveryDate: time.Now(),
		Lines: []CreateLineReq{{SalesOrderLineID: 1, ProductID: 3, QuantityToDeliver: 1}},
	}, 2)

	require.ErrorContains(t, err, "different fulfillment warehouse")
}
