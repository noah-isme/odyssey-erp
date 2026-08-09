package procurement

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ContractService handles business logic for supplier contracts
type ContractService struct {
	repo *ContractRepository
}

// NewContractService creates a new contract service
func NewContractService(db *pgxpool.Pool) *ContractService {
	return &ContractService{
		repo: NewContractRepository(db),
	}
}

// CreateContractDraft creates a new draft supplier contract
func (s *ContractService) CreateContractDraft(ctx context.Context, input CreateContractInput) (*SupplierContract, error) {
	contractID, err := s.repo.CreateContract(ctx, input)
	if err != nil {
		return nil, err
	}

	// Insert price lines if provided
	for i := range input.PriceLines {
		input.PriceLines[i].ContractID = contractID
		if err := s.repo.InsertContractPriceLine(ctx, input.PriceLines[i]); err != nil {
			return nil, fmt.Errorf("failed to insert price line: %w", err)
		}
	}

	// Fetch and return the created contract
	return s.repo.GetContract(ctx, contractID)
}

// ListContracts returns the company-scoped contract headers used by the
// procurement workbench. Price lines are intentionally loaded only when a
// single contract is requested.
func (s *ContractService) ListContracts(ctx context.Context, companyID, supplierID int64, status string, limit, offset int) ([]SupplierContract, error) {
	return s.repo.ListContracts(ctx, companyID, supplierID, status, limit, offset)
}

// ListPendingVariances returns unresolved PO contract exceptions for the
// current company.
func (s *ContractService) ListPendingVariances(ctx context.Context, companyID int64, limit, offset int) ([]POContractVariance, error) {
	return s.repo.ListPendingPOVariances(ctx, companyID, limit, offset)
}

// SubmitContractForApproval transitions contract to APPROVAL status
func (s *ContractService) SubmitContractForApproval(ctx context.Context, contractID int64) error {
	contract, err := s.repo.GetContract(ctx, contractID)
	if err != nil {
		return err
	}

	if contract.Status != ContractStatusDraft {
		return fmt.Errorf("can only submit DRAFT contracts for approval, current status: %s", contract.Status)
	}

	return s.repo.UpdateContractStatus(ctx, contractID, ContractStatusApproval)
}

// ApproveContract transitions contract to ACTIVE status
func (s *ContractService) ApproveContract(ctx context.Context, contractID int64, approvedBy int64) error {
	contract, err := s.repo.GetContract(ctx, contractID)
	if err != nil {
		return err
	}

	if contract.Status != ContractStatusApproval {
		return fmt.Errorf("can only approve contracts in APPROVAL status, current status: %s", contract.Status)
	}

	return s.repo.ApproveContract(ctx, contractID, approvedBy)
}

// RejectContract transitions contract back to DRAFT status
func (s *ContractService) RejectContract(ctx context.Context, contractID int64) error {
	contract, err := s.repo.GetContract(ctx, contractID)
	if err != nil {
		return err
	}

	if contract.Status != ContractStatusApproval {
		return fmt.Errorf("can only reject contracts in APPROVAL status, current status: %s", contract.Status)
	}

	return s.repo.UpdateContractStatus(ctx, contractID, ContractStatusDraft)
}

// TerminateContract transitions contract from ACTIVE to TERMINATED
func (s *ContractService) TerminateContract(ctx context.Context, contractID int64) error {
	contract, err := s.repo.GetContract(ctx, contractID)
	if err != nil {
		return err
	}

	if contract.Status != ContractStatusActive {
		return fmt.Errorf("can only terminate ACTIVE contracts, current status: %s", contract.Status)
	}

	return s.repo.TerminateContract(ctx, contractID)
}

// GetApplicableContractForPO finds the best-matching active contract for a purchase order line
// Returns nil if no applicable contract found (not an error)
func (s *ContractService) GetApplicableContractForPO(ctx context.Context, companyID, supplierID, productID int64, qty accountingmoney.Money) (*SupplierContract, error) {
	contracts, err := s.repo.ListActiveContracts(ctx, companyID, supplierID)
	if err != nil {
		return nil, err
	}

	for i := range contracts {
		if _, err := s.repo.GetApplicablePriceLine(ctx, contracts[i].ID, productID, qty); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		return &contracts[i], nil
	}
	return nil, nil // No active contract covers this product and quantity.
}

// GetPriceForPOLine retrieves the contract price and terms applicable to a PO line
// Returns nil if no applicable contract found
func (s *ContractService) GetPriceForPOLine(ctx context.Context, contractID, productID int64, qty accountingmoney.Money) (*ContractPriceLine, error) {
	return s.repo.GetApplicablePriceLine(ctx, contractID, productID, qty)
}

