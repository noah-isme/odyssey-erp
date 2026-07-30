package payroll

import (
	"context"
	"errors"
)

// OutboxDispatcher retries payslip jobs whose durable payroll_payslips row has
// not yet been marked delivered. A failed item does not prevent later items.
type OutboxDispatcher struct {
	store    Store
	delivery PayslipDelivery
}

func NewOutboxDispatcher(store Store, delivery PayslipDelivery) *OutboxDispatcher {
	return &OutboxDispatcher{store: store, delivery: delivery}
}

func (d *OutboxDispatcher) DispatchPending(ctx context.Context) error {
	if d == nil || d.store == nil || d.delivery == nil {
		return nil
	}
	lines, err := d.store.PendingPayslips(ctx, 0)
	if err != nil {
		return err
	}
	var dispatchErrors []error
	for _, line := range lines {
		if err = d.delivery.EnqueuePayslip(ctx, line); err != nil {
			dispatchErrors = append(dispatchErrors, err)
		}
	}
	return errors.Join(dispatchErrors...)
}
