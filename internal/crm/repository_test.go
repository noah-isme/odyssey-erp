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
	where, args := visibility(Scope{CompanyID: 4, UserID: 9}, "owner_id")
	if where != "company_id=$1 AND owner_id=$2" || len(args) != 2 {
		t.Fatalf("owned where=%q args=%v", where, args)
	}
	where, args = visibility(Scope{CompanyID: 4, UserID: 9, ViewAll: true}, "owner_id")
	if where != "company_id=$1" || len(args) != 1 {
		t.Fatalf("team where=%q args=%v", where, args)
	}
}
