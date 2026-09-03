package shared

import (
	"context"
	"testing"
)

func TestIdentityFromContextRequiresAuthenticatedTenant(t *testing.T) {
	if _, ok := IdentityFromContext(context.Background()); ok {
		t.Fatal("background context must not produce an identity")
	}
	session := &Session{}
	session.SetUser("42")
	session.Set("company_id", "7")
	identity, ok := IdentityFromContext(ContextWithSession(context.Background(), session))
	if !ok || identity.UserID != 42 || identity.CompanyID != 7 {
		t.Fatalf("identity=%+v ok=%v", identity, ok)
	}

	session.Set("company_id", "not-a-company")
	if _, ok := IdentityFromContext(ContextWithSession(context.Background(), session)); ok {
		t.Fatal("invalid company id must be rejected")
	}
}
