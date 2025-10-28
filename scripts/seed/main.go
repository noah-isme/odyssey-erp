package main

import (
	"context"
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
		{"admin", "Full access to all modules", []string{"org.view", "org.edit", "master.view", "master.edit", "master.import", "rbac.view", "rbac.edit", "report.view"}},
		{"manager", "Manage organization and master data", []string{"org.view", "org.edit", "master.view", "master.edit", "master.import", "report.view"}},
		{"viewer", "Read-only access", []string{"org.view", "master.view", "report.view"}},
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

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
