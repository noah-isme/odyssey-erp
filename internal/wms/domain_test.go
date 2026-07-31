package wms

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryRepo struct {
	task       PickTask
	target     BarcodeTarget
	scans      map[string]int64
	updated    []PickTask
	nextScanID int64
}

func (m *memoryRepo) CreateBin(context.Context, Bin) (Bin, error)                      { return Bin{}, nil }
func (m *memoryRepo) CreateBarcode(context.Context, int64, string, int64, int64) error { return nil }
func (m *memoryRepo) CreatePickTask(_ context.Context, task PickTask) (PickTask, error) {
	m.task = task
	return task, nil
}
func (m *memoryRepo) ResolveBarcode(context.Context, int64, string) (BarcodeTarget, error) {
	return m.target, nil
}
func (m *memoryRepo) GetPickTask(context.Context, int64, int64) (PickTask, error) { return m.task, nil }
func (m *memoryRepo) HasScan(_ context.Context, _ int64, _ int64, key string) (bool, error) {
	_, ok := m.scans[key]
	return ok, nil
}
func (m *memoryRepo) RecordScan(_ context.Context, _ int64, _ int64, _ string, _ float64, _ int64, key string) (int64, bool, error) {
	if id, ok := m.scans[key]; ok {
		return id, true, nil
	}
	m.nextScanID++
	m.scans[key] = m.nextScanID
	return m.nextScanID, false, nil
}
func (m *memoryRepo) UpdatePickTask(_ context.Context, task PickTask) error {
	m.task = task
	m.updated = append(m.updated, task)
	return nil
}

func TestScanCompletesTaskAndIsIdempotent(t *testing.T) {
	repo := &memoryRepo{task: PickTask{ID: 7, CompanyID: 3, ProductID: 11, RequestedQty: 2, Status: "OPEN"}, target: BarcodeTarget{ProductID: 11}, scans: map[string]int64{}}
	svc := NewService(repo)
	result, err := svc.Scan(context.Background(), 3, 7, 9, "P-11", "scan-1", 2)
	require.NoError(t, err)
	require.Equal(t, "PICKED", result.Task.Status)
	require.Equal(t, 2.0, result.Task.PickedQty)
	duplicate, err := svc.Scan(context.Background(), 3, 7, 9, "P-11", "scan-1", 2)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Len(t, repo.updated, 1)
}

func TestScanRejectsWrongProduct(t *testing.T) {
	repo := &memoryRepo{task: PickTask{ID: 7, CompanyID: 3, ProductID: 11, RequestedQty: 2, Status: "OPEN"}, target: BarcodeTarget{ProductID: 99}, scans: map[string]int64{}}
	_, err := NewService(repo).Scan(context.Background(), 3, 7, 9, "P-99", "scan-1", 1)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestTransitionRequiresPickThenPackThenShip(t *testing.T) {
	repo := &memoryRepo{task: PickTask{ID: 7, CompanyID: 3, ProductID: 11, RequestedQty: 2, PickedQty: 2, Status: "PICKED"}, scans: map[string]int64{}}
	svc := NewService(repo)
	task, err := svc.Transition(context.Background(), 3, 7, "PACKED")
	require.NoError(t, err)
	require.Equal(t, "PACKED", task.Status)
	task, err = svc.Transition(context.Background(), 3, 7, "SHIPPED")
	require.NoError(t, err)
	require.Equal(t, "SHIPPED", task.Status)
}
