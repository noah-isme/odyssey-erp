//go:build integration

package crm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryCRMWorkflow(t *testing.T) {
	dsn := os.Getenv("CRM_TEST_DSN")
	if dsn == "" {
		t.Skip("CRM_TEST_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suffix := time.Now().UnixNano()
	var companyID, ownerID, managerID int64
	if err = pool.QueryRow(ctx, `INSERT INTO companies(code,name) VALUES($1,$2) RETURNING id`, fmt.Sprintf("CRM-%d", suffix), "CRM integration").Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name) VALUES($1,'test','Owner') RETURNING id`, fmt.Sprintf("owner-%d@example.test", suffix)).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name) VALUES($1,'test','Manager') RETURNING id`, fmt.Sprintf("manager-%d@example.test", suffix)).Scan(&managerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM companies WHERE id=$1`, companyID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, ownerID, managerID)
	})

	repo := NewRepository(pool)
	owner := Scope{CompanyID: companyID, UserID: ownerID}
	lead, err := repo.CreateLead(ctx, CreateLeadInput{CompanyID: companyID, OwnerID: ownerID, CreatedBy: ownerID, Name: "Ayu", Email: "ayu@example.test", Source: "WEBSITE"})
	if err != nil {
		t.Fatal(err)
	}
	opp, err := repo.Qualify(ctx, owner, QualifyInput{LeadID: lead.ID, ActorID: ownerID, ExpectedValue: 1500000.25})
	if err != nil {
		t.Fatal(err)
	}
	if opp.ExpectedValue != 1500000.25 {
		t.Fatalf("expected value lost precision: %v", opp.ExpectedValue)
	}
	duplicate, err := repo.CreateLead(ctx, CreateLeadInput{CompanyID: companyID, OwnerID: ownerID, CreatedBy: ownerID, Name: "Ayu duplicate", Email: "ayu@example.test", Source: "OTHER"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Qualify(ctx, owner, QualifyInput{LeadID: duplicate.ID, ActorID: ownerID}); !errors.Is(err, ErrDuplicateContact) {
		t.Fatalf("duplicate err=%v", err)
	}
	activity, err := repo.AddActivity(ctx, owner, ActivityInput{CompanyID: companyID, OwnerID: ownerID, CreatedBy: ownerID, OpportunityID: &opp.ID, Type: "CALL", Subject: "Discovery"})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CompleteActivity(ctx, owner, activity.ID, ownerID, time.Now()); err != nil {
		t.Fatal(err)
	}
	stages, err := repo.Stages(ctx, owner)
	if err != nil || len(stages) != 6 {
		t.Fatalf("stages=%d err=%v", len(stages), err)
	}
	if _, err = repo.Move(ctx, owner, opp.ID, stages[1], "", ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Move(ctx, owner, opp.ID, stages[4], "won", ownerID); err != nil {
		t.Fatal(err)
	}
	var customerID, quotationID int64
	if err = pool.QueryRow(ctx, `INSERT INTO customers(code,name,company_id,created_by) VALUES($1,'CRM customer',$2,$3) RETURNING id`, fmt.Sprintf("CRM-%d", suffix), companyID, ownerID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO quotations(doc_number,company_id,customer_id,quote_date,valid_until,created_by,crm_opportunity_id) VALUES($1,$2,$3,CURRENT_DATE,CURRENT_DATE,$4,$5) RETURNING id`, fmt.Sprintf("CRM-Q-%d", suffix), companyID, customerID, ownerID, opp.ID).Scan(&quotationID); err != nil {
		t.Fatal(err)
	}
	if err = repo.LinkConversion(ctx, owner, opp.ID, customerID, quotationID, ownerID); err != nil {
		t.Fatal(err)
	}
	converted, err := repo.Opportunity(ctx, owner, opp.ID)
	if err != nil || converted.CustomerID == nil || *converted.CustomerID != customerID || converted.QuotationID == nil || *converted.QuotationID != quotationID {
		t.Fatalf("converted=%+v err=%v", converted, err)
	}
	manager := Scope{CompanyID: companyID, UserID: managerID, ViewAll: true}
	if err = repo.Reassign(ctx, manager, "OPPORTUNITY", opp.ID, managerID, managerID); err != nil {
		t.Fatal(err)
	}
	pipeline, err := repo.Pipeline(ctx, manager)
	if err != nil || len(pipeline.Opportunities) != 1 || pipeline.Opportunities[0].OwnerID != managerID {
		t.Fatalf("pipeline=%+v err=%v", pipeline, err)
	}
}
