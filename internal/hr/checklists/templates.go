package checklists

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Template maps an hr_checklist_templates row with its items (seq order).
type Template struct {
	ID        int64
	CompanyID int64
	Code      string
	Name      string
	Kind      string
	IsActive  bool
	CreatedAt time.Time
	Items     []Item
}

// Item maps an hr_checklist_template_items row.
type Item struct {
	ID                    int64
	CompanyID             int64
	TemplateID            int64
	Seq                   int
	Title                 string
	Description           string
	AssigneeRole          string
	DefaultAssigneeUserID *int64
	DefaultAssigneeName   string
	DueOffsetDays         int
	ActionKey             string // "" when NULL
}

// TemplateInput creates a template. Kind is fixed at creation.
type TemplateInput struct {
	Code     string
	Name     string
	Kind     string
	IsActive bool
}

// TemplateUpdate changes a template's code, name, and active flag.
type TemplateUpdate struct {
	Code     string
	Name     string
	IsActive bool
}

// ItemInput adds a template item. Seq 0 appends after the last item.
// DefaultAssigneeUserID is used for HR and IT items (and ignored for
// MANAGER items, which resolve to the employee's manager at start).
type ItemInput struct {
	Seq                   int
	Title                 string
	Description           string
	AssigneeRole          string
	DefaultAssigneeUserID *int64
	DueOffsetDays         int
	ActionKey             string
}

const templateCols = `t.id,t.company_id,t.code,t.name,t.kind,t.is_active,t.created_at`

func scanTemplate(row pgx.Row) (Template, error) {
	var t Template
	err := row.Scan(&t.ID, &t.CompanyID, &t.Code, &t.Name, &t.Kind, &t.IsActive, &t.CreatedAt)
	return t, err
}

// itemSelect reaches items through their company-owning template.
const itemSelect = `SELECT i.id,t.company_id,i.template_id,i.seq,i.title,i.description,i.assignee_role,i.default_assignee_user_id,COALESCE(u.name,''),i.due_offset_days,COALESCE(i.action_key,'')
FROM hr_checklist_template_items i
JOIN hr_checklist_templates t ON t.id=i.template_id AND t.company_id=$1
LEFT JOIN users u ON u.id=i.default_assignee_user_id`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	err := row.Scan(&it.ID, &it.CompanyID, &it.TemplateID, &it.Seq, &it.Title, &it.Description, &it.AssigneeRole, &it.DefaultAssigneeUserID, &it.DefaultAssigneeName, &it.DueOffsetDays, &it.ActionKey)
	return it, err
}

