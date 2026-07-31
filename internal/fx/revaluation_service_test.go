package fx

import "testing"

func TestPaymentFXSourceKey(t *testing.T) {
	if got := PaymentFXSourceKey("AR", 12, 4); got != "AR_PAYMENT_FX:12:4" {
		t.Fatalf("got %q", got)
	}
}
