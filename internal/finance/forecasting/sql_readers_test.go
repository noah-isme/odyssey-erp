package forecasting

import (
	"context"
	"testing"
	"time"
)

func TestNewDatabaseReadersIncludesScenarioScopedAdjustments(t *testing.T) {
	readers := NewDatabaseReaders(nil)
	if len(readers) != 8 {
		t.Fatalf("database readers = %d, want 8", len(readers))
	}

	var found bool
	for _, reader := range readers {
		sqlReader, ok := reader.(*SQLSourceReader)
		if !ok {
			continue
		}
		if sqlReader.kind == SourceTypeManualAdjustment {
			found = true
			if sqlReader.name != "forecast-adjustments" {
				t.Fatalf("adjustment reader name = %q, want forecast-adjustments", sqlReader.name)
			}
			if _, ok := reader.(ScenarioSourceReader); !ok {
				t.Fatal("adjustment reader does not implement scenario-scoped source contract")
			}
		}
	}
	if !found {
		t.Fatal("database readers omitted manual/recurring adjustment reader")
	}
}

func TestSQLSourceReaderRejectsUnscopedAdjustmentReads(t *testing.T) {
	reader := &SQLSourceReader{kind: SourceTypeManualAdjustment, name: "forecast-adjustments"}
	date := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	_, err := reader.ReadExpectedFlows(context.Background(), 7, date, date.AddDate(0, 0, 1))
	if err == nil || err.Error() != "forecast source reader database is not configured" {
		t.Fatalf("ReadExpectedFlows() error = %v, want missing database error", err)
	}
}
