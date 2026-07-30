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
	opp, err := repo.Qualify(ctx, owner, QualifyInput{LeadID: lead.ID, ActorID: ownerID, ExpectedValue: 1500000})
	if err != nil {
		t.Fatal(err)
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
	manager := Scope{CompanyID: companyID, UserID: managerID, ViewAll: true}
	if err = repo.Reassign(ctx, manager, "OPPORTUNITY", opp.ID, managerID, managerID); err != nil {
		t.Fatal(err)
	}
	pipeline, err := repo.Pipeline(ctx, manager)
	if err != nil || len(pipeline.Opportunities) != 1 || pipeline.Opportunities[0].OwnerID != managerID {
		t.Fatalf("pipeline=%+v err=%v", pipeline, err)
	}
}
