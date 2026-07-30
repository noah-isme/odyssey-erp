package crm

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDuplicateContactMapsUniqueViolation(t *testing.T) {
	err := duplicateContact(&pgconn.PgError{Code: "23505"})
	if !errors.Is(err, ErrDuplicateContact) {
		t.Fatalf("err=%v", err)
	}
}

func TestVisibilityScopesOwnerUnlessTeamAccess(t *testing.T) {
	where, args := visibility(Scope{CompanyID: 4, UserID: 9}, "o.company_id", "o.owner_id")
	if where != "o.company_id=$1 AND o.owner_id=$2" || len(args) != 2 {
		t.Fatalf("owned where=%q args=%v", where, args)
	}
	where, args = visibility(Scope{CompanyID: 4, UserID: 9, ViewAll: true}, "o.company_id", "o.owner_id")
	if where != "o.company_id=$1" || len(args) != 1 {
		t.Fatalf("team where=%q args=%v", where, args)
	}
}

func TestCRMEntityAllowList(t *testing.T) {
	for _, entity := range []string{"LEAD", "OPPORTUNITY", "ACTIVITY"} {
		if !validEntity(entity) {
			t.Fatalf("expected %s to be allowed", entity)
		}
	}
	if validEntity("lead; DROP TABLE crm_events") {
		t.Fatal("unexpected entity accepted")
	}
}
