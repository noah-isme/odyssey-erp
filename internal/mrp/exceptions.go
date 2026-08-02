package mrp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Exception struct {
	ID, CompanyID                                                      int64
	Fingerprint, Type, Severity, Status, ResolvedAction, Comment       string
	ProductID, WarehouseID, WorkOrderID, OperationID, RecommendationID *int64
	DueDate                                                            *time.Time
	Explanation                                                        json.RawMessage
	OwnerID, ResolvedBy                                                *int64
	ResolvedAt                                                         *time.Time
}
type ExceptionService struct{ pool *pgxpool.Pool }

func NewExceptionService(pool *pgxpool.Pool) *ExceptionService { return &ExceptionService{pool: pool} }
func (s *ExceptionService) Upsert(ctx context.Context, e Exception) error {
	if s == nil || s.pool == nil || e.CompanyID <= 0 || e.Fingerprint == "" || e.Type == "" || e.Severity == "" {
		return ErrInvalidState
	}
	if len(e.Explanation) == 0 {
		e.Explanation = []byte("{}")
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO mrp_exceptions(company_id,fingerprint,exception_type,severity,product_id,warehouse_id,work_order_id,operation_id,recommendation_id,due_date,explanation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb) ON CONFLICT(company_id,fingerprint) WHERE status IN ('OPEN','ASSIGNED') DO UPDATE SET severity=EXCLUDED.severity,due_date=EXCLUDED.due_date,explanation=EXCLUDED.explanation,updated_at=NOW()`, e.CompanyID, e.Fingerprint, e.Type, e.Severity, e.ProductID, e.WarehouseID, e.WorkOrderID, e.OperationID, e.RecommendationID, e.DueDate, string(e.Explanation))
	return err
}
func (s *ExceptionService) List(ctx context.Context, companyID int64, status, severity string) ([]Exception, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,company_id,fingerprint,exception_type,severity,status,COALESCE(resolved_action,''),comment,product_id,warehouse_id,work_order_id,operation_id,recommendation_id,due_date::timestamptz,explanation::text,owner_id,resolved_by,resolved_at FROM mrp_exceptions WHERE company_id=$1 AND ($2='' OR status=$2) AND ($3='' OR severity=$3) ORDER BY CASE severity WHEN 'CRITICAL' THEN 1 WHEN 'HIGH' THEN 2 WHEN 'MEDIUM' THEN 3 ELSE 4 END,due_date NULLS LAST,id DESC`, companyID, status, severity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exception
	for rows.Next() {
		var e Exception
		var explanation string
		if err := rows.Scan(&e.ID, &e.CompanyID, &e.Fingerprint, &e.Type, &e.Severity, &e.Status, &e.ResolvedAction, &e.Comment, &e.ProductID, &e.WarehouseID, &e.WorkOrderID, &e.OperationID, &e.RecommendationID, &e.DueDate, &explanation, &e.OwnerID, &e.ResolvedBy, &e.ResolvedAt); err != nil {
			return nil, err
		}
		e.Explanation = []byte(explanation)
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *ExceptionService) Act(ctx context.Context, companyID, actorID, id int64, action, comment string, ownerID *int64) error {
	status := "RESOLVED"
	if action == "assign" {
		status = "ASSIGNED"
	}
	if action == "dismiss" {
		status = "DISMISSED"
	}
	if action == "firm" || action == "reschedule" || action == "split" || action == "cancel" {
		status = "RESOLVED"
	}
	if status == "" {
		return ErrInvalidState
	}
	res, err := s.pool.Exec(ctx, `UPDATE mrp_exceptions SET status=$1,owner_id=COALESCE($2,owner_id),resolved_action=$3,comment=$4,resolved_by=CASE WHEN $1 IN ('RESOLVED','DISMISSED') THEN $5 ELSE NULL END,resolved_at=CASE WHEN $1 IN ('RESOLVED','DISMISSED') THEN NOW() ELSE NULL END,updated_at=NOW() WHERE id=$6 AND company_id=$7 AND status IN ('OPEN','ASSIGNED')`, status, ownerID, action, comment, actorID, id, companyID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func PlanningException(companyID int64, recommendationID *int64, r PlanningRecommendation) Exception {
	x, _ := json.Marshal(map[string]any{"demand_source": r.DemandSourceRef, "quantity": r.Quantity, "release_date": r.ReleaseDate, "due_date": r.DueDate})
	typ := "MATERIAL_SHORTAGE"
	if r.Late {
		typ = "LATE_SUPPLY"
	}
	return Exception{CompanyID: companyID, Fingerprint: fmt.Sprintf("planning:%s:%d:%d:%s", typ, r.ProductID, r.WarehouseID, r.DueDate.Format("2006-01-02")), Type: typ, Severity: map[bool]string{true: "HIGH", false: "MEDIUM"}[r.Late], ProductID: &r.ProductID, WarehouseID: &r.WarehouseID, RecommendationID: recommendationID, DueDate: &r.DueDate, Explanation: x}
}
