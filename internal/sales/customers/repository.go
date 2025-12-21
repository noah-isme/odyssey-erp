package customers

import (
	"context"
	"errors"
	"fmt"


	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

var (
	ErrNotFound      = errors.New("record not found")
	ErrAlreadyExists = errors.New("record already exists")
)

type Repository interface {
	WithTx(ctx context.Context, fn func(context.Context, Repository) error) error
	Get(ctx context.Context, id int64) (*Customer, error)
	GetByCode(ctx context.Context, companyID int64, code string) (*Customer, error)
	List(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error)
	Create(ctx context.Context, customer Customer) (int64, error)
	Update(ctx context.Context, id int64, updates map[string]interface{}) error
	GenerateCode(ctx context.Context, companyID int64) (string, error)
}

type dbtx interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

type repository struct {
	db      dbtx
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{
		db:      pool,
		queries: sqlc.New(pool),
		pool:    pool,
	}
}

func (r *repository) WithTx(ctx context.Context, fn func(context.Context, Repository) error) error {
	return db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		repoTx := &repository{
			db:      tx,
			queries: r.queries.WithTx(tx),
			pool:    r.pool,
		}
		return fn(ctx, repoTx)
	})
}

func (r *repository) Get(ctx context.Context, id int64) (*Customer, error) {
	row, err := r.queries.GetCustomer(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c := mapFromSqlc(row)
	return &c, nil
}

func (r *repository) GetByCode(ctx context.Context, companyID int64, code string) (*Customer, error) {
	row, err := r.queries.GetCustomerByCode(ctx, sqlc.GetCustomerByCodeParams{
		CompanyID: companyID,
		Code:      code,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c := mapFromSqlc(row)
	return &c, nil
}

func (r *repository) List(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, fmt.Sprintf("company_id = $%d", argPos))
	args = append(args, req.CompanyID)
	argPos++

	if req.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *req.IsActive)
		argPos++
	}

	if req.Search != nil && *req.Search != "" {
		searchPattern := "%" + *req.Search + "%"
		conditions = append(conditions, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d OR email ILIKE $%d)", argPos, argPos, argPos))
		args = append(args, searchPattern)
		argPos++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			whereClause += " AND " + conditions[i]
		}
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM customers %s", whereClause)
	var total int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch records
	query := fmt.Sprintf(`
		SELECT id, code, name, company_id, email, phone, tax_id,
		       credit_limit, payment_terms_days, address_line1, address_line2,
		       city, state, postal_code, country, is_active, notes,
		       created_by, created_at, updated_at
		FROM customers
		%s
		ORDER BY code
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var c Customer
		var creditLimit pgtype.Numeric
		var createdAt, updatedAt pgtype.Timestamptz
		var email, phone, taxID, addr1, addr2, city, state, postal, notes pgtype.Text

		err := rows.Scan(
			&c.ID, &c.Code, &c.Name, &c.CompanyID, &email, &phone, &taxID,
			&creditLimit, &c.PaymentTermsDays, &addr1, &addr2,
			&city, &state, &postal, &c.Country, &c.IsActive, &notes,
			&c.CreatedBy, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, 0, err
		}

		if email.Valid { c.Email = &email.String }
		if phone.Valid { c.Phone = &phone.String }
		if taxID.Valid { c.TaxID = &taxID.String }
		if creditLimit.Valid {
			f, _ := creditLimit.Float64Value()
			c.CreditLimit = f.Float64
		}
		if addr1.Valid { c.AddressLine1 = &addr1.String }
		if addr2.Valid { c.AddressLine2 = &addr2.String }
		if city.Valid { c.City = &city.String }
		if state.Valid { c.State = &state.String }
		if postal.Valid { c.PostalCode = &postal.String }
		if notes.Valid { c.Notes = &notes.String }
		if createdAt.Valid { c.CreatedAt = createdAt.Time }
		if updatedAt.Valid { c.UpdatedAt = updatedAt.Time }

		customers = append(customers, c)
	}

	return customers, total, rows.Err()
}

func (r *repository) Create(ctx context.Context, customer Customer) (int64, error) {
	var creditLimit pgtype.Numeric
	creditLimit.Scan(fmt.Sprintf("%f", customer.CreditLimit))
	
	return r.queries.CreateCustomer(ctx, sqlc.CreateCustomerParams{
		Code:             customer.Code,
		Name:             customer.Name,
		CompanyID:        customer.CompanyID,
		Email:            pgtype.Text{String: getString(customer.Email), Valid: customer.Email != nil},
		Phone:            pgtype.Text{String: getString(customer.Phone), Valid: customer.Phone != nil},
		TaxID:            pgtype.Text{String: getString(customer.TaxID), Valid: customer.TaxID != nil},
		CreditLimit:      creditLimit,
		PaymentTermsDays: int32(customer.PaymentTermsDays),
		AddressLine1:     pgtype.Text{String: getString(customer.AddressLine1), Valid: customer.AddressLine1 != nil},
		AddressLine2:     pgtype.Text{String: getString(customer.AddressLine2), Valid: customer.AddressLine2 != nil},
		City:             pgtype.Text{String: getString(customer.City), Valid: customer.City != nil},
		State:            pgtype.Text{String: getString(customer.State), Valid: customer.State != nil},
		PostalCode:       pgtype.Text{String: getString(customer.PostalCode), Valid: customer.PostalCode != nil},
		Country:          customer.Country,
		IsActive:         customer.IsActive,
		Notes:            pgtype.Text{String: getString(customer.Notes), Valid: customer.Notes != nil},
		CreatedBy:        customer.CreatedBy,
	})
}

func (r *repository) Update(ctx context.Context, id int64, updates map[string]interface{}) error {
	query := "UPDATE customers SET updated_at = NOW()"
	var args []interface{}
	argPos := 1
	
	if v, ok := updates["name"]; ok {
		query += fmt.Sprintf(", name = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["email"]; ok {
		query += fmt.Sprintf(", email = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["phone"]; ok {
		query += fmt.Sprintf(", phone = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["tax_id"]; ok {
		query += fmt.Sprintf(", tax_id = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["credit_limit"]; ok {
		query += fmt.Sprintf(", credit_limit = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["payment_terms_days"]; ok {
		query += fmt.Sprintf(", payment_terms_days = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["address_line1"]; ok {
		query += fmt.Sprintf(", address_line1 = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["address_line2"]; ok {
		query += fmt.Sprintf(", address_line2 = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["city"]; ok {
		query += fmt.Sprintf(", city = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["state"]; ok {
		query += fmt.Sprintf(", state = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["postal_code"]; ok {
		query += fmt.Sprintf(", postal_code = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["country"]; ok {
		query += fmt.Sprintf(", country = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["is_active"]; ok {
		query += fmt.Sprintf(", is_active = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["notes"]; ok {
		query += fmt.Sprintf(", notes = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	
	query += fmt.Sprintf(" WHERE id = $%d", argPos)
	args = append(args, id)
	
	_, err := r.db.Exec(ctx, query, args...)
	return err
}

func (r *repository) GenerateCode(ctx context.Context, companyID int64) (string, error) {
	// Pattern: CUST-{YYYY}-{SEQ}
	// Simplified: CUST-{SEQ} for globally unique or per company
	// Let's us sequence table or count. Count is risky.
	// But `GenerateCode` is usually "best effort" or "suggestion" in UI if form exists.
	// If it needs to be transactional, it should be done in Create.
	// But UI calls it to pre-fill form.
	// We'll return a count based suggestion.
	var count int64
	err := r.db.QueryRow(ctx, "SELECT count(*) FROM customers WHERE company_id = $1", companyID).Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CUST-%05d", count+1), nil
}

func mapFromSqlc(row sqlc.Customer) Customer {
	c := Customer{
		ID:               row.ID,
		Code:             row.Code,
		Name:             row.Name,
		CompanyID:        row.CompanyID,
		PaymentTermsDays: int(row.PaymentTermsDays),
		Country:          row.Country,
		IsActive:         row.IsActive,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
	if row.TaxID.Valid {
		val := row.TaxID.String
		c.TaxID = &val
	}
	if row.Email.Valid {
		val := row.Email.String
		c.Email = &val
	}
	if row.Phone.Valid {
		val := row.Phone.String
		c.Phone = &val
	}
	if row.CreditLimit.Valid {
		f, _ := row.CreditLimit.Float64Value()
		c.CreditLimit = f.Float64
	}
	if row.AddressLine1.Valid {
		val := row.AddressLine1.String
		c.AddressLine1 = &val
	}
	if row.AddressLine2.Valid {
		val := row.AddressLine2.String
		c.AddressLine2 = &val
	}
	if row.City.Valid {
		val := row.City.String
		c.City = &val
	}
	if row.State.Valid {
		val := row.State.String
		c.State = &val
	}
	if row.PostalCode.Valid {
		val := row.PostalCode.String
		c.PostalCode = &val
	}
	if row.Notes.Valid {
		val := row.Notes.String
		c.Notes = &val
	}
	return c
}

func getString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
