package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/midtrans"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

func main() {
	ctx := context.Background()
	dsn := "postgres://odyssey:odyssey@localhost:5434/odyssey?sslmode=disable"
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	vault, err := shared.NewVault()
	if err != nil {
		log.Fatalf("init vault: %v", err)
	}

	registry := connectors.NewRegistry()
	registry.Register("midtrans", midtrans.NewAdapter(slog.Default(), vault))

	connectorsService := connectors.NewService(connectors.NewRepository(pool), vault, registry)

	// Create a Midtrans connection
	fmt.Println("1. Creating Midtrans connection...")
	connRec, err := connectorsService.CreateConnection(ctx, connectors.CreateConnectionParams{
		CompanyID:       1,
		Provider:        "midtrans",
		Type:            "payment",
		Name:            "Midtrans Sandbox",
		SecretPlaintext: "SB-Mid-server-DUMMYKEY123", // Dummy Midtrans server key
	})
	if err != nil {
		log.Fatalf("create connection: %v", err)
	}
	fmt.Printf("   -> Created Connection ID: %d\n", connRec.ID)

	// Create an AR Invoice to pay
	fmt.Println("2. Skipping AR Invoice creation for test simplicity...")

	fmt.Println("3. Initiating Checkout Intent...")
	result, err := connectorsService.CreateCheckoutIntent(ctx, connectors.CreateCheckoutIntentRequest{
		CompanyID:     1,
		ConnectionID:  connRec.ID,
		SourceType:    "ar_invoice",
		SourceID:      999, // Fake Invoice ID
		Amount:        1500000,
		Currency:      "IDR",
		CustomerName:  "Test Customer",
		CustomerEmail: "test@example.com",
		OrderID:       fmt.Sprintf("e2e-order-%d", 123456789),
	})

	if err != nil {
		log.Fatalf("create checkout intent failed: %v", err)
	}

	fmt.Printf("   -> Success!\n")
	fmt.Printf("   -> Payment Intent ID: %d\n", result.PaymentIntentID)
	fmt.Printf("   -> Snap Token: %s\n", result.Token)
	fmt.Printf("   -> Redirect URL: %s\n", result.RedirectURL)

	// Query the payment_intents table to ensure it persisted correctly
	fmt.Println("4. Verifying DB persistence...")
	queries := sqlc.New(pool)
	intent, err := queries.GetPaymentIntent(ctx, sqlc.GetPaymentIntentParams{
		ID:        result.PaymentIntentID,
		CompanyID: 1,
	})
	if err != nil {
		log.Fatalf("get payment intent: %v", err)
	}

	fmt.Printf("   -> Found DB Record: Status=%s, URL=%s\n", intent.Status, intent.CheckoutUrl.String)
	fmt.Println("E2E Test Passed!")
}
