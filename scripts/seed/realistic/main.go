package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	skipMV := flag.Bool("skip-mv", false, "Skip refreshing analytical materialized views in Phase 18")
	flag.Parse()

	if *skipMV {
		SkipMaterializedViews = true
	}

	startTime := time.Now()
	dsn := Getenv("PG_DSN", "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable")
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres pool: %v", err)
	}
	defer pool.Close()

	sctx := NewSeedContext(ctx, pool)

	fmt.Println("========================================================================")
	fmt.Println(" Odyssey ERP Realistic Seed Suite — PT Nusantara Teknik Perkasa")
	fmt.Println("========================================================================")

	type phaseDef struct {
		num  string
		name string
		fn   func(context.Context, *SeedContext) error
	}

	phases := []phaseDef{
		{"01", "Foundation Master Data", seedPhase01Foundation},
		{"02", "Finance & Accounting", seedPhase02Finance},
		{"03", "CRM Pipeline & Leads", seedPhase03CRM},
		{"04", "Procurement (P2P)", seedPhase04Procurement},
		{"05", "Sales (O2C)", seedPhase05Sales},
		{"06", "Manufacturing & MRP", seedPhase06MRP},
		{"07", "CMMS Maintenance", seedPhase07CMMS},
		{"08", "QMS Quality Management", seedPhase08QMS},
		{"09", "HR & Payroll Engine", seedPhase09HRPayroll},
		{"10", "Fixed Assets Management", seedPhase10Assets},
		{"11", "Point of Sale (POS)", seedPhase11POS},
		{"12", "Projects & Timesheets", seedPhase12Projects},
		{"13", "Logistics & Fleet Management", seedPhase13Logistics},
		{"14", "Inventory & WMS Management", seedPhase14InventoryWMS},
		{"15", "Banking & Treasury", seedPhase15Banking},
		{"16", "Document Management System", seedPhase16Documents},
		{"17", "Consolidation & Financial Close", seedPhase17Consolidation},
		{"18", "General Ledger Posting & Materialized Views Refresh", seedPhase18JournalsRefresh},
	}

	for _, p := range phases {
		phaseStart := time.Now()
		fmt.Printf("→ Seeding Phase %s: %s...\n", p.num, p.name)
		if err := p.fn(ctx, sctx); err != nil {
			log.Fatalf("❌ Phase %s failed: %v", p.num, err)
		}
		fmt.Printf("✔ Phase %s completed in %v\n", p.num, time.Since(phaseStart).Round(time.Millisecond))
	}

	fmt.Println("========================================================================")
	fmt.Printf(" Realistic seed execution finished successfully in %v\n", time.Since(startTime).Round(time.Millisecond))
	fmt.Printf(" Completed at %s\n", time.Now().Format(time.RFC3339))
	fmt.Println("========================================================================")
}
