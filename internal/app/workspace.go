package app

import (
	"context"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// loadWorkspaceCompanies returns the companies userID may work in.
// Stub with the v0.10.0-rc.10 behaviour: every company.
func loadWorkspaceCompanies(ctx context.Context, db sqlc.DBTX, userID int64, globalRoles []string) ([]workspaceCompany, error) {
	_ = userID
	_ = globalRoles
	rows, err := db.Query(ctx, "SELECT id, name FROM companies ORDER BY name, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	companies := make([]workspaceCompany, 0)
	for rows.Next() {
		var company workspaceCompany
		if err := rows.Scan(&company.ID, &company.Name); err != nil {
			return nil, err
		}
		companies = append(companies, company)
	}
	return companies, rows.Err()
}

// resolveWorkspaceCompany returns the company a session may keep.
// Stub with the v0.10.0-rc.10 behaviour: any set company is kept, otherwise
// the lowest company id.
func resolveWorkspaceCompany(ctx context.Context, db sqlc.DBTX, userID, current int64, globalRoles []string) (int64, error) {
	_ = userID
	_ = globalRoles
	if current > 0 {
		return current, nil
	}
	var companyID int64
	err := db.QueryRow(ctx, "SELECT id FROM companies ORDER BY id LIMIT 1").Scan(&companyID)
	return companyID, err
}
