package automation

import "errors"

var ErrIncompatiblePaymentDuties = errors.New("finance automation: incompatible payment duties")

// ValidatePaymentDutySeparation enforces the configured maker/checker and
// executor separation for a payment batch. It is called by the later payment
// workflow at proposal approval and execution time.
func ValidatePaymentDutySeparation(settings Settings, proposerID, approverID, executorID int64) error {
	if proposerID <= 0 || approverID <= 0 || executorID <= 0 {
		return ErrIncompatiblePaymentDuties
	}
	if settings.PaymentMakerCheckerEnabled && proposerID == approverID {
		return ErrIncompatiblePaymentDuties
	}
	if settings.PaymentExecutorSeparationEnabled && (proposerID == executorID || approverID == executorID) {
		return ErrIncompatiblePaymentDuties
	}
	return nil
}
