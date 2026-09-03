package reporting

import (
	"testing"
)

func TestDatasetDefinitionAllowsOnlyDeclaredFieldRoles(t *testing.T) {
	dataset := DatasetDefinition{Fields: map[string]DatasetField{
		"period":  {Name: "period", IsDimension: true},
		"revenue": {Name: "revenue", IsMeasure: true},
		"company": {Name: "company"},
	}}

	if !dataset.IsDimensionAllowed("period") || dataset.IsDimensionAllowed("revenue") {
		t.Fatal("dimension permissions are incorrect")
	}
	if !dataset.IsMeasureAllowed("revenue") || dataset.IsMeasureAllowed("period") {
		t.Fatal("measure permissions are incorrect")
	}
	if !dataset.IsFieldAllowed("company") || dataset.IsFieldAllowed("unknown") {
		t.Fatal("field permissions are incorrect")
	}
}

func TestCatalogFieldAndSourceMappingHelpers(t *testing.T) {
	fields := []DatasetFieldRecord{{Name: "revenue", Type: "numeric", Classification: "internal", IsMeasure: true}}
	if len(fields) != 1 || fields[0].Name != "revenue" || !fields[0].IsMeasure {
		t.Fatalf("fields=%+v", fields)
	}
	if got := mapSourceTable("finance_analytics"); got != "analytics_finance_cube" {
		t.Fatalf("finance table=%q", got)
	}
	if got := mapSourceTable("unknown"); got != "unknown_table" {
		t.Fatalf("unknown table=%q", got)
	}
}
