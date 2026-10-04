package ap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// ErrActorMismatch is returned when the actor carried by a processing task
// contradicts the invoice's recorded creator.
var ErrActorMismatch = errors.New("ap: task actor does not match invoice creator")

type Orchestrator struct {
	matchingService  *MatchingService
	exceptionService *ExceptionService
	apService        *Service
	repo             Repository
	logger           *slog.Logger
}

func NewOrchestrator(ms *MatchingService, es *ExceptionService, as *Service, repo Repository) *Orchestrator {
	return &Orchestrator{
		matchingService:  ms,
		exceptionService: es,
		apService:        as,
		repo:             repo,
		logger:           slog.Default(),
	}
}

// ProcessInvoice runs matching for a newly created invoice and either records
// an exception or auto-posts it. It is safe under duplicate and concurrent
// delivery of the ap:invoice_process task:
//
//  1. A non-blocking per-invoice advisory lock serializes processing; a
//     concurrent duplicate returns nil immediately (if the holder crashes,
//     PostgreSQL releases the lock and asynq redelivers the holder's task).
//  2. Attribution integrity (not authorization): the actor recorded as run_by
//     and posted_by is the invoice's created_by. A payload actor that
//     contradicts a recorded creator returns ErrActorMismatch with no writes,
//     even for an already posted invoice. Legacy invoices with a NULL creator
//     keep the payload actor.
//  3. Only DRAFT invoices are processed. At most one matching run is created
//     per invoice by this path, and an exception is created only if none of
//     the same type exists for the same invoice and run, in any status, so a
//     redelivery after a user resolved an exception creates nothing new.
//
// Re-running matching after a data fix is not done here: there is no re-run
// producer (the task is enqueued only at invoice creation). A future
// "re-run matching" action must call MatchingService.RunMatch directly, which
// always creates a new run row; exceptions for that new run are keyed by the
// new run ID and are therefore not suppressed by this dedupe.
func (o *Orchestrator) ProcessInvoice(ctx context.Context, invoiceID, createdBy int64) error {
	release, acquired, err := o.repo.AcquireProcessingLock(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("acquire AP processing lock: %w", err)
	}
	if !acquired {
		o.logger.Info("AP invoice already processing; skipping duplicate", slog.Int64("invoice_id", invoiceID))
		return nil
	}
	defer release()

	inv, err := o.repo.GetAPInvoice(ctx, invoiceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrInvoiceNotFound) {
			return fmt.Errorf("%w: id %d", ErrInvoiceNotFound, invoiceID)
		}
		return fmt.Errorf("load AP invoice: %w", err)
	}

	actor := inv.CreatedBy
	if createdBy != 0 && actor != 0 && createdBy != actor {
		o.logger.Warn("AP invoice processing actor mismatch; rejecting task",
			slog.Int64("invoice_id", invoiceID),
			slog.Int64("payload_actor", createdBy),
			slog.Int64("invoice_created_by", actor))
		return fmt.Errorf("%w: invoice %d", ErrActorMismatch, invoiceID)
	}
	if actor == 0 {
		actor = createdBy
	}

	if inv.Status != APStatusDraft {
		return nil
	}

	run, err := o.repo.GetLatestMatchingRun(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("load latest matching run: %w", err)
	}
	if run == nil {
		matchRun, err := o.matchingService.RunMatch(ctx, invoiceID, actor)
		if err != nil {
			if errors.Is(err, ErrMatchingPolicyNotFound) {
				return o.createExceptionOnce(ctx, APException{
					APInvoiceID:   invoiceID,
					ExceptionType: "MISSING_MAPPING",
					Severity:      "HIGH",
					Reason:        "No matching policy found",
				})
			}
			return fmt.Errorf("failed to run match: %w", err)
		}
		run = &matchRun
	}

	return o.evaluateRun(ctx, invoiceID, actor, run)
}

func (o *Orchestrator) evaluateRun(ctx context.Context, invoiceID, actor int64, run *MatchingRun) error {
	runID := run.ID
	switch run.Status {
	case "EXCEPTION", "DUPLICATE_REVIEW":
		excType := "MISMATCH"
		if run.Status == "DUPLICATE_REVIEW" {
			excType = "DUPLICATE"
		}
		return o.createExceptionOnce(ctx, APException{
			APInvoiceID:     invoiceID,
			APMatchingRunID: &runID,
			ExceptionType:   excType,
			Severity:        "HIGH",
			Reason:          "Matching failed with variances or duplicate candidate",
		})
	case "MATCHED", "WITHIN_TOLERANCE":
		postErr := o.apService.PostAPInvoice(ctx, PostAPInvoiceInput{
			InvoiceID: invoiceID,
			PostedBy:  actor,
		})
		if postErr == nil {
			return nil
		}
		if excErr := o.createExceptionOnce(ctx, APException{
			APInvoiceID:     invoiceID,
			APMatchingRunID: &runID,
			ExceptionType:   "CLOSED_PERIOD", // Default guess, could be parsed from err
			Severity:        "HIGH",
			Reason:          fmt.Sprintf("Failed to post: %v", postErr),
		}); excErr != nil {
			return fmt.Errorf("failed to post invoice: %v (and %w)", postErr, excErr)
		}
		return nil
	}
	return nil
}

// createExceptionOnce creates exc unless an exception of the same type already
// exists for the same invoice and matching run, regardless of its status.
func (o *Orchestrator) createExceptionOnce(ctx context.Context, exc APException) error {
	exists, err := o.repo.ExceptionExists(ctx, exc.APInvoiceID, exc.APMatchingRunID, exc.ExceptionType)
	if err != nil {
		return fmt.Errorf("check existing AP exception: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := o.exceptionService.CreateException(ctx, exc); err != nil {
		return fmt.Errorf("failed to create exception: %w", err)
	}
	return nil
}
