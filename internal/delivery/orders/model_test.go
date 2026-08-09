package orders

import "testing"

func TestDeliveryStatusLifecycleRules(t *testing.T) {
	for _, status := range []Status{StatusDraft, StatusConfirmed, StatusInTransit, StatusDelivered, StatusCancelled} {
		if !status.IsValid() {
			t.Errorf("%q should be valid", status)
		}
	}
	if Status("UNKNOWN").IsValid() {
		t.Fatal("unknown status reported as valid")
	}
	if !StatusDraft.CanEdit() || !StatusDraft.CanConfirm() {
		t.Fatal("draft should be editable and confirmable")
	}
	if StatusConfirmed.CanEdit() || StatusConfirmed.CanConfirm() {
		t.Fatal("confirmed should not be editable or confirmable")
	}
	if !StatusConfirmed.CanCancel() || !StatusDraft.CanCancel() || StatusInTransit.CanCancel() {
		t.Fatal("unexpected cancellation rules")
	}
}

func TestReturnStatusLifecycleRules(t *testing.T) {
	for _, status := range []ReturnStatus{ReturnStatusDraft, ReturnStatusConfirmed, ReturnStatusCancelled} {
		if !status.IsValid() {
			t.Errorf("%q should be valid", status)
		}
	}
	if ReturnStatus("UNKNOWN").IsValid() {
		t.Fatal("unknown return status reported as valid")
	}
	if !ReturnStatusDraft.CanEdit() || !ReturnStatusDraft.CanConfirm() || !ReturnStatusConfirmed.CanCancel() {
		t.Fatal("unexpected return lifecycle rules")
	}
}
