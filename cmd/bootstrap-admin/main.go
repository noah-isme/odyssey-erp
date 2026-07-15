package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func main() {
	dsn := strings.TrimSpace(os.Getenv("PG_DSN"))
	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if err := validateBootstrapInput(dsn, email, password); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := db.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return bootstrapAdmin(ctx, tx, email, string(hash))
	}); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}
	log.Printf("administrator %s is ready", email)
}

func validateBootstrapInput(dsn, email, password string) error {
	if dsn == "" {
		return errors.New("PG_DSN is required")
	}
	if email == "" || !strings.Contains(email, "@") {
		return errors.New("BOOTSTRAP_ADMIN_EMAIL must be a valid email address")
	}
	if len(password) < 12 {
		return errors.New("BOOTSTRAP_ADMIN_PASSWORD must contain at least 12 characters")
	}
	return nil
}

func bootstrapAdmin(ctx context.Context, tx pgx.Tx, email, passwordHash string) error {
	var userID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
		VALUES ($1, $2, TRUE, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE
		SET password_hash = EXCLUDED.password_hash, is_active = TRUE, updated_at = NOW()
		RETURNING id`, email, passwordHash).Scan(&userID); err != nil {
		return fmt.Errorf("upsert administrator: %w", err)
	}

	var roleID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO roles (name, description)
		VALUES ('admin', 'Production administrator')
		ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
		RETURNING id`).Scan(&roleID); err != nil {
		return fmt.Errorf("upsert admin role: %w", err)
	}

	for _, name := range adminPermissions() {
		var permissionID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO permissions (name, description)
			VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET description = permissions.description
			RETURNING id`, name, "Odyssey application permission").Scan(&permissionID); err != nil {
			return fmt.Errorf("upsert permission %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, roleID, permissionID); err != nil {
			return fmt.Errorf("assign permission %s: %w", name, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
		return fmt.Errorf("assign admin role: %w", err)
	}
	return nil
}

func adminPermissions() []string {
	permissions := []string{
		"org.view", "org.edit",
		"master.view", "master.edit", "master.import",
		"rbac.view", "rbac.edit", "report.view",
		"inventory.view", "inventory.edit",
		"procurement.view", "procurement.edit",
		"finance.ap.view", "finance.ap.create", "finance.ap.post", "finance.ap.void", "finance.ap.payment",
	}
	permissions = append(permissions, shared.CoreScopes()...)
	permissions = append(permissions, shared.FinanceScopes()...)
	permissions = append(permissions, shared.FinanceAnalyticsScopes()...)
	permissions = append(permissions, shared.FinanceAuditScopes()...)
	permissions = append(permissions, shared.FinanceConsolidationScopes()...)
	permissions = append(permissions, shared.FinanceInsightsScopes()...)
	permissions = append(permissions, shared.AllSalesDeliveryScopes()...)
	return permissions
}
