package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	dsn := getenv("PG_DSN", "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if err := seedChartOfAccounts(ctx, pool); err != nil {
		log.Fatalf("seed accounts: %v", err)
	}
	if err := seedMappings(ctx, pool); err != nil {
		log.Fatalf("seed mappings: %v", err)
	}
	log.Println("Phase 4.2 finance seed complete")
}

func seedChartOfAccounts(ctx context.Context, pool *pgxpool.Pool) error {
	file, err := os.Open(filepath.Join("samples", "coa.csv"))
	if err != nil {
		return fmt.Errorf("open samples/coa.csv: %w", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}
	if len(rows) <= 1 {
		return errors.New("coa.csv empty")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // nolint:errcheck

	codeToID := make(map[string]int64)
	for idx, row := range rows[1:] {
		if len(row) < 4 {
			return fmt.Errorf("row %d invalid", idx+1)
		}
		code := strings.TrimSpace(row[0])
		name := strings.TrimSpace(row[1])
		accType := strings.TrimSpace(row[2])
		parentCode := strings.TrimSpace(row[3])
		var parent any
		if parentCode != "" {
			if id, ok := codeToID[parentCode]; ok {
				parent = id
			} else {
				if err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE code=$1`, parentCode).Scan(&parent); err != nil {
					return fmt.Errorf("lookup parent %s: %w", parentCode, err)
				}
			}
		}
		var id int64
		err := tx.QueryRow(ctx, `INSERT INTO accounts (code, name, type, parent_id, is_active, created_at, updated_at)
VALUES ($1,$2,$3,$4,TRUE,NOW(),NOW())
ON CONFLICT (code) DO UPDATE SET name=EXCLUDED.name, type=EXCLUDED.type, parent_id=EXCLUDED.parent_id, is_active=EXCLUDED.is_active, updated_at=NOW()
RETURNING id`, code, name, accType, parent).Scan(&id)
		if err != nil {
			return fmt.Errorf("upsert account %s: %w", code, err)
		}
		codeToID[code] = id
	}
	return tx.Commit(ctx)
}

func seedMappings(ctx context.Context, pool *pgxpool.Pool) error {
	mappings := map[string]string{
		"grn.inventory":                  "1300",
		"grn.grir":                       "5500",
		"ap.invoice.ap":                  "2100",
		"ap.invoice.inventory":           "1300",
		"ap.invoice.expense":             "5200",
		"ap.invoice.tax_input":           "5400",
		"ap.payment.cash":                "1110",
		"ap.payment.ap":                  "2100",
		"inventory.adjustment.gain":      "5300",
		"inventory.adjustment.loss":      "5300",
		"inventory.adjustment.inventory": "1300",
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // nolint:errcheck

	for key, code := range mappings {
		parts := strings.SplitN(key, ".", 2)
		module := strings.ToUpper(parts[0])
		mappingKey := key
		var accountID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE code=$1`, code).Scan(&accountID); err != nil {
			return fmt.Errorf("lookup account %s: %w", code, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
VALUES ($1,$2,$3,NOW(),NOW())
ON CONFLICT (module, key) DO UPDATE SET account_id=EXCLUDED.account_id, updated_at=NOW()`, module, mappingKey, accountID); err != nil {
			return fmt.Errorf("upsert mapping %s: %w", key, err)
		}
	}
	return tx.Commit(ctx)
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
