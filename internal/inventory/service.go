package inventory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// RepositoryPort abstracts repository usage for service.
type RepositoryPort interface {
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error
	GetStockCard(ctx context.Context, filter StockCardFilter) ([]StockCardEntry, error)
}

// AuditPort abstracts audit logging functionality.
type AuditPort interface {
	Record(ctx context.Context, log shared.AuditLog) error
}

// Service coordinates inventory operations.
type Service struct {
	repo        RepositoryPort
	audit       AuditPort
	idempotency *shared.IdempotencyStore
	allowNeg    bool
	integration IntegrationHandler
}

// ServiceConfig groups optional settings.
type ServiceConfig struct {
	AllowNegativeStock bool
}

// NewService builds Service.
func NewService(repo RepositoryPort, audit AuditPort, idem *shared.IdempotencyStore, cfg ServiceConfig, integration IntegrationHandler) *Service {
	return &Service{repo: repo, audit: audit, idempotency: idem, allowNeg: cfg.AllowNegativeStock, integration: integration}
}

// PostInbound posts an inbound movement (e.g. GRN).
func (s *Service) PostInbound(ctx context.Context, input InboundInput) (StockCardEntry, error) {
	if input.WarehouseID == 0 || input.ProductID == 0 {
		return StockCardEntry{}, errors.New("inventory: warehouse and product required")
	}
	if input.Qty <= 0 {
		return StockCardEntry{}, ErrInvalidQuantity
	}
	if input.UnitCost < 0 {
		return StockCardEntry{}, ErrInvalidUnitCost
	}
	params := movementParams{
		Code:        input.Code,
		WarehouseID: input.WarehouseID,
		ProductID:   input.ProductID,
		QtyChange:   input.Qty,
		UnitCost:    input.UnitCost,
		TxType:      TransactionTypeIn,
		Note:        input.Note,
		ActorID:     input.ActorID,
		RefModule:   input.RefModule,
		RefID:       input.RefID,
	}
	return s.postMovement(ctx, params)
}

// PostAdjustment posts an adjustment which may be positive or negative.
func (s *Service) PostAdjustment(ctx context.Context, input AdjustmentInput) (StockCardEntry, error) {
	if input.WarehouseID == 0 || input.ProductID == 0 {
		return StockCardEntry{}, errors.New("inventory: warehouse and product required")
	}
	if math.Abs(input.Qty) < 1e-9 {
		return StockCardEntry{}, ErrInvalidQuantity
	}
	if input.Qty > 0 && input.UnitCost < 0 {
		return StockCardEntry{}, ErrInvalidUnitCost
	}
	params := movementParams{
		Code:        input.Code,
		WarehouseID: input.WarehouseID,
		ProductID:   input.ProductID,
		QtyChange:   input.Qty,
		UnitCost:    input.UnitCost,
		TxType:      TransactionTypeAdjust,
		Note:        input.Note,
		ActorID:     input.ActorID,
		RefModule:   input.RefModule,
		RefID:       input.RefID,
	}
	entry, err := s.postMovement(ctx, params)
	if err != nil {
		return StockCardEntry{}, err
	}
	if s.integration != nil {
		evt := AdjustmentPostedEvent{
			Code:        entry.TxCode,
			WarehouseID: input.WarehouseID,
			ProductID:   input.ProductID,
			Qty:         input.Qty,
			UnitCost:    entry.UnitCost,
			PostedAt:    entry.PostedAt,
		}
		if err := s.integration.HandleInventoryAdjustmentPosted(ctx, evt); err != nil {
			return StockCardEntry{}, err
		}
	}
	return entry, nil
}

// PostTransfer moves stock between warehouses using OUT + IN.
func (s *Service) PostTransfer(ctx context.Context, input TransferInput) (StockCardEntry, StockCardEntry, error) {
	if input.SrcWarehouse == 0 || input.DstWarehouse == 0 || input.ProductID == 0 {
		return StockCardEntry{}, StockCardEntry{}, errors.New("inventory: warehouse and product required")
	}
	if input.SrcWarehouse == input.DstWarehouse {
		return StockCardEntry{}, StockCardEntry{}, errors.New("inventory: source and destination warehouse must differ")
	}
	if input.Qty <= 0 {
		return StockCardEntry{}, StockCardEntry{}, ErrInvalidQuantity
	}
	if input.UnitCost < 0 {
		return StockCardEntry{}, StockCardEntry{}, ErrInvalidUnitCost
	}
	outParams := movementParams{
		Code:        fmt.Sprintf("%s-OUT", baseCode(input.Code)),
		WarehouseID: input.SrcWarehouse,
		ProductID:   input.ProductID,
		QtyChange:   -input.Qty,
		UnitCost:    input.UnitCost,
		TxType:      TransactionTypeTransfer,
		Note:        fmt.Sprintf("Transfer to %d: %s", input.DstWarehouse, input.Note),
		ActorID:     input.ActorID,
		RefModule:   input.RefModule,
		RefID:       input.RefID,
	}
	inParams := movementParams{
		Code:        fmt.Sprintf("%s-IN", baseCode(input.Code)),
		WarehouseID: input.DstWarehouse,
		ProductID:   input.ProductID,
		QtyChange:   input.Qty,
		UnitCost:    input.UnitCost,
		TxType:      TransactionTypeTransfer,
		Note:        fmt.Sprintf("Transfer from %d: %s", input.SrcWarehouse, input.Note),
		ActorID:     input.ActorID,
		RefModule:   input.RefModule,
		RefID:       input.RefID,
	}
	outCard, err := s.postMovement(ctx, outParams)
	if err != nil {
		return StockCardEntry{}, StockCardEntry{}, err
	}
	inCard, err := s.postMovement(ctx, inParams)
	if err != nil {
		return StockCardEntry{}, StockCardEntry{}, err
	}
	return outCard, inCard, nil
}

