package performance

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
)

var lockParticipantCls = []string{"id", "cycle_id", "employee_id", "name", "status", "status", "self_review_due", "user_id", "reviewer_employee_id", "user_id", "manager_comments"}

const lockParticipantSQL = `FROM hr_performance_participants p
JOIN hr_performance_cycles c ON c.id=p.cycle_id AND c.company_id=$2`

type participantState struct {
	cycleStatus string
	stage       string
	selfDue     time.Time
	reviewer    bool
}

func activeEnrolled() participantState {
	return participantState{cycleStatus: CycleActive, stage: StageEnrolled, selfDue: selfDue, reviewer: true}
}

func expectLockParticipant(db pgxmock.PgxPoolIface, company int64, st participantState) {
	empUser, mgrEmp, mgrUser := employeeUser, managerEmpID, managerUser
	var reviewerEmp, reviewerUser *int64
	if st.reviewer {
		reviewerEmp, reviewerUser = &mgrEmp, &mgrUser
	}
	db.ExpectQuery(regexp.QuoteMeta(lockParticipantSQL)).WithArgs(participantID, company).
		WillReturnRows(pgxmock.NewRows(lockParticipantCls).AddRow(participantID, cycleID, employeeID, "2026 H1", st.cycleStatus, st.stage, st.selfDue, &empUser, reviewerEmp, reviewerUser, "Solid half"))
}

