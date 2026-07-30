package crm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func visibility(scope Scope, companyColumn, ownerColumn string) (string, []any) {
	if scope.ViewAll {
		return companyColumn + "=$1", []any{scope.CompanyID}
	}
	return companyColumn + "=$1 AND " + ownerColumn + "=$2", []any{scope.CompanyID, scope.UserID}
}

func (r *Repository) Stages(ctx context.Context, scope Scope) ([]Stage, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,company_id,name,stage_type,position,probability FROM crm_pipeline_stages WHERE company_id=$1 ORDER BY position`, scope.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Stage
	for rows.Next() {
		var x Stage
		if err = rows.Scan(&x.ID, &x.CompanyID, &x.Name, &x.Type, &x.Position, &x.Probability); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func scanLead(row pgx.Row, x *Lead) error {
	return row.Scan(&x.ID, &x.CompanyID, &x.OwnerID, &x.Source, &x.Name, &x.Organization, &x.Email, &x.Phone, &x.Status, &x.Notes, &x.CustomerID, &x.ContactID, &x.CreatedBy, &x.CreatedAt, &x.UpdatedAt)
}
func scanOpportunity(row pgx.Row, x *Opportunity) error {
	return row.Scan(&x.ID, &x.CompanyID, &x.LeadID, &x.ContactID, &x.CustomerID, &x.QuotationID, &x.OwnerID, &x.StageID, &x.Name, &x.Source, &x.ExpectedValue, &x.CloseDate, &x.Status, &x.Reason, &x.StageName, &x.CreatedBy, &x.CreatedAt, &x.UpdatedAt)
}

func (r *Repository) Pipeline(ctx context.Context, scope Scope) (Pipeline, error) {
	out := Pipeline{}
	var err error
	out.Stages, err = r.Stages(ctx, scope)
	if err != nil {
		return out, err
	}
	where, args := visibility(scope, "o.company_id", "o.owner_id")
	rows, err := r.pool.Query(ctx, `SELECT o.id,o.company_id,o.lead_id,o.contact_id,o.customer_id,o.quotation_id,o.owner_id,o.stage_id,o.name,o.source,o.expected_value,o.close_date,o.status,o.win_loss_reason,s.name,o.created_by,o.created_at,o.updated_at FROM crm_opportunities o JOIN crm_pipeline_stages s ON s.id=o.stage_id WHERE `+where+` ORDER BY s.position,o.close_date NULLS LAST,o.id`, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var x Opportunity
		if err = scanOpportunity(rows, &x); err != nil {
			return out, err
		}
		out.Opportunities = append(out.Opportunities, x)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	leadWhere, leadArgs := visibility(scope, "company_id", "owner_id")
	leadRows, err := r.pool.Query(ctx, `SELECT id,company_id,owner_id,source,name,organization,email,phone,status,notes,converted_customer_id,converted_contact_id,created_by,created_at,updated_at FROM crm_leads WHERE `+leadWhere+` AND status='NEW' ORDER BY created_at DESC`, leadArgs...)
	if err != nil {
		return out, err
	}
	defer leadRows.Close()
	for leadRows.Next() {
		var x Lead
		if err = scanLead(leadRows, &x); err != nil {
			return out, err
		}
		out.Leads = append(out.Leads, x)
	}
	return out, leadRows.Err()
}

func (r *Repository) CreateLead(ctx context.Context, in CreateLeadInput) (Lead, error) {
	var x Lead
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO crm_leads(company_id,owner_id,source,name,organization,email,phone,notes,created_by) VALUES($1,$2,$3,$4,$5,LOWER($6),$7,$8,$9) RETURNING id,company_id,owner_id,source,name,organization,email,phone,status,notes,converted_customer_id,converted_contact_id,created_by,created_at,updated_at`, in.CompanyID, in.OwnerID, in.Source, in.Name, in.Organization, in.Email, in.Phone, in.Notes, in.CreatedBy).Scan(&x.ID, &x.CompanyID, &x.OwnerID, &x.Source, &x.Name, &x.Organization, &x.Email, &x.Phone, &x.Status, &x.Notes, &x.CustomerID, &x.ContactID, &x.CreatedBy, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO crm_events(company_id,entity_type,entity_id,event_type,actor_id) VALUES($1,'LEAD',$2,'CREATED',$3)`, x.CompanyID, x.ID, in.CreatedBy)
		return err
	})
	return x, err
}

func (r *Repository) Lead(ctx context.Context, scope Scope, id int64) (Lead, error) {
	var x Lead
	where, args := visibility(scope, "company_id", "owner_id")
	args = append(args, id)
	err := scanLead(r.pool.QueryRow(ctx, `SELECT id,company_id,owner_id,source,name,organization,email,phone,status,notes,converted_customer_id,converted_contact_id,created_by,created_at,updated_at FROM crm_leads WHERE `+where+fmt.Sprintf(" AND id=$%d", len(args)), args...), &x)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return x, err
}

func (r *Repository) Qualify(ctx context.Context, scope Scope, in QualifyInput) (out Opportunity, err error) {
	err = pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(tx pgx.Tx) error {
		var lead Lead
		where, args := visibility(scope, "company_id", "owner_id")
		args = append(args, in.LeadID)
		if e := scanLead(tx.QueryRow(ctx, `SELECT id,company_id,owner_id,source,name,organization,email,phone,status,notes,converted_customer_id,converted_contact_id,created_by,created_at,updated_at FROM crm_leads WHERE `+where+fmt.Sprintf(" AND id=$%d FOR UPDATE", len(args)), args...), &lead); e != nil {
			return e
		}
		if lead.Status == "QUALIFIED" || lead.Status == "CONVERTED" {
			return scanOpportunity(tx.QueryRow(ctx, `SELECT o.id,o.company_id,o.lead_id,o.contact_id,o.customer_id,o.quotation_id,o.owner_id,o.stage_id,o.name,o.source,o.expected_value,o.close_date,o.status,o.win_loss_reason,s.name,o.created_by,o.created_at,o.updated_at FROM crm_opportunities o JOIN crm_pipeline_stages s ON s.id=o.stage_id WHERE o.lead_id=$1`, lead.ID), &out)
		}
		if lead.Status != "NEW" {
			return ErrInvalidStage
		}
		var contactID int64
		if lead.Email != "" {
			var exists int64
			e := tx.QueryRow(ctx, `SELECT id FROM crm_contacts WHERE company_id=$1 AND LOWER(email)=LOWER($2)`, lead.CompanyID, lead.Email).Scan(&exists)
			if e == nil {
				return ErrDuplicateContact
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
		}
		if e := tx.QueryRow(ctx, `INSERT INTO crm_contacts(company_id,lead_id,name,email,phone,created_by) VALUES($1,$2,$3,LOWER($4),$5,$6) ON CONFLICT (company_id,LOWER(email)) WHERE email <> '' DO NOTHING RETURNING id`, lead.CompanyID, lead.ID, lead.Name, lead.Email, lead.Phone, in.ActorID).Scan(&contactID); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrDuplicateContact
			}
			return duplicateContact(e)
		}
		var stageID int64
		if e := tx.QueryRow(ctx, `SELECT id FROM crm_pipeline_stages WHERE company_id=$1 AND stage_type='OPEN' ORDER BY position LIMIT 1`, lead.CompanyID).Scan(&stageID); e != nil {
			return e
		}
		name := in.OpportunityName
		if name == "" {
			name = lead.Organization
			if name == "" {
				name = lead.Name
			}
		}
		if e := tx.QueryRow(ctx, `INSERT INTO crm_opportunities(company_id,lead_id,contact_id,owner_id,stage_id,name,source,expected_value,close_date,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, lead.CompanyID, lead.ID, contactID, lead.OwnerID, stageID, name, lead.Source, in.ExpectedValue, in.CloseDate, in.ActorID).Scan(&out.ID); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE crm_leads SET status='QUALIFIED',converted_contact_id=$2,updated_at=NOW() WHERE id=$1`, lead.ID, contactID); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO crm_events(company_id,entity_type,entity_id,event_type,actor_id,details) VALUES($1,'LEAD',$2,'QUALIFIED',$3,jsonb_build_object('opportunity_id',$4::bigint))`, lead.CompanyID, lead.ID, in.ActorID, out.ID); e != nil {
			return e
		}
		return scanOpportunity(tx.QueryRow(ctx, `SELECT o.id,o.company_id,o.lead_id,o.contact_id,o.customer_id,o.quotation_id,o.owner_id,o.stage_id,o.name,o.source,o.expected_value,o.close_date,o.status,o.win_loss_reason,s.name,o.created_by,o.created_at,o.updated_at FROM crm_opportunities o JOIN crm_pipeline_stages s ON s.id=o.stage_id WHERE o.id=$1`, out.ID), &out)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return out, err
}

