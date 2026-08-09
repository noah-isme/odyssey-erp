package logistics

import (
	"context"
	"strings"
	"testing"
)

func TestServiceRejectsInvalidSetupInputsWithoutRepository(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"carrier name", func() error { _, err := svc.RegisterCarrier(ctx, CreateCarrierInput{}); return err }, "carrier name is required"},
		{"rate card carrier", func() error { _, err := svc.SetRateCard(ctx, CreateRateCardInput{}); return err }, "carrier ID is required"},
		{"rate card cities", func() error { _, err := svc.SetRateCard(ctx, CreateRateCardInput{CarrierID: 1}); return err }, "both from and to cities required"},
		{"fleet name", func() error { _, err := svc.CreateFleet(ctx, CreateFleetInput{}); return err }, "fleet name is required"},
		{"vehicle registration", func() error { _, err := svc.RegisterVehicle(ctx, CreateVehicleInput{}); return err }, "vehicle registration is required"},
		{"driver name", func() error { _, err := svc.RegisterDriver(ctx, CreateDriverInput{}); return err }, "driver name is required"},
		{"shipment number", func() error { _, err := svc.CreateShipment(ctx, CreateShipmentInput{}); return err }, "shipment number is required"},
		{"shipment type", func() error {
			_, err := svc.CreateShipment(ctx, CreateShipmentInput{ShipmentNumber: "SHP-1", ShipmentType: ShipmentType("INVALID")})
			return err
		}, "invalid shipment type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want %q", err, tt.want)
			}
		})
	}
}
