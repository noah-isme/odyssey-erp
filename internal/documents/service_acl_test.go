package documents

import (
	"testing"
	"time"
)

func TestEvaluateACLsHonoursExpiryAndDenyPrecedence(t *testing.T) {
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	userID := int64(7)
	roleID := int64(11)
	expired := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	acls := []DocumentACL{
		{PrincipalType: "USER", PrincipalID: &userID, Permission: "READ", Effect: "ALLOW", ExpiresAt: &expired},
		{PrincipalType: "ROLE", PrincipalID: &roleID, Permission: "READ", Effect: "ALLOW", ExpiresAt: &future},
		{PrincipalType: "USER", PrincipalID: &userID, Permission: "READ", Effect: "DENY", ExpiresAt: &future},
	}
	allowed, matched, denied := evaluateACLs(acls, []int64{roleID}, userID, "read", now)
	if allowed || !matched || !denied {
		t.Fatalf("evaluateACLs() = allowed=%t matched=%t denied=%t, want false/true/true", allowed, matched, denied)
	}
}

func TestEvaluateACLsMatchesPublicAndRole(t *testing.T) {
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	roleID := int64(11)
	acls := []DocumentACL{
		{PrincipalType: "ROLE", PrincipalID: &roleID, Permission: "WRITE", Effect: "ALLOW"},
	}
	allowed, matched, denied := evaluateACLs(acls, []int64{roleID}, 99, "WRITE", now)
	if !allowed || !matched || denied {
		t.Fatalf("role ACL = allowed=%t matched=%t denied=%t, want true/true/false", allowed, matched, denied)
	}
	public := []DocumentACL{{PrincipalType: "PUBLIC", Permission: "READ", Effect: "ALLOW"}}
	allowed, matched, denied = evaluateACLs(public, nil, 0, "READ", now)
	if !allowed || !matched || denied {
		t.Fatalf("public ACL = allowed=%t matched=%t denied=%t, want true/true/false", allowed, matched, denied)
	}
}
