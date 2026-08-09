package mappings

import (
	"context"
	"errors"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestGetRejectsIncompleteMappingKeyBeforeDatabase(t *testing.T) {
	repo := &repository{}
	for _, tc := range []struct {
		module string
		key    string
	}{
		{module: "", key: "invoice"},
		{module: "sales", key: ""},
	} {
		if _, err := repo.Get(context.Background(), tc.module, tc.key); err == nil {
			t.Fatalf("Get(%q, %q) accepted an incomplete key", tc.module, tc.key)
		}
	}
}

func TestGetNormalizesModuleAndMapsRows(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &repository{db: db}
	db.ExpectQuery("SELECT module, key, account_id, created_at, updated_at FROM account_mappings").
		WithArgs("SALES", "invoice").
		WillReturnRows(pgxmock.NewRows([]string{"module", "key", "account_id", "created_at", "updated_at"}).
			AddRow("SALES", "invoice", int64(12), nil, nil))
	mapping, err := repo.Get(context.Background(), "sales", "invoice")
	if err != nil || mapping.Module != "SALES" || mapping.AccountID != 12 {
		t.Fatalf("Get() = %#v, %v", mapping, err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetMapsMissingRowsToDomainError(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &repository{db: db}
	db.ExpectQuery("SELECT module, key, account_id, created_at, updated_at FROM account_mappings").
		WithArgs("SALES", "missing").WillReturnRows(pgxmock.NewRows([]string{"module", "key", "account_id", "created_at", "updated_at"}))
	_, err = repo.Get(context.Background(), "SALES", "missing")
	if !errors.Is(err, shared.ErrMappingNotFound) {
		t.Fatalf("Get() error = %v", err)
	}
}
