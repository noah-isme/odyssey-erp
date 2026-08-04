//go:build ignore

package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		dsn = "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// 1. Migrate Inspection Plans
	_, err = pool.Exec(ctx, `
		INSERT INTO qms_inspection_plans (company_id, name, description, reference_module, reference_id, is_active, created_by, created_at)
		SELECT company_id, name, 'Migrated from MRP', 'MRP_PRODUCT', product_id, active, created_by, created_at
		FROM mrp_inspection_plans
	`)
	if err != nil {
		log.Printf("Error migrating inspection plans: %v", err)
	} else {
		log.Println("Migrated inspection plans")
	}

	// 2. Migrate Inspections
	_, err = pool.Exec(ctx, `
		INSERT INTO qms_inspections (company_id, name, description, reference_module, reference_id, status, inspector_id, created_by, created_at)
		SELECT company_id, 'MRP Inspection ' || id, 'Migrated MRP inspection', 'MRP_WORK_ORDER', work_order_id, status, inspector_id, COALESCE(inspector_id, 1), created_at
		FROM mrp_inspections
	`)
	if err != nil {
		log.Printf("Error migrating inspections: %v", err)
	} else {
		log.Println("Migrated inspections")
	}

	// 3. Migrate Holds
	_, err = pool.Exec(ctx, `
		INSERT INTO qms_holds (company_id, reference_module, reference_id, reason, status, created_by, created_at, released_by, released_at)
		SELECT company_id, 'MRP', work_order_id, reason, status, created_by, created_at, released_by, released_at
		FROM mrp_quality_holds
	`)
	if err != nil {
		log.Printf("Error migrating holds: %v", err)
	} else {
		log.Println("Migrated holds")
	}

	log.Println("MRP Quality Cutover Migration completed successfully!")
}