func collectItems(rows pgx.Rows) ([]Item, error) {
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ListTemplates returns the company's templates (by kind, then code) with
// their items.
func (s *Service) ListTemplates(ctx context.Context, companyID int64) ([]Template, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT `+templateCols+` FROM hr_checklist_templates t WHERE t.company_id=$1 ORDER BY t.kind, t.code`, companyID)
	if err != nil {
		return nil, fmt.Errorf("list checklist templates: %w", err)
	}
	var out []Template
	index := map[int64]int{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		index[t.ID] = len(out)
		out = append(out, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	itemRows, err := s.pool.Query(ctx, itemSelect+` ORDER BY i.template_id, i.seq`, companyID)
	if err != nil {
		return nil, fmt.Errorf("list checklist items: %w", err)
	}
	items, err := collectItems(itemRows)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if i, ok := index[it.TemplateID]; ok {
			out[i].Items = append(out[i].Items, it)
		}
	}
	return out, nil
}

// CreateTemplate inserts a template, audited CHECKLIST_TEMPLATE_CREATED.
func (s *Service) CreateTemplate(ctx context.Context, companyID int64, in TemplateInput, actorID int64) (Template, error) {
	if companyID <= 0 {
		return Template{}, ErrCompanyRequired
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.Kind = strings.ToUpper(strings.TrimSpace(in.Kind))
	if in.Code == "" || in.Name == "" {
		return Template{}, ErrInvalidTemplate
	}
	if !validKind(in.Kind) {
		return Template{}, ErrInvalidKind
	}
	var out Template
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		t, err := scanTemplate(tx.QueryRow(ctx, `INSERT INTO hr_checklist_templates AS t(company_id,code,name,kind,is_active) VALUES($1,$2,$3,$4,$5) RETURNING `+templateCols, companyID, in.Code, in.Name, in.Kind, in.IsActive))
		if isUniqueViolation(err) {
			return ErrDuplicateCode
		}
		if err != nil {
			return fmt.Errorf("insert checklist template: %w", err)
		}
		out = t
		return s.record(ctx, tx, actorID, ActionTemplateCreated, entityTemplate, t.ID, map[string]any{"code": t.Code, "kind": t.Kind})
	})
	return out, err
}

// UpdateTemplate changes code, name, and active flag, audited
// CHECKLIST_TEMPLATE_UPDATED. The kind cannot change.
func (s *Service) UpdateTemplate(ctx context.Context, companyID, templateID int64, in TemplateUpdate, actorID int64) (Template, error) {
	if companyID <= 0 {
		return Template{}, ErrCompanyRequired
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	if templateID <= 0 || in.Code == "" || in.Name == "" {
		return Template{}, ErrInvalidTemplate
	}
	var out Template
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		t, err := scanTemplate(tx.QueryRow(ctx, `UPDATE hr_checklist_templates t SET code=$3, name=$4, is_active=$5 WHERE t.id=$1 AND t.company_id=$2 RETURNING `+templateCols, templateID, companyID, in.Code, in.Name, in.IsActive))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTemplateNotFound
		}
		if isUniqueViolation(err) {
			return ErrDuplicateCode
		}
		if err != nil {
			return fmt.Errorf("update checklist template: %w", err)
		}
		out = t
		return s.record(ctx, tx, actorID, ActionTemplateUpdated, entityTemplate, t.ID, map[string]any{"code": t.Code, "is_active": t.IsActive})
	})
	return out, err
}

// AddItem appends an item to a company template, audited
// CHECKLIST_TEMPLATE_ITEM_ADDED. A default assignee must be an active user.
func (s *Service) AddItem(ctx context.Context, companyID, templateID int64, in ItemInput, actorID int64) (Item, error) {
	if companyID <= 0 {
		return Item{}, ErrCompanyRequired
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.AssigneeRole = strings.ToUpper(strings.TrimSpace(in.AssigneeRole))
	in.ActionKey = strings.ToUpper(strings.TrimSpace(in.ActionKey))
	switch {
	case templateID <= 0 || in.Title == "" || in.Seq < 0:
		return Item{}, ErrInvalidItem
	case !validRole(in.AssigneeRole):
		return Item{}, ErrInvalidRole
	case in.ActionKey != "" && !validActionKey(in.ActionKey):
		return Item{}, ErrInvalidActionKey
	}
	if in.DefaultAssigneeUserID != nil && *in.DefaultAssigneeUserID <= 0 {
		in.DefaultAssigneeUserID = nil
	}
	var actionKey *string
	if in.ActionKey != "" {
		actionKey = &in.ActionKey
	}
	var out Item
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var tid int64
		err := tx.QueryRow(ctx, `SELECT id FROM hr_checklist_templates WHERE id=$1 AND company_id=$2 FOR UPDATE`, templateID, companyID).Scan(&tid)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTemplateNotFound
		}
		if err != nil {
			return fmt.Errorf("lock checklist template: %w", err)
		}
		if in.DefaultAssigneeUserID != nil {
			if err := requireActiveUser(ctx, tx, *in.DefaultAssigneeUserID); err != nil {
				return err
			}
		}
		seq := in.Seq
		if seq == 0 {
			if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM hr_checklist_template_items WHERE template_id=$1`, templateID).Scan(&seq); err != nil {
				return fmt.Errorf("next item seq: %w", err)
			}
		}
		out = Item{CompanyID: companyID, TemplateID: templateID, Seq: seq, Title: in.Title, Description: in.Description, AssigneeRole: in.AssigneeRole,
			DefaultAssigneeUserID: in.DefaultAssigneeUserID, DueOffsetDays: in.DueOffsetDays, ActionKey: in.ActionKey}
		err = tx.QueryRow(ctx, `INSERT INTO hr_checklist_template_items(company_id,template_id,seq,title,description,assignee_role,default_assignee_user_id,due_offset_days,action_key)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, companyID, templateID, seq, in.Title, in.Description, in.AssigneeRole, in.DefaultAssigneeUserID, in.DueOffsetDays, actionKey).Scan(&out.ID)
		if isUniqueViolation(err) {
			return ErrDuplicateSeq
		}
		if err != nil {
			return fmt.Errorf("insert checklist item: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionTemplateItemAdded, entityTemplate, templateID, map[string]any{"item_id": out.ID, "seq": seq, "assignee_role": in.AssigneeRole})
	})
	return out, err
}

// DeleteItem removes a template item (reached through its company
// template), audited CHECKLIST_TEMPLATE_ITEM_DELETED. Tasks already created
// from the item are unaffected.
func (s *Service) DeleteItem(ctx context.Context, companyID, itemID, actorID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if itemID <= 0 {
		return ErrInvalidItem
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var templateID int64
		err := tx.QueryRow(ctx, `DELETE FROM hr_checklist_template_items i USING hr_checklist_templates t WHERE i.id=$1 AND t.id=i.template_id AND t.company_id=$2 RETURNING i.template_id`, itemID, companyID).Scan(&templateID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrItemNotFound
		}
		if err != nil {
			return fmt.Errorf("delete checklist item: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionTemplateItemDeleted, entityTemplate, templateID, map[string]any{"item_id": itemID})
	})
}

func requireActiveUser(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID int64) error {
	var active bool
	err := q.QueryRow(ctx, `SELECT is_active FROM users WHERE id=$1`, userID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return ErrAssigneeInactive
	}
	if err != nil {
		return fmt.Errorf("check assignee: %w", err)
	}
	return nil
}