func duplicateContact(err error) error {
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return ErrDuplicateContact
	}
	return err
}

func (r *Repository) Opportunity(ctx context.Context, scope Scope, id int64) (Opportunity, error) {
	var x Opportunity
	where, args := visibility(scope, "o.company_id", "o.owner_id")
	args = append(args, id)
	err := scanOpportunity(r.pool.QueryRow(ctx, `SELECT o.id,o.company_id,o.lead_id,o.contact_id,o.customer_id,o.quotation_id,o.owner_id,o.stage_id,o.name,o.source,o.expected_value,o.close_date,o.status,o.win_loss_reason,s.name,o.created_by,o.created_at,o.updated_at FROM crm_opportunities o JOIN crm_pipeline_stages s ON s.id=o.stage_id WHERE `+where+fmt.Sprintf(" AND o.id=$%d", len(args)), args...), &x)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return x, err
}

func (r *Repository) Move(ctx context.Context, scope Scope, id int64, stage Stage, reason string, actor int64) (Opportunity, error) {
	where, args := visibility(scope, "company_id", "owner_id")
	args = append(args, stage.ID, stage.Type, reason, id)
	stageArg := len(args) - 3
	tag, err := r.pool.Exec(ctx, `UPDATE crm_opportunities SET stage_id=$`+fmt.Sprint(stageArg)+`,status=$`+fmt.Sprint(len(args)-2)+`,win_loss_reason=$`+fmt.Sprint(len(args)-1)+`,updated_at=NOW() WHERE `+where+fmt.Sprintf(" AND id=$%d AND status='OPEN'", len(args))+` AND EXISTS (SELECT 1 FROM crm_pipeline_stages s WHERE s.id=$`+fmt.Sprint(stageArg)+` AND s.company_id=crm_opportunities.company_id)`, args...)
	if err != nil {
		return Opportunity{}, err
	}
	if tag.RowsAffected() != 1 {
		return Opportunity{}, ErrInvalidStage
	}
	details, _ := json.Marshal(map[string]any{"stage_id": stage.ID, "reason": reason})
	_, _ = r.pool.Exec(ctx, `INSERT INTO crm_events(company_id,entity_type,entity_id,event_type,actor_id,details) VALUES($1,'OPPORTUNITY',$2,$3,$4,$5)`, scope.CompanyID, id, stage.Type, actor, string(details))
	return r.Opportunity(ctx, scope, id)
}

