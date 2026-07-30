package tax

import (
	"testing"
	"time"
)

func TestSourceDigestIsStableAndSourceSpecific(t *testing.T) {
	s := sourceSnapshot{companyID: 1, sourceID: 9, sourceType: "AR_INVOICE", number: "INV-9", kind: "INVOICE", direction: "OUTPUT", counterpartyName: "Buyer", counterpartyTaxID: "123", postedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), base: 1000, vat: 110, gross: 1110, sign: 1}
	first := sourceDigest(s)
	if first != sourceDigest(s) {
		t.Fatal("digest is not deterministic")
	}
	mutations := map[string]func(*sourceSnapshot){
		"source type":         func(v *sourceSnapshot) { v.sourceType = "AR_CREDIT_NOTE" },
		"kind":                func(v *sourceSnapshot) { v.kind = "CREDIT_NOTE" },
		"direction":           func(v *sourceSnapshot) { v.direction = "INPUT" },
		"counterparty name":   func(v *sourceSnapshot) { v.counterpartyName = "Other Buyer" },
		"counterparty tax ID": func(v *sourceSnapshot) { v.counterpartyTaxID = "456" },
	}
	for name, mutate := range mutations {
		changed := s
		mutate(&changed)
		if first == sourceDigest(changed) {
			t.Fatalf("%s is absent from digest", name)
		}
	}
}
