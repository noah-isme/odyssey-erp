package ap

import (
	"context"
	"errors"
	"math"
)

var (
	ErrMatchingPolicyNotFound = errors.New("no matching policy found for invoice")
)

type MatchingService struct {
	repo Repository
}

func NewMatchingService(repo Repository) *MatchingService {
	return &MatchingService{repo: repo}
}

func (s *MatchingService) RunMatch(ctx context.Context, invoiceID int64, runBy int64) (MatchingRun, error) {
	// Need to get invoice with details
	invWithDetails, err := s.repo.GetAPInvoiceWithDetails(ctx, invoiceID)
	if err != nil {
		return MatchingRun{}, err
	}

	// 1. Resolve Policy
	// Suppose invoice belongs to a company, we don't have company_id on APInvoice, so pass nil for now.
	supplierIDPtr := &invWithDetails.SupplierID
	policy, err := s.repo.GetActiveMatchingPolicy(ctx, nil, supplierIDPtr, nil)
	if err != nil {
		// fallback to global policy if possible
		return MatchingRun{}, ErrMatchingPolicyNotFound
	}

	// 2. We need PO line progress to compare with invoice lines
	var progress map[int64]*POLineProgress
	if invWithDetails.POID != nil {
		progress, err = s.repo.GetPOLineProgressByPO(ctx, *invWithDetails.POID)
		if err != nil {
			return MatchingRun{}, err
		}
	} else {
		// If no PO, it's an exception (unbacked invoice). Or we just mark as EXCEPTION.
		progress = make(map[int64]*POLineProgress)
	}

	run := MatchingRun{
		APInvoiceID:       invoiceID,
		PolicyID:          &policy.ID,
		InvoiceTotal:      invWithDetails.Total,
		Status:            "MATCHED",
		ActionRecommended: "AUTO_POST",
	}

	// Summaries
	var poTotal, grnTotal float64

	// 3. For each invoice line, check tolerance against po_line_progress
	for _, line := range invWithDetails.Lines {
		runLine := MatchingRunLine{
			APInvoiceLineID: line.ID,
			POLineID:        line.POLineID,
			GRNLineID:       line.GRNLineID,
			InvoiceQty:      line.Quantity,
			InvoicePrice:    line.UnitPrice,
			Status:          "MATCHED",
		}

		var p *POLineProgress
		if line.POLineID != nil {
			p = progress[*line.POLineID]
		}

		if p == nil {
			runLine.Status = "EXCEPTION"
			runLine.Reasons = append(runLine.Reasons, "NO_PO_LINE_LINK")
			run.Status = "EXCEPTION"
		} else {
			poQty := p.OrderedQty
			poPrice := p.UnitPrice
			runLine.POQty = &poQty
			runLine.POPrice = &poPrice
			poTotal += (poQty * poPrice)

			if p.ReceivedQty > 0 {
				grnQty := float64(p.ReceivedQty)
				runLine.GRNQty = &grnQty
				grnTotal += (grnQty * poPrice)
			}

			// Tolerance logic (simplified version for now)
			qtyDiff := math.Abs(runLine.InvoiceQty - poQty)
			qtyDiffPct := 0.0
			if poQty > 0 {
				qtyDiffPct = (qtyDiff / poQty) * 100
			}

			if qtyDiffPct > policy.QtyTolerancePct {
				runLine.Status = "EXCEPTION"
				runLine.Reasons = append(runLine.Reasons, "QTY_VARIANCE")
				run.Status = "EXCEPTION"
			} else if qtyDiffPct > 0 {
				if runLine.Status == "MATCHED" {
					runLine.Status = "WITHIN_TOLERANCE"
				}
				if run.Status == "MATCHED" {
					run.Status = "WITHIN_TOLERANCE"
				}
			}

			priceDiff := math.Abs(runLine.InvoicePrice - poPrice)
			priceDiffPct := 0.0
			if poPrice > 0 {
				priceDiffPct = (priceDiff / poPrice) * 100
			}

			if priceDiffPct > policy.PriceTolerancePct {
				runLine.Status = "EXCEPTION"
				runLine.Reasons = append(runLine.Reasons, "PRICE_VARIANCE")
				run.Status = "EXCEPTION"
			} else if priceDiffPct > 0 {
				if runLine.Status == "MATCHED" {
					runLine.Status = "WITHIN_TOLERANCE"
				}
				if run.Status == "MATCHED" {
					run.Status = "WITHIN_TOLERANCE"
				}
			}
		}

		run.Lines = append(run.Lines, runLine)
	}

	run.POTotal = &poTotal
	run.GRNTotal = &grnTotal

	// Total tolerance check
	totalDiff := math.Abs(run.InvoiceTotal - poTotal)
	if totalDiff > policy.TotalToleranceAmt {
		run.Status = "EXCEPTION"
		run.Reasons = append(run.Reasons, "TOTAL_VARIANCE")
	} else if totalDiff > 0 {
		if run.Status == "MATCHED" {
			run.Status = "WITHIN_TOLERANCE"
		}
	}

	if run.Status == "EXCEPTION" {
		run.ActionRecommended = "REVIEW_REQUIRED"
	}

	// Persist the run
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		runID, err := tx.CreateMatchingRun(ctx, run)
		if err != nil {
			return err
		}
		run.ID = runID
		for i := range run.Lines {
			run.Lines[i].MatchingRunID = runID
			if err := tx.CreateMatchingRunLine(ctx, run.Lines[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return MatchingRun{}, err
	}

	return run, nil
}
