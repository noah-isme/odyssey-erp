package leave

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestTypesAndOwnRequestsMapRows(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &Service{pool: db}
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 2)
	db.ExpectQuery("SELECT id,code,name,default_days FROM hr_leave_types").WithArgs(int64(2)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "name", "default_days"}).AddRow(int64(1), "ANNUAL", "Annual", 12.0))
	types, err := service.Types(context.Background(), 2)
	if err != nil || len(types) != 1 || types[0].Code != "ANNUAL" {
		t.Fatalf("Types() = %#v, %v", types, err)
	}
	approvalID := int64(55)
	db.ExpectQuery("SELECT r.id,r.employee_id,r.leave_type_id,t.name").WithArgs(int64(9)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_id", "leave_type_id", "name", "start_date", "end_date", "days", "reason", "status", "approval_request_id"}).
			AddRow(int64(3), int64(8), int64(1), "Annual", start, end, 3.0, "Travel", "PENDING", &approvalID))
	requests, err := service.ListOwn(context.Background(), 9)
	if err != nil || len(requests) != 1 || requests[0].ApprovalRequestID == nil || *requests[0].ApprovalRequestID != 55 {
		t.Fatalf("ListOwn() = %#v, %v", requests, err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