func TestSubmitSelfReviewAuthorizationAndStage(t *testing.T) {
	enrolled := activeEnrolled()
	selfSubmitted := activeEnrolled()
	selfSubmitted.stage = StageSelfSubmitted
	closed := activeEnrolled()
	closed.cycleStatus = CycleClosed
	cases := []struct {
		name    string
		actor   int64
		state   participantState
		wantErr error
	}{
		{name: "participant submits", actor: employeeUser, state: enrolled},
		{name: "manager cannot submit self review", actor: managerUser, state: enrolled, wantErr: ErrForbidden},
		{name: "admin cannot submit self review", actor: adminUser, state: enrolled, wantErr: ErrForbidden},
		{name: "already submitted", actor: employeeUser, state: selfSubmitted, wantErr: ErrInvalidStage},
		{name: "closed cycle", actor: employeeUser, state: closed, wantErr: ErrCycleNotActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit, _ := newMock(t)
			db.ExpectBegin()
			expectLockParticipant(db, companyA, tc.state)
			if tc.wantErr == nil {
				db.ExpectExec(regexp.QuoteMeta(`UPDATE hr_performance_goals SET self_score=$3 WHERE id=$1 AND participant_id=$2`)).
					WithArgs(goalID, participantID, 4).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				db.ExpectExec(regexp.QuoteMeta(`UPDATE hr_performance_participants SET status='SELF_SUBMITTED'`)).
					WithArgs(participantID, 4, "Delivered the plan").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				db.ExpectCommit()
			} else {
				db.ExpectRollback()
			}
			err := service.SubmitSelfReview(context.Background(), companyA, participantID, tc.actor,
				ReviewInput{Rating: 4, Comments: " Delivered the plan ", GoalScores: map[int64]int{goalID: 4}})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil {
				if got := audit.actions(); len(got) != 1 || got[0] != ActionSelfSubmitted || audit.logs[0].Meta["stage"] != StageSelfSubmitted {
					t.Fatalf("audit = %#v", audit.logs)
				}
				audit.assertNoSensitiveMeta(t)
			} else if len(audit.logs) != 0 {
				t.Fatalf("unexpected audit %v", audit.actions())
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSubmitSelfReviewRejectsForeignGoalAndBadRating(t *testing.T) {
	db, service, _, _ := newMock(t)
	if err := service.SubmitSelfReview(context.Background(), companyA, participantID, employeeUser, ReviewInput{Rating: 6}); !errors.Is(err, ErrInvalidRating) {
		t.Fatalf("rating err = %v", err)
	}
	db.ExpectBegin()
	expectLockParticipant(db, companyA, activeEnrolled())
	db.ExpectExec(regexp.QuoteMeta(`UPDATE hr_performance_goals SET self_score=$3`)).
		WithArgs(int64(999), participantID, 3).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectRollback()
	err := service.SubmitSelfReview(context.Background(), companyA, participantID, employeeUser, ReviewInput{Rating: 3, GoalScores: map[int64]int{999: 3}})
	if !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("foreign goal err = %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitManagerReviewAuthorizationAndStage(t *testing.T) {
	// The service clock is 2026-10-12; selfDue (2026-07-15) has passed.
	selfSubmitted := activeEnrolled()
	selfSubmitted.stage = StageSelfSubmitted
	enrolledBeforeDue := activeEnrolled()
	enrolledBeforeDue.selfDue = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	enrolledAfterDue := activeEnrolled()
	completed := activeEnrolled()
	completed.stage = StageCompleted
	noReviewer := selfSubmitted
	noReviewer.reviewer = false
	draft := selfSubmitted
	draft.cycleStatus = CycleDraft
	cases := []struct {
		name    string
		actor   int64
		isAdmin bool
		state   participantState
		wantErr error
	}{
		{name: "reviewer after self submission", actor: managerUser, state: selfSubmitted},
		{name: "admin who is not the reviewer", actor: adminUser, isAdmin: true, state: selfSubmitted},
		{name: "other manager forbidden", actor: otherUser, state: selfSubmitted, wantErr: ErrForbidden},
		{name: "participant cannot submit manager review", actor: employeeUser, state: selfSubmitted, wantErr: ErrForbidden},
		{name: "no reviewer, non-admin forbidden", actor: managerUser, state: noReviewer, wantErr: ErrForbidden},
		{name: "self review pending until due passes", actor: managerUser, state: enrolledBeforeDue, wantErr: ErrSelfReviewPending},
		{name: "self review overdue allows manager review", actor: managerUser, state: enrolledAfterDue},
		{name: "already completed", actor: managerUser, state: completed, wantErr: ErrInvalidStage},
		{name: "draft cycle", actor: managerUser, state: draft, wantErr: ErrCycleNotActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit, _ := newMock(t)
			db.ExpectBegin()
			expectLockParticipant(db, companyA, tc.state)
			if tc.wantErr == nil {
				db.ExpectExec(regexp.QuoteMeta(`UPDATE hr_performance_participants SET status='MANAGER_SUBMITTED'`)).
					WithArgs(participantID, 3, "Meets expectations").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				db.ExpectCommit()
			} else {
				db.ExpectRollback()
			}
			err := service.SubmitManagerReview(context.Background(), companyA, participantID, tc.actor, tc.isAdmin, ReviewInput{Rating: 3, Comments: "Meets expectations"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil {
				if got := audit.actions(); len(got) != 1 || got[0] != ActionManagerSubmitted {
					t.Fatalf("audit = %v", got)
				}
				audit.assertNoSensitiveMeta(t)
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGoalAuthorization(t *testing.T) {
	draft := activeEnrolled()
	draft.cycleStatus = CycleDraft
	cases := []struct {
		name    string
		actor   int64
		isAdmin bool
		state   participantState
		wantErr error
	}{
		{name: "participant adds goal", actor: employeeUser, state: activeEnrolled()},
		{name: "admin adds goal", actor: adminUser, isAdmin: true, state: activeEnrolled()},
		{name: "manager cannot add goal", actor: managerUser, state: activeEnrolled(), wantErr: ErrForbidden},
		{name: "draft cycle", actor: employeeUser, state: draft, wantErr: ErrCycleNotActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit, _ := newMock(t)
			db.ExpectBegin()
			expectLockParticipant(db, companyA, tc.state)
			if tc.wantErr == nil {
				db.ExpectQuery(regexp.QuoteMeta(`INSERT INTO hr_performance_goals(company_id,participant_id,seq,title,description,kpi_target,weight)`)).
					WithArgs(companyA, participantID, "Ship v2", "", "100% scope", "40").
					WillReturnRows(pgxmock.NewRows([]string{"id", "participant_id", "seq", "title", "description", "kpi_target", "weight", "self_score", "manager_score", "created_at"}).
						AddRow(goalID, participantID, 1, "Ship v2", "", "100% scope", "40.00", nil, nil, createdAt))
				db.ExpectCommit()
			} else {
				db.ExpectRollback()
			}
			g, err := service.AddGoal(context.Background(), companyA, participantID, tc.actor, tc.isAdmin, GoalInput{Title: " Ship v2 ", KPITarget: "100% scope", Weight: "40"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil {
				if g.ID != goalID || g.Seq != 1 {
					t.Fatalf("goal = %#v", g)
				}
				if got := audit.actions(); len(got) != 1 || got[0] != ActionGoalChanged || audit.logs[0].Meta["change"] != "added" {
					t.Fatalf("audit = %#v", audit.logs)
				}
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGoalValidationAndCrossTenant(t *testing.T) {
	db, service, _, _ := newMock(t)
	for _, in := range []GoalInput{{Title: " "}, {Title: "x", Weight: "101"}, {Title: "x", Weight: "-1"}, {Title: "x", Weight: "NaN"}, {Title: "x", Weight: "1.234"}, {Title: "x", Weight: "1e2"}} {
		if _, err := service.AddGoal(context.Background(), companyA, participantID, employeeUser, false, in); !errors.Is(err, ErrInvalidGoal) {
			t.Fatalf("AddGoal(%#v) err = %v", in, err)
		}
	}
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(`SELECT g.participant_id FROM hr_performance_goals g`)).WithArgs(goalID, companyB).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	if err := service.DeleteGoal(context.Background(), companyB, goalID, adminUser, true); !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("cross-tenant DeleteGoal err = %v", err)
	}

	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(`SELECT g.participant_id FROM hr_performance_goals g`)).WithArgs(goalID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"participant_id"}).AddRow(participantID))
	expectLockParticipant(db, companyA, activeEnrolled())
	db.ExpectExec(regexp.QuoteMeta(`UPDATE hr_performance_goals SET title=$2`)).WithArgs(goalID, "Ship v3", "", "", "12.50").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	if err := service.UpdateGoal(context.Background(), companyA, goalID, employeeUser, false, GoalInput{Title: "Ship v3", Weight: "12.50"}); err != nil {
		t.Fatalf("UpdateGoal err = %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteInsertsSummaryReviewWithCompanyID(t *testing.T) {
	db, service, audit, _ := newMock(t)
	st := activeEnrolled()
	st.stage = StageManagerSubmitted
	db.ExpectBegin()
	expectLockParticipant(db, companyA, st)
	db.ExpectQuery(regexp.QuoteMeta(`INSERT INTO hr_performance_reviews(company_id,employee_id,reviewer_id,review_period,rating,comments)`)).
		WithArgs(companyA, employeeID, managerEmpID, "2026 H1", 4, "Solid half").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(600)))
	db.ExpectExec(regexp.QuoteMeta(`UPDATE hr_performance_participants SET status='COMPLETED', final_rating=$2, completed_at=NOW(), review_id=$3 WHERE id=$1`)).
		WithArgs(participantID, 4, int64(600)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	reviewID, err := service.Complete(context.Background(), companyA, participantID, adminUser, 4)
	if err != nil || reviewID != 600 {
		t.Fatalf("Complete() = %d, %v", reviewID, err)
	}
	if got := audit.actions(); len(got) != 1 || got[0] != ActionCompleted || audit.logs[0].Meta["review_id"] != int64(600) {
		t.Fatalf("audit = %#v", audit.logs)
	}
	audit.assertNoSensitiveMeta(t)
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteStageAndTenantGuards(t *testing.T) {
	selfSubmitted := activeEnrolled()
	selfSubmitted.stage = StageSelfSubmitted
	done := activeEnrolled()
	done.stage = StageCompleted
	for name, st := range map[string]participantState{"self submitted": selfSubmitted, "already completed": done} {
		db, service, audit, _ := newMock(t)
		db.ExpectBegin()
		expectLockParticipant(db, companyA, st)
		db.ExpectRollback()
		if _, err := service.Complete(context.Background(), companyA, participantID, adminUser, 4); !errors.Is(err, ErrInvalidStage) {
			t.Fatalf("%s: err = %v", name, err)
		}
		if len(audit.logs) != 0 {
			t.Fatalf("%s: audit %v", name, audit.actions())
		}
		if err := db.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}

	db, service, _, _ := newMock(t)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(lockParticipantSQL)).WithArgs(participantID, companyB).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	if _, err := service.Complete(context.Background(), companyB, participantID, adminUser, 4); !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("cross-tenant err = %v", err)
	}
	if _, err := service.Complete(context.Background(), companyA, participantID, adminUser, 0); !errors.Is(err, ErrInvalidRating) {
		t.Fatalf("rating err = %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteFallsBackToActorEmployeeWhenNoReviewer(t *testing.T) {
	db, service, _, _ := newMock(t)
	st := activeEnrolled()
	st.stage = StageManagerSubmitted
	st.reviewer = false
	db.ExpectBegin()
	expectLockParticipant(db, companyA, st)
	db.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM hr_employees WHERE user_id=$1 AND company_id=$2`)).WithArgs(adminUser, companyA).
		WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	if _, err := service.Complete(context.Background(), companyA, participantID, adminUser, 4); !errors.Is(err, ErrReviewerRequired) {
		t.Fatalf("err = %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMyAndTeamReviewsScopeByCompanyAndUser(t *testing.T) {
	db, service, _, _ := newMock(t)
	db.ExpectQuery(regexp.QuoteMeta(`JOIN hr_performance_cycles c ON c.id=p.cycle_id AND c.company_id=$1`)+`(?s).*`+regexp.QuoteMeta(`WHERE e.user_id=$2 AND c.status<>'DRAFT'`)).
		WithArgs(companyA, employeeUser).WillReturnRows(pgxmock.NewRows([]string{"id"}))
	if _, err := service.MyReviews(context.Background(), companyA, employeeUser); err != nil {
		t.Fatal(err)
	}
	db.ExpectQuery(regexp.QuoteMeta(`WHERE r.user_id=$2 AND c.status<>'DRAFT'`)).
		WithArgs(companyA, managerUser).WillReturnRows(pgxmock.NewRows([]string{"id"}))
	if _, err := service.TeamReviews(context.Background(), companyA, managerUser); err != nil {
		t.Fatal(err)
	}
	if _, err := service.MyReviews(context.Background(), 0, employeeUser); !errors.Is(err, ErrCompanyRequired) {
		t.Fatalf("company 0 err = %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
