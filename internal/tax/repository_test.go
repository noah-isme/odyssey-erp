package tax

import (
	"testing"
	"time"
)

func TestSourceDigestIsStableAndSourceSpecific(t *testing.T) {
	s := sourceSnapshot{companyID: 1, sourceID: 9, sourceType: "AR_INVOICE", number: "INV-9", postedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), base: 1000, vat: 110, gross: 1110, sign: 1}
	first := sourceDigest(s)
	if first != sourceDigest(s) {
		t.Fatal("digest is not deterministic")
	}
	s.sourceType = "AR_CREDIT_NOTE"
	if first == sourceDigest(s) {
		t.Fatal("source type is absent from digest")
	}
}