// GetStockCard lists stock card entries.
func (s *Service) GetStockCard(ctx context.Context, filter StockCardFilter) ([]StockCardEntry, error) {
	if filter.WarehouseID == 0 || filter.ProductID == 0 {
		return nil, errors.New("inventory: warehouse and product required")
	}
	return s.repo.GetStockCard(ctx, filter)
}

type movementParams struct {
	Code        string
	WarehouseID int64
	ProductID   int64
	QtyChange   float64
	UnitCost    float64
	TxType      TransactionType
	Note        string
	ActorID     int64
	RefModule   string
	RefID       string
}

func (s *Service) postMovement(ctx context.Context, params movementParams) (StockCardEntry, error) {
	if params.QtyChange == 0 {
		return StockCardEntry{}, ErrInvalidQuantity
	}
	if params.WarehouseID == 0 || params.ProductID == 0 {
		return StockCardEntry{}, errors.New("inventory: warehouse and product required")
	}
	now := time.Now().UTC()
	code := params.Code
	if code == "" {
		code = fmt.Sprintf("INV-%d", now.UnixNano())
	}
	if params.RefID != "" {
		if _, err := uuid.Parse(params.RefID); err != nil {
			return StockCardEntry{}, fmt.Errorf("inventory: invalid ref id: %w", err)
		}
	}
	var card StockCardEntry
	key := fmt.Sprintf("%s:%s:%d:%d", params.TxType, code, params.WarehouseID, params.ProductID)
	insertedKey := false
	if s.idempotency != nil {
		if err := s.idempotency.CheckAndInsert(ctx, key, "inventory"); err != nil {
			return StockCardEntry{}, err
		}
		insertedKey = true
	}

	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		balance, err := tx.GetBalanceForUpdate(ctx, params.WarehouseID, params.ProductID)
		if err != nil && !errors.Is(err, ErrBalanceNotFound) {
			return err
		}
		if errors.Is(err, ErrBalanceNotFound) {
			balance = Balance{WarehouseID: params.WarehouseID, ProductID: params.ProductID}
		}
		qtyChange := params.QtyChange
		newQty := balance.Qty + qtyChange
		if !s.allowNeg && newQty < -0.0001 {
			return ErrNegativeStock
		}
		var unitCost float64
		var newAvg float64
		if qtyChange > 0 {
			unitCost = params.UnitCost
			totalCost := balance.Qty*balance.AvgCost + qtyChange*unitCost
			if newQty != 0 {
				newAvg = totalCost / newQty
			}
		} else {
			unitCost = balance.AvgCost
			if math.Abs(newQty) < 0.0001 {
				newQty = 0
			}
			if newQty <= 0 {
				newAvg = 0
			} else {
				newAvg = balance.AvgCost
			}
		}
		// When outbound and zero balance, ensure not negative unless allow
		if !s.allowNeg && newQty < -0.0001 {
			return ErrNegativeStock
		}
		txHeader := Transaction{
			Code:        code,
			Type:        params.TxType,
			WarehouseID: params.WarehouseID,
			RefModule:   params.RefModule,
			RefID:       params.RefID,
			Note:        params.Note,
			PostedAt:    now,
			CreatedBy:   params.ActorID,
		}
		txID, err := tx.InsertTransaction(ctx, txHeader)
		if err != nil {
			return err
		}
		line := TransactionLine{
			TransactionID: txID,
			ProductID:     params.ProductID,
			Qty:           qtyChange,
			UnitCost:      unitCost,
		}
		if qtyChange < 0 {
			line.SrcWarehouseID = params.WarehouseID
		} else {
			line.DstWarehouseID = params.WarehouseID
		}
		if err := tx.InsertTransactionLines(ctx, txID, []TransactionLine{line}); err != nil {
			return err
		}
		balance.Qty = newQty
		balance.AvgCost = newAvg
		if err := tx.UpsertBalance(ctx, balance); err != nil {
			return err
		}
		card = StockCardEntry{
			TxCode:      code,
			TxType:      params.TxType,
			PostedAt:    now,
			QtyIn:       math.Max(qtyChange, 0),
			QtyOut:      math.Max(-qtyChange, 0),
			BalanceQty:  newQty,
			UnitCost:    unitCost,
			BalanceCost: newAvg,
			Note:        params.Note,
		}
		if err := tx.InsertCardEntry(ctx, card, params.WarehouseID, params.ProductID, txID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if insertedKey {
			_ = s.idempotency.Delete(ctx, key)
		}
		return StockCardEntry{}, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, shared.AuditLog{
			ActorID:  params.ActorID,
			Action:   fmt.Sprintf("inventory:%s", params.TxType),
			Entity:   "inventory_tx",
			EntityID: fmt.Sprintf("%s:%d", params.TxType, params.ProductID),
			Meta: map[string]any{
				"warehouse_id": params.WarehouseID,
				"product_id":   params.ProductID,
				"qty":          params.QtyChange,
				"note":         params.Note,
			},
		})
	}
	return card, nil
}

func baseCode(code string) string {
	if code != "" {
		return code
	}
	return fmt.Sprintf("TRF-%d", time.Now().UnixNano())
}
