package ap

import (
	"context"
	"errors"
	"fmt"
)

type Orchestrator struct {
	matchingService  *MatchingService
	exceptionService *ExceptionService
	apService        *Service
}

func NewOrchestrator(ms *MatchingService, es *ExceptionService, as *Service) *Orchestrator {
	return &Orchestrator{
		matchingService:  ms,
		exceptionService: es,
		apService:        as,
	}
}

func (o *Orchestrator) ProcessInvoice(ctx context.Context, invoiceID, createdBy int64) error {
	// 1. Run matching engine
	matchRun, err := o.matchingService.RunMatch(ctx, invoiceID, createdBy)
	if err != nil {
		if errors.Is(err, ErrMatchingPolicyNotFound) {
			// Missing mapping exception
			_, err = o.exceptionService.CreateException(ctx, APException{
				APInvoiceID:   invoiceID,
				ExceptionType: "MISSING_MAPPING",
				Severity:      "HIGH",
				Reason:        "No matching policy found",
			})
			if err != nil {
				return fmt.Errorf("failed to create exception: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to run match: %w", err)
	}

	// 2. Evaluate result
	if matchRun.Status == "EXCEPTION" || matchRun.Status == "DUPLICATE_REVIEW" {
		excType := "MISMATCH"
		if matchRun.Status == "DUPLICATE_REVIEW" {
			excType = "DUPLICATE"
		}
		_, err = o.exceptionService.CreateException(ctx, APException{
			APInvoiceID:     invoiceID,
			APMatchingRunID: &matchRun.ID,
			ExceptionType:   excType,
			Severity:        "HIGH",
			Reason:          "Matching failed with variances or duplicate candidate",
		})
		if err != nil {
			return fmt.Errorf("failed to create exception: %w", err)
		}
		return nil
	}

	// 3. Attempt auto-post
	if matchRun.Status == "MATCHED" || matchRun.Status == "WITHIN_TOLERANCE" {
		err = o.apService.PostAPInvoice(ctx, PostAPInvoiceInput{
			InvoiceID: invoiceID,
			PostedBy:  createdBy,
		})
		if err != nil {
			_, excErr := o.exceptionService.CreateException(ctx, APException{
				APInvoiceID:     invoiceID,
				APMatchingRunID: &matchRun.ID,
				ExceptionType:   "CLOSED_PERIOD", // Default guess, could be parsed from err
				Severity:        "HIGH",
				Reason:          fmt.Sprintf("Failed to post: %v", err),
			})
			if excErr != nil {
				return fmt.Errorf("failed to post invoice: %v (and failed to create exception: %v)", err, excErr)
			}
			return nil
		}
	}

	return nil
}
