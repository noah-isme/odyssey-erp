package wms

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound        = errors.New("wms: not found")
	ErrInvalidState    = errors.New("wms: invalid state transition")
	ErrInvalidQuantity = errors.New("wms: invalid quantity")
)

type Bin struct {
	ID, CompanyID, WarehouseID int64
	Code, Name                 string
	Capacity                   *float64
	Active                     bool
}

type PickTask struct {
	ID, CompanyID, WaveID, ProductID int64
	RequestedQty, PickedQty          float64
	Status                           string
}

type BarcodeTarget struct {
	ProductID, BinID int64
}

type ScanResult struct {
	Task      PickTask
	Duplicate bool
	ScanID    int64
	ScannedAt time.Time
}

type Repository interface {
	CreateBin(context.Context, Bin) (Bin, error)
	CreateBarcode(context.Context, int64, string, int64, int64) error
	CreatePickTask(context.Context, PickTask) (PickTask, error)
	ResolveBarcode(context.Context, int64, string) (BarcodeTarget, error)
	GetPickTask(context.Context, int64, int64) (PickTask, error)
	HasScan(context.Context, int64, int64, string) (bool, error)
	RecordScan(context.Context, int64, int64, string, float64, int64, string) (int64, bool, error)
	UpdatePickTask(context.Context, PickTask) error
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreateBin(ctx context.Context, bin Bin) (Bin, error) {
	if s == nil || s.repo == nil {
		return Bin{}, errors.New("wms: repository is required")
	}
	if bin.CompanyID == 0 || bin.WarehouseID == 0 || bin.Code == "" || bin.Name == "" {
		return Bin{}, errors.New("wms: company, warehouse, code, and name are required")
	}
	return s.repo.CreateBin(ctx, bin)
}

func (s *Service) RegisterBarcode(ctx context.Context, companyID int64, barcode string, productID, binID int64) error {
	if s == nil || s.repo == nil {
		return errors.New("wms: repository is required")
	}
	if companyID == 0 || barcode == "" || (productID == 0 && binID == 0) {
		return errors.New("wms: company, barcode, and target are required")
	}
	return s.repo.CreateBarcode(ctx, companyID, barcode, productID, binID)
}

func (s *Service) CreatePickTask(ctx context.Context, task PickTask) (PickTask, error) {
	if s == nil || s.repo == nil {
		return PickTask{}, errors.New("wms: repository is required")
	}
	if task.CompanyID == 0 || task.WaveID == 0 || task.ProductID == 0 || task.RequestedQty <= 0 {
		return PickTask{}, ErrInvalidQuantity
	}
	task.Status = "OPEN"
	return s.repo.CreatePickTask(ctx, task)
}

func (s *Service) Transition(ctx context.Context, companyID, taskID int64, status string) (PickTask, error) {
	task, err := s.repo.GetPickTask(ctx, companyID, taskID)
	if err != nil {
		return PickTask{}, err
	}
	valid := (task.Status == "PICKED" && status == "PACKED") || (task.Status == "PACKED" && status == "SHIPPED")
	if !valid {
		return PickTask{}, fmt.Errorf("%w: %s to %s", ErrInvalidState, task.Status, status)
	}
	task.Status = status
	if err := s.repo.UpdatePickTask(ctx, task); err != nil {
		return PickTask{}, err
	}
	return task, nil
}

func (s *Service) Scan(ctx context.Context, companyID, taskID, actorID int64, barcode, idempotencyKey string, quantity float64) (ScanResult, error) {
	if s == nil || s.repo == nil {
		return ScanResult{}, errors.New("wms: repository is required")
	}
	if companyID == 0 || taskID == 0 || actorID == 0 || barcode == "" || idempotencyKey == "" || quantity <= 0 {
		return ScanResult{}, ErrInvalidQuantity
	}
	task, err := s.repo.GetPickTask(ctx, companyID, taskID)
	if err != nil {
		return ScanResult{}, err
	}
	if task.CompanyID != companyID {
		return ScanResult{}, ErrNotFound
	}
	seen, err := s.repo.HasScan(ctx, companyID, taskID, idempotencyKey)
	if err != nil {
		return ScanResult{}, err
	}
	if seen {
		return ScanResult{Task: task, Duplicate: true}, nil
	}
	if task.Status != "OPEN" && task.Status != "PICKING" && task.Status != "SHORT" {
		return ScanResult{}, fmt.Errorf("%w: task is %s", ErrInvalidState, task.Status)
	}
	target, err := s.repo.ResolveBarcode(ctx, companyID, barcode)
	if err != nil {
		return ScanResult{}, err
	}
	if target.ProductID != 0 && target.ProductID != task.ProductID {
		return ScanResult{}, fmt.Errorf("%w: barcode product does not match task", ErrNotFound)
	}
	if task.PickedQty+quantity > task.RequestedQty {
		return ScanResult{}, ErrInvalidQuantity
	}
	scanID, duplicate, err := s.repo.RecordScan(ctx, companyID, taskID, barcode, quantity, actorID, idempotencyKey)
	if err != nil {
		return ScanResult{}, err
	}
	if duplicate {
		return ScanResult{Task: task, Duplicate: true}, nil
	}
	task.PickedQty += quantity
	task.Status = "PICKING"
	if task.PickedQty == task.RequestedQty {
		task.Status = "PICKED"
	}
	if err := s.repo.UpdatePickTask(ctx, task); err != nil {
		return ScanResult{}, err
	}
	return ScanResult{Task: task, ScanID: scanID, ScannedAt: time.Now().UTC()}, nil
}
