package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

func main() {
	force := flag.Bool("force", false, "force password reset for existing administrator")
	flag.Parse()

	dsn := strings.TrimSpace(os.Getenv("PG_DSN"))
	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))
	password := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"))
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
		q := sqlc.New(tx)
		return bootstrapAdmin(ctx, q, email, string(hash), *force)
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

func bootstrapAdmin(ctx context.Context, q *sqlc.Queries, email, passwordHash string, force bool) error {
	// 1. Ensure the user exists (upsert password only if force is set)
	var userID int64
	var err error
	if force {
		userID, err = q.UpsertAdminUser(ctx, sqlc.UpsertAdminUserParams{
			Email:        email,
			PasswordHash: passwordHash,
		})
		if err != nil {
			return fmt.Errorf("upsert administrator: %w", err)
		}
		log.Println("force: administrator password was reset")
	} else {
		_, err = q.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{
			Email:        email,
			PasswordHash: passwordHash,
		})
		if err != nil {
			log.Printf("administrator %s already exists; password unchanged (re-run with --force to reset)", email)
		}
		// Re-read existing user to get the ID for role/permission assignment
		user, err := q.AuthGetUserByEmail(ctx, email)
		if err != nil {
			return fmt.Errorf("read existing administrator: %w", err)
		}
		userID = user.ID
	}

	// 2. Upsert admin role
	role, err := q.UpsertRole(ctx, sqlc.UpsertRoleParams{
		Name:        "admin",
		Description: "Production administrator",
	})
	if err != nil {
		return fmt.Errorf("upsert admin role: %w", err)
	}

	// 3. Upsert permissions and attach to role
	for _, name := range adminPermissions() {
		perm, err := q.CreatePermission(ctx, sqlc.CreatePermissionParams{
			Name:        name,
			Description: "Odyssey application permission",
		})
		if err != nil {
			return fmt.Errorf("upsert permission %s: %w", name, err)
		}
		if err := q.AttachPermissionToRole(ctx, sqlc.AttachPermissionToRoleParams{
			RoleID:       role.ID,
			PermissionID: perm.ID,
		}); err != nil {
			return fmt.Errorf("assign permission %s: %w", name, err)
		}
	}

	// 4. Assign role to user
	if err := q.AssignRoleToUser(ctx, sqlc.AssignRoleToUserParams{
		UserID: userID,
		RoleID: role.ID,
	}); err != nil {
		return fmt.Errorf("assign admin role: %w", err)
	}

	if !force {
		log.Println("administrator role assigned; password not changed")
	}
	return nil
}

func adminPermissions() []string {
	permissions := shared.CoreScopes()
	permissions = append(permissions, shared.FinanceScopes()...)
	permissions = append(permissions, shared.FinanceAnalyticsScopes()...)
	permissions = append(permissions, shared.FinanceAuditScopes()...)
	permissions = append(permissions, shared.FinanceConsolidationScopes()...)
	permissions = append(permissions, shared.FinanceInsightsScopes()...)
	permissions = append(permissions, shared.AllSalesDeliveryScopes()...)
	return permissions
}