func (r *Repository) AddActivity(ctx context.Context, scope Scope, in ActivityInput) (Activity, error) {
	if in.CompanyID != scope.CompanyID || (!scope.ViewAll && in.OwnerID != scope.UserID) {
		return Activity{}, ErrForbidden
	}
	var x Activity
	err := r.pool.QueryRow(ctx, `INSERT INTO crm_activities(company_id,lead_id,opportunity_id,contact_id,owner_id,activity_type,subject,body,due_at,reminder_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id,company_id,lead_id,opportunity_id,contact_id,owner_id,activity_type,subject,body,due_at,completed_at,reminder_at,reminder_sent_at,escalated_at,created_by,created_at`, in.CompanyID, in.LeadID, in.OpportunityID, in.ContactID, in.OwnerID, in.Type, in.Subject, in.Body, in.DueAt, in.ReminderAt, in.CreatedBy).Scan(&x.ID, &x.CompanyID, &x.LeadID, &x.OpportunityID, &x.ContactID, &x.OwnerID, &x.Type, &x.Subject, &x.Body, &x.DueAt, &x.CompletedAt, &x.ReminderAt, &x.ReminderSentAt, &x.EscalatedAt, &x.CreatedBy, &x.CreatedAt)
	return x, err
}

func (r *Repository) CompleteActivity(ctx context.Context, scope Scope, id, actor int64, at time.Time) error {
	where, args := visibility(scope, "company_id", "owner_id")
	args = append(args, at, id)
	tag, err := r.pool.Exec(ctx, `UPDATE crm_activities SET completed_at=$`+fmt.Sprint(len(args)-1)+`,updated_at=NOW() WHERE `+where+fmt.Sprintf(" AND id=$%d AND completed_at IS NULL", len(args)), args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO crm_events(company_id,entity_type,entity_id,event_type,actor_id) VALUES($1,'ACTIVITY',$2,'COMPLETED',$3)`, scope.CompanyID, id, actor)
	return err
}

func (r *Repository) Timeline(ctx context.Context, scope Scope, entity string, id int64) ([]Activity, []Event, error) {
	if !validEntity(entity) {
		return nil, nil, ErrInvalidInput
	}
	column := "opportunity_id"
	if entity == "LEAD" {
		column = "lead_id"
	}
	rows, err := r.pool.Query(ctx, `SELECT id,company_id,lead_id,opportunity_id,contact_id,owner_id,activity_type,subject,body,due_at,completed_at,reminder_at,reminder_sent_at,escalated_at,created_by,created_at FROM crm_activities WHERE company_id=$1 AND `+column+`=$2 AND ($3 OR owner_id=$4) ORDER BY created_at DESC`, scope.CompanyID, id, scope.ViewAll, scope.UserID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var activities []Activity
	for rows.Next() {
		var x Activity
		if err = rows.Scan(&x.ID, &x.CompanyID, &x.LeadID, &x.OpportunityID, &x.ContactID, &x.OwnerID, &x.Type, &x.Subject, &x.Body, &x.DueAt, &x.CompletedAt, &x.ReminderAt, &x.ReminderSentAt, &x.EscalatedAt, &x.CreatedBy, &x.CreatedAt); err != nil {
			return nil, nil, err
		}
		activities = append(activities, x)
	}
	eRows, err := r.pool.Query(ctx, `SELECT id,entity_type,entity_id,event_type,actor_id,details::text,created_at FROM crm_events WHERE company_id=$1 AND entity_type=$2 AND entity_id=$3 ORDER BY created_at DESC`, scope.CompanyID, entity, id)
	if err != nil {
		return nil, nil, err
	}
	defer eRows.Close()
	var events []Event
	for eRows.Next() {
		var x Event
		if err = eRows.Scan(&x.ID, &x.EntityType, &x.EntityID, &x.EventType, &x.ActorID, &x.Details, &x.CreatedAt); err != nil {
			return nil, nil, err
		}
		events = append(events, x)
	}
	return activities, events, eRows.Err()
}

func (r *Repository) Reassign(ctx context.Context, scope Scope, entity string, id, owner, actor int64) error {
	if !validEntity(entity) {
		return ErrInvalidInput
	}
	table := "crm_opportunities"
	if entity == "LEAD" {
		table = "crm_leads"
	} else if entity == "ACTIVITY" {
		table = "crm_activities"
	}
	where, args := visibility(scope, "company_id", "owner_id")
	args = append(args, owner, id)
	tag, err := r.pool.Exec(ctx, `UPDATE `+table+` SET owner_id=$`+fmt.Sprint(len(args)-1)+`,updated_at=NOW() WHERE `+where+fmt.Sprintf(" AND id=$%d", len(args)), args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO crm_events(company_id,entity_type,entity_id,event_type,actor_id,details) VALUES($1,$2,$3,'REASSIGNED',$4,jsonb_build_object('owner_id',$5::bigint))`, scope.CompanyID, entity, id, actor, owner)
	return err
}

