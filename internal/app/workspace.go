package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Workspace tenancy. A user may work in the companies where they hold an
// active rbac_user_role_assignments row (company-wide or branch-scoped).
// Users holding a role named in RBAC_GLOBAL_ROLES (legacy user_roles) see
// every company. Permission evaluation itself stays on user_roles.

// normalizeGlobalRoles trims, lower-cases and de-duplicates the configured
// RBAC_GLOBAL_ROLES names, dropping empty entries.
func normalizeGlobalRoles(roles []string) []string {
	seen := make(map[string]struct{}, len(roles))
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	return out
}

// userHoldsGlobalRole reports whether userID holds one of globalRoles
// (already normalized) through user_roles. Role names compare
// case-insensitively. No query runs when the list is empty.
func userHoldsGlobalRole(ctx context.Context, q *sqlc.Queries, userID int64, globalRoles []string) (bool, error) {
	if len(globalRoles) == 0 {
		return false, nil
	}
	names, err := q.UserGlobalRoleNames(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("list user roles: %w", err)
	}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		for _, global := range globalRoles {
			if name == global {
				return true, nil
			}
		}
	}
	return false, nil
}

// loadWorkspaceCompanies returns the companies userID may work in, ordered by
// name then id. The result is never nil, so it encodes as [] when empty.
func loadWorkspaceCompanies(ctx context.Context, db sqlc.DBTX, userID int64, globalRoles []string) ([]workspaceCompany, error) {
	q := sqlc.New(db)
	global, err := userHoldsGlobalRole(ctx, q, userID, normalizeGlobalRoles(globalRoles))
	if err != nil {
		return nil, err
	}
	companies := make([]workspaceCompany, 0)
	if global {
		rows, err := db.Query(ctx, "SELECT id, name FROM companies ORDER BY name, id")
		if err != nil {
			return nil, fmt.Errorf("list companies: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var company workspaceCompany
			if err := rows.Scan(&company.ID, &company.Name); err != nil {
				return nil, fmt.Errorf("scan company: %w", err)
			}
			companies = append(companies, company)
		}
		return companies, rows.Err()
	}
	rows, err := q.ListUserCompanies(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user companies: %w", err)
	}
	for _, row := range rows {
		companies = append(companies, workspaceCompany{ID: row.ID, Name: row.Name})
	}
	return companies, nil
}

// resolveWorkspaceCompany returns the company a session may keep: current
// when the user may work in it, otherwise the first permitted company, or 0
// when the user may work in none (fail closed: no implicit company).
func resolveWorkspaceCompany(ctx context.Context, db sqlc.DBTX, userID, current int64, globalRoles []string) (int64, error) {
	if current > 0 {
		ok, err := sqlc.New(db).UserHasActiveCompanyAssignment(ctx, sqlc.UserHasActiveCompanyAssignmentParams{UserID: userID, CompanyID: current})
		if err != nil {
			return 0, fmt.Errorf("check company assignment: %w", err)
		}
		if ok {
			return current, nil
		}
	}
	companies, err := loadWorkspaceCompanies(ctx, db, userID, globalRoles)
	if err != nil {
		return 0, err
	}
	if len(companies) == 0 {
		return 0, nil
	}
	for _, company := range companies {
		if company.ID == current {
			return current, nil
		}
	}
	return companies[0].ID, nil
}

// workspaceCompanyPermitted reports whether userID may switch to companyID.
func workspaceCompanyPermitted(ctx context.Context, db sqlc.DBTX, userID, companyID int64, globalRoles []string) (bool, error) {
	companies, err := loadWorkspaceCompanies(ctx, db, userID, globalRoles)
	if err != nil {
		return false, err
	}
	for _, company := range companies {
		if company.ID == companyID {
			return true, nil
		}
	}
	return false, nil
}
