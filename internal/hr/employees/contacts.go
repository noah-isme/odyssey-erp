package employees

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// EmergencyContact maps an hr_employee_emergency_contacts row.
type EmergencyContact struct {
	ID, EmployeeID                   int64
	Name, Relationship, Phone, Email string
	IsPrimary                        bool
	CreatedAt, UpdatedAt             time.Time
}

// ContactInput creates (ID == 0) or updates (ID > 0) an emergency contact.
type ContactInput struct {
	ID                               int64
	Name, Relationship, Phone, Email string
	IsPrimary                        bool
	ActorID                          int64
}

const contactCols = `c.id,c.employee_id,c.name,c.relationship,c.phone,c.email,c.is_primary,c.created_at,c.updated_at`

func scanContact(row pgx.Row) (EmergencyContact, error) {
	var c EmergencyContact
	err := row.Scan(&c.ID, &c.EmployeeID, &c.Name, &c.Relationship, &c.Phone, &c.Email, &c.IsPrimary, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// ListContacts returns the employee's emergency contacts, primary first.
func (s *Service) ListContacts(ctx context.Context, companyID, employeeID int64) ([]EmergencyContact, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT `+contactCols+` FROM hr_employee_emergency_contacts c JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=$1 WHERE c.employee_id=$2 ORDER BY c.is_primary DESC, c.name, c.id`, companyID, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmergencyContact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpsertContact creates or updates an emergency contact of an employee in the
// company. Marking a contact primary clears the flag on the others.
func (s *Service) UpsertContact(ctx context.Context, companyID, employeeID int64, in ContactInput) (EmergencyContact, error) {
	if companyID <= 0 {
		return EmergencyContact{}, errCompanyRequired
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Relationship = strings.TrimSpace(in.Relationship)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Email = strings.TrimSpace(in.Email)
	if employeeID <= 0 || in.ID < 0 || in.Name == "" {
		return EmergencyContact{}, errors.New("hr: invalid emergency contact")
	}
	if s.audit == nil {
		return EmergencyContact{}, errAuditNotConfigured
	}
	var out EmergencyContact
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var locked int64
		err := tx.QueryRow(ctx, `SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE`, employeeID, companyID).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEmployeeNotFound
		}
		if err != nil {
			return fmt.Errorf("lock employee: %w", err)
		}
		if in.ID > 0 {
			out, err = scanContact(tx.QueryRow(ctx, `UPDATE hr_employee_emergency_contacts c SET name=$1, relationship=$2, phone=$3, email=$4, is_primary=$5, updated_at=NOW()
FROM hr_employees e WHERE e.id=c.employee_id AND e.company_id=$6 AND c.employee_id=$7 AND c.id=$8 RETURNING `+contactCols,
				in.Name, in.Relationship, in.Phone, in.Email, in.IsPrimary, companyID, employeeID, in.ID))
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrContactNotFound
			}
		} else {
			out, err = scanContact(tx.QueryRow(ctx, `INSERT INTO hr_employee_emergency_contacts AS c(company_id,employee_id,name,relationship,phone,email,is_primary)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+contactCols,
				companyID, employeeID, in.Name, in.Relationship, in.Phone, in.Email, in.IsPrimary))
		}
		if err != nil {
			return fmt.Errorf("save emergency contact: %w", err)
		}
		if in.IsPrimary {
			if _, err := tx.Exec(ctx, `UPDATE hr_employee_emergency_contacts SET is_primary=FALSE, updated_at=NOW() WHERE employee_id=$1 AND id<>$2 AND is_primary`, employeeID, out.ID); err != nil {
				return fmt.Errorf("clear primary contact: %w", err)
			}
		}
		return s.recordTx(ctx, tx, in.ActorID, ActionEmployeeContactUpsert, employeeID, map[string]any{
			"contact_id": out.ID,
			"created":    in.ID == 0,
			"fields":     []string{"name", "relationship", "phone", "email", "is_primary"},
		})
	})
	if err != nil {
		return EmergencyContact{}, err
	}
	return out, nil
}

// DeleteContact removes an emergency contact of an employee in the company.
func (s *Service) DeleteContact(ctx context.Context, companyID, employeeID, contactID, actorID int64) error {
	if companyID <= 0 {
		return errCompanyRequired
	}
	if employeeID <= 0 || contactID <= 0 {
		return ErrContactNotFound
	}
	if s.audit == nil {
		return errAuditNotConfigured
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM hr_employee_emergency_contacts c USING hr_employees e WHERE e.id=c.employee_id AND e.company_id=$1 AND c.employee_id=$2 AND c.id=$3`, companyID, employeeID, contactID)
		if err != nil {
			return fmt.Errorf("delete emergency contact: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrContactNotFound
		}
		return s.recordTx(ctx, tx, actorID, ActionEmployeeContactDeleted, employeeID, map[string]any{"contact_id": contactID})
	})
}
