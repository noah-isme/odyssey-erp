package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dsn := getenv("PG_DSN", "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable")
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if err := seedUsers(ctx, pool); err != nil {
		log.Fatalf("seed users: %v", err)
	}
	if err := seedRBAC(ctx, pool); err != nil {
		log.Fatalf("seed rbac: %v", err)
	}
	if err := seedBoardPackTemplates(ctx, pool); err != nil {
		log.Fatalf("seed board pack templates: %v", err)
	}
	fmt.Println("Seed complete at", time.Now().Format(time.RFC3339))
}

func seedUsers(ctx context.Context, pool *pgxpool.Pool) error {
	password, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	_, err := pool.Exec(ctx, `INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
VALUES ($1, $2, TRUE, NOW(), NOW())
ON CONFLICT (email) DO NOTHING`, "admin@odyssey.local", string(password))
	return err
}

func seedRBAC(ctx context.Context, pool *pgxpool.Pool) error {
	perms := []struct {
		name        string
		description string
	}{
		{"org.view", "View organization data"},
		{"org.edit", "Manage organization data"},
		{"master.view", "View master data"},
		{"master.edit", "Manage master data"},
		{"master.import", "Import master data via CSV"},
		{"rbac.view", "View RBAC setup"},
		{"rbac.edit", "Manage RBAC configuration"},
		{"report.view", "Access reports"},
		{"inventory.view", "View inventory transactions"},
		{"inventory.edit", "Post inventory transactions"},
		{"procurement.view", "View procurement documents"},
		{"procurement.edit", "Manage procurement documents"},
		{"finance.ap.view", "View AP documents"},
		{"finance.ap.edit", "Manage AP documents"},
		{"finance.boardpack", "Generate Board Pack"},
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // nolint:errcheck

	for _, perm := range perms {
		if _, err := tx.Exec(ctx, `INSERT INTO permissions (name, description)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description`, perm.name, perm.description); err != nil {
			return err
		}
	}

	roles := []struct {
		name        string
		description string
		permissions []string
	}{
		{"admin", "Full access to all modules", []string{"org.view", "org.edit", "master.view", "master.edit", "master.import", "rbac.view", "rbac.edit", "report.view", "inventory.view", "inventory.edit", "procurement.view", "procurement.edit", "finance.ap.view", "finance.ap.edit", "finance.boardpack"}},
		{"manager", "Manage operations", []string{"org.view", "org.edit", "master.view", "master.edit", "master.import", "report.view", "inventory.view", "inventory.edit", "procurement.view", "procurement.edit", "finance.ap.view", "finance.boardpack"}},
		{"viewer", "Read-only access", []string{"org.view", "master.view", "report.view", "inventory.view", "procurement.view", "finance.ap.view"}},
	}

	for _, role := range roles {
		var roleID int64
		err := tx.QueryRow(ctx, `INSERT INTO roles (name, description)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, updated_at = NOW()
RETURNING id`, role.name, role.description).Scan(&roleID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
			return err
		}
		for _, permName := range role.permissions {
			if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id)
SELECT $1, id FROM permissions WHERE name = $2
ON CONFLICT DO NOTHING`, roleID, permName); err != nil {
				return err
			}
		}
	}

	var adminID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, "admin@odyssey.local").Scan(&adminID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			adminID = 0
		} else {
			return err
		}
	}
	if adminID != 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, adminID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id)
SELECT $1, id FROM roles WHERE name = 'admin'
ON CONFLICT DO NOTHING`, adminID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func seedBoardPackTemplates(ctx context.Context, pool *pgxpool.Pool) error {
	const name = "Standard Executive Pack"
	var exists bool
	err := pool.QueryRow(ctx, `SELECT TRUE FROM board_pack_templates WHERE name = $1 LIMIT 1`, name).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var createdBy int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, "admin@odyssey.local").Scan(&createdBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if err := pool.QueryRow(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&createdBy); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	sections := []map[string]any{
		{"type": "EXEC_SUMMARY", "title": "Executive Summary"},
		{"type": "PL_SUMMARY", "title": "Profit & Loss"},
		{"type": "BS_SUMMARY", "title": "Balance Sheet"},
		{"type": "CASHFLOW_SUMMARY", "title": "Cashflow"},
		{"type": "TOP_VARIANCES", "title": "Top Variances", "options": map[string]any{"limit": 10}},
	}
	payload, err := json.Marshal(sections)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `INSERT INTO board_pack_templates (name, description, sections, is_default, is_active, created_by)
VALUES ($1,$2,$3,TRUE,TRUE,$4)`, name, "Default sections covering summary, PL, BS, cashflow, dan variance.", payload, createdBy)
	return err
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