// CheckPOVariances identifies any deviations from contract terms for a purchase order
// Returns empty slice if no variances detected
func (s *ContractService) CheckPOVariances(ctx context.Context, input CheckPOVariancesInput) ([]POContractVariance, error) {
	var variances []POContractVariance

	// If no contract specified, check if one should have been used
	if input.ContractID == nil {
		applicableContract, err := s.GetApplicableContractForPO(ctx, input.CompanyID, input.SupplierID, input.ProductID, input.Quantity)
		if err != nil {
			return nil, err
		}

		if applicableContract != nil {
			// Should have used a contract
			variance := POContractVariance{
				CompanyID:      input.CompanyID,
				POID:           input.POID,
				POLineID:       input.POLineID,
				ContractID:     &applicableContract.ID,
				VarianceType:   VarianceTypeNoContract,
				VarianceReason: "No contract selected when an applicable contract exists",
				ApprovalStatus: ApprovalStatusPending,
			}
			variances = append(variances, variance)
		}
		return variances, nil
	}

	// Validate the specified contract
	contract, err := s.repo.GetContract(ctx, *input.ContractID)
	if err != nil {
		return nil, err
	}
	if contract.CompanyID != input.CompanyID || contract.SupplierID != input.SupplierID {
		return nil, fmt.Errorf("contract does not belong to the purchase order supplier/company")
	}

	// Check if contract is expired
	if contract.EffectiveTo != nil && contract.EffectiveTo.Before(time.Now()) {
		variance := POContractVariance{
			CompanyID:      input.CompanyID,
			POID:           input.POID,
			POLineID:       input.POLineID,
			ContractID:     input.ContractID,
			VarianceType:   VarianceTypeExpiredContract,
			VarianceReason: fmt.Sprintf("Contract expired on %s", contract.EffectiveTo.Format("2006-01-02")),
			ApprovalStatus: ApprovalStatusPending,
		}
		variances = append(variances, variance)
	}

	// Check if contract is not yet effective
	if contract.EffectiveFrom.After(time.Now()) {
		variance := POContractVariance{
			CompanyID:      input.CompanyID,
			POID:           input.POID,
			POLineID:       input.POLineID,
			ContractID:     input.ContractID,
			VarianceType:   VarianceTypeTermVariance,
			VarianceReason: fmt.Sprintf("Contract not yet effective (effective from %s)", contract.EffectiveFrom.Format("2006-01-02")),
			ApprovalStatus: ApprovalStatusPending,
		}
		variances = append(variances, variance)
	}

	// Check price variance if applicable contract price is known
	if len(input.POPrice) > 0 && len(contract.PriceLines) > 0 {
		priceLine, err := s.repo.GetApplicablePriceLine(ctx, *input.ContractID, input.ProductID, input.Quantity)
		if err == nil && priceLine != nil {
			poPrice, err := accountingmoney.Parse(input.POPrice, 4)
			if err != nil {
				return nil, fmt.Errorf("invalid PO price: %w", err)
			}

			// Calculate variance percentage
			contractPrice := priceLine.UnitPrice
			if contractPrice.Cmp(accountingmoney.Must("0", 0)) > 0 {
				variance := poPrice.Sub(contractPrice)
				if variance.Cmp(accountingmoney.Must("0", 0)) != 0 {
					variancePct, err := percentageDifference(variance, contractPrice)
					if err != nil {
						return nil, err
					}

					v := POContractVariance{
						CompanyID:          input.CompanyID,
						POID:               input.POID,
						POLineID:           input.POLineID,
						ContractID:         input.ContractID,
						VarianceType:       VarianceTypePriceVariance,
						VariancePercentage: &variancePct,
						VarianceReason:     fmt.Sprintf("PO price %s differs from contract price %s", poPrice.String(), contractPrice.String()),
						ApprovalStatus:     ApprovalStatusPending,
					}
					variances = append(variances, v)
				}
			}
		}
	}

	return variances, nil
}

// percentageDifference returns (difference / base) * 100 without passing
// through float64. PO and contract prices are NUMERIC values, so a rounded
// comparison here can otherwise approve a materially different purchase price.
func percentageDifference(difference, base accountingmoney.Money) (accountingmoney.Money, error) {
	baseRat, ok := new(big.Rat).SetString(base.String())
	if !ok || baseRat.Sign() == 0 {
		return accountingmoney.Money{}, fmt.Errorf("contract price must be greater than zero")
	}
	differenceRat, ok := new(big.Rat).SetString(difference.String())
	if !ok {
		return accountingmoney.Money{}, fmt.Errorf("invalid price difference")
	}
	percentage := new(big.Rat).Quo(differenceRat, baseRat)
	percentage.Mul(percentage, big.NewRat(100, 1))
	return accountingmoney.Parse(percentage.FloatString(2), 2)
}

// RecordContractPriceObservation records this contract into price history for trend tracking
func (s *ContractService) RecordContractPriceObservation(ctx context.Context, contractID int64) error {
	contract, err := s.repo.GetContract(ctx, contractID)
	if err != nil {
		return err
	}

	for _, line := range contract.PriceLines {
		input := RecordPriceHistoryInput{
			CompanyID:    contract.CompanyID,
			SupplierID:   contract.SupplierID,
			ProductID:    line.ProductID,
			SourceType:   PriceHistorySourceContract,
			SourceID:     contractID,
			Currency:     contract.Currency,
			UnitPrice:    line.UnitPrice,
			Quantity:     line.MinQuantity,
			TaxRate:      line.TaxRate,
			MOQ:          line.MOQ,
			LeadTimeDays: line.LeadTimeDays,
			Note:         fmt.Sprintf("Contract v%d effective %s", contract.Version, contract.EffectiveFrom.Format("2006-01-02")),
		}

		if _, err := s.repo.RecordPriceHistory(ctx, input); err != nil {
			return fmt.Errorf("failed to record price history for product %d: %w", line.ProductID, err)
		}
	}

	return nil
}

// CheckPOVariancesInput collects information needed to detect PO variances
type CheckPOVariancesInput struct {
	CompanyID  int64
	POID       int64
	POLineID   int64
	SupplierID int64
	ProductID  int64
	ContractID *int64
	Quantity   accountingmoney.Money
	POPrice    string // NUMERIC value as string to preserve precision
}
