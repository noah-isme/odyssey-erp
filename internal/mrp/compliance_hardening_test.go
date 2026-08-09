package mrp

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestCanonicalizeJSONIsStableAndRejectsTrailingValues(t *testing.T) {
	canonical, err := canonicalizeJSON([]byte(`{"z":2,"a":{"b":1,"a":2},"n":1.0}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(canonical), `{"a":{"a":2,"b":1},"n":1.0,"z":2}`; got != want {
		t.Fatalf("canonical JSON = %s, want %s", got, want)
	}
	if _, err := canonicalizeJSON([]byte(`{"a":1}{"b":2}`)); err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	}
}

func TestCanonicalSnapshotHashAndRecordTypes(t *testing.T) {
	canonical, err := canonicalizeJSON([]byte(`{"record_id":7,"record_type":"BOM"}`))
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(canonical)
	if got, want := hashCanonicalJSON(canonical), hex.EncodeToString(expected[:]); got != want {
		t.Fatalf("hash = %s, want %s", got, want)
	}
	for _, recordType := range []string{"BOM", "WorkOrder", "mrp_work_order", "QUALITY_HOLD", "NCR", "CAPA"} {
		if _, err := normalizeRecordType(recordType); err != nil {
			t.Errorf("normalizeRecordType(%q): %v", recordType, err)
		}
	}
	if recordTypesEqual("WorkOrder", "BOM") {
		t.Fatal("different record types must not compare equal")
	}
}

func TestReauthenticationRequiresARealCredential(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if method, valid := verifyReauthenticationCredential(string(hash), false, "", ""); valid || method != "" {
		t.Fatal("empty reauthentication token must fail")
	}
	if method, valid := verifyReauthenticationCredential(string(hash), false, "", "wrong-password"); valid || method != "" {
		t.Fatal("incorrect password must fail")
	}
	if method, valid := verifyReauthenticationCredential(string(hash), false, "", "correct-password"); !valid || method != "PASSWORD" {
		t.Fatalf("password reauthentication = %q, %v; want PASSWORD, true", method, valid)
	}
}