func validEntity(entity string) bool {
	return entity == "LEAD" || entity == "OPPORTUNITY" || entity == "ACTIVITY"
}

func (r *Repository) DueActivities(ctx context.Context, now time.Time, limit int) ([]Activity, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.id,a.company_id,a.lead_id,a.opportunity_id,a.contact_id,a.owner_id,a.activity_type,a.subject,a.body,a.due_at,a.completed_at,a.reminder_at,a.reminder_sent_at,a.escalated_at,a.created_by,a.created_at,COALESCE(manager_user.id,a.owner_id) FROM crm_activities a LEFT JOIN hr_employees owner_employee ON owner_employee.user_id=a.owner_id AND owner_employee.company_id=a.company_id LEFT JOIN hr_employees manager_employee ON manager_employee.id=owner_employee.manager_id LEFT JOIN users manager_user ON manager_user.id=manager_employee.user_id AND manager_user.is_active=TRUE WHERE a.completed_at IS NULL AND ((a.reminder_at<=$1 AND a.reminder_sent_at IS NULL) OR (a.due_at<$1 AND a.escalated_at IS NULL)) ORDER BY COALESCE(a.reminder_at,a.due_at),a.id LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Activity
	for rows.Next() {
		var x Activity
		if err = rows.Scan(&x.ID, &x.CompanyID, &x.LeadID, &x.OpportunityID, &x.ContactID, &x.OwnerID, &x.Type, &x.Subject, &x.Body, &x.DueAt, &x.CompletedAt, &x.ReminderAt, &x.ReminderSentAt, &x.EscalatedAt, &x.CreatedBy, &x.CreatedAt, &x.EscalationRecipientID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) MarkReminder(ctx context.Context, id int64, escalated bool, at time.Time) error {
	column := "reminder_sent_at"
	if escalated {
		column = "escalated_at"
	}
	_, err := r.pool.Exec(ctx, `UPDATE crm_activities SET `+column+`=$2,updated_at=$2 WHERE id=$1 AND `+column+` IS NULL`, id, at)
	return err
}
func (r *Repository) CustomerByEmail(ctx context.Context, companyID int64, email string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM customers WHERE company_id=$1 AND LOWER(email)=LOWER($2) ORDER BY id LIMIT 1`, companyID, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}
func (r *Repository) LinkConversion(ctx context.Context, scope Scope, opportunityID, customerID, quotationID, actor int64) error {
	return pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE crm_opportunities o SET customer_id=$3,quotation_id=NULLIF($4,0),updated_at=NOW() WHERE o.id=$2 AND o.company_id=$1 AND ($5 OR o.owner_id=$6) AND o.status='WON' AND (o.customer_id IS NULL OR o.customer_id=$3) AND (o.quotation_id IS NULL OR o.quotation_id=NULLIF($4,0))`, scope.CompanyID, opportunityID, customerID, quotationID, scope.ViewAll, scope.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrNotFound
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_leads l SET converted_customer_id=$2,status='CONVERTED',updated_at=NOW() FROM crm_opportunities o WHERE o.id=$1 AND l.id=o.lead_id`, opportunityID, customerID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_contacts c SET customer_id=$2,updated_at=NOW() FROM crm_opportunities o WHERE o.id=$1 AND c.id=o.contact_id`, opportunityID, customerID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO crm_events(company_id,entity_type,entity_id,event_type,actor_id,details) SELECT $1,'OPPORTUNITY',$2,'CONVERTED',$3,jsonb_build_object('quotation_id',NULLIF($4,0)::bigint) WHERE NOT EXISTS (SELECT 1 FROM crm_events WHERE company_id=$1 AND entity_type='OPPORTUNITY' AND entity_id=$2 AND event_type='CONVERTED')`, scope.CompanyID, opportunityID, actor, quotationID)
		return err
	})
}

func (r *Repository) WinLoss(ctx context.Context, scope Scope) (WinLoss, error) {
	var out WinLoss
	where, args := visibility(scope, "company_id", "owner_id")
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FILTER(WHERE status='WON'),COALESCE(SUM(expected_value) FILTER(WHERE status='WON'),0),COUNT(*) FILTER(WHERE status='LOST'),COALESCE(SUM(expected_value) FILTER(WHERE status='LOST'),0) FROM crm_opportunities WHERE `+where, args...).Scan(&out.WonCount, &out.WonValue, &out.LostCount, &out.LostValue)
	if err != nil {
		return out, err
	}
	rows, err := r.pool.Query(ctx, `SELECT COALESCE(NULLIF(win_loss_reason,''),'Unspecified'),COUNT(*),SUM(expected_value) FROM crm_opportunities WHERE `+where+` AND status='LOST' GROUP BY 1 ORDER BY 2 DESC`, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var x ReasonTotal
		if err = rows.Scan(&x.Reason, &x.Count, &x.Value); err != nil {
			return out, err
		}
		out.Reasons = append(out.Reasons, x)
	}
	return out, rows.Err()
}
