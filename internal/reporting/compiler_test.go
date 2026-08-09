package reporting

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSafeCompilerBuildsTenantScopedParameterizedPlan(t *testing.T) {
	companyID := uuid.New()
	dataset := &DatasetDefinition{
		SourceTable: "analytics_finance_cube",
		Fields: map[string]DatasetField{
			"period":   {Name: "period", IsDimension: true},
			"revenue":  {Name: "revenue", IsMeasure: true},
			"currency": {Name: "currency"},
		},
	}
	plan, err := NewSafeCompiler().Compile(context.Background(), companyID, uuid.New(), dataset, ReportQuery{
		Dimensions: []string{"period"},
		Measures:   []string{"revenue"},
		Filters:    []Filter{{Field: "currency", Operator: "=", Value: "IDR"}},
		RowLimit:   20000,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"SELECT period, SUM(revenue) as revenue",
		"FROM analytics_finance_cube",
		"company_id = $1 AND currency = $2",
		"GROUP BY period",
		"LIMIT 10000",
	} {
		if !strings.Contains(plan.SQL, want) {
			t.Fatalf("SQL missing %q: %s", want, plan.SQL)
		}
	}
	if len(plan.Args) != 2 || plan.Args[0] != companyID || plan.Args[1] != "IDR" || plan.CostLimit != 1000 {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestSafeCompilerRejectsUnapprovedQueries(t *testing.T) {
	dataset := &DatasetDefinition{SourceTable: "safe_table", Fields: map[string]DatasetField{
		"period":  {Name: "period", IsDimension: true},
		"revenue": {Name: "revenue", IsMeasure: true},
	}}
	compiler := NewSafeCompiler()
	for _, query := range []ReportQuery{
		{},
		{Dimensions: []string{"unknown"}},
		{Measures: []string{"unknown"}},
		{Dimensions: []string{"period"}, Filters: []Filter{{Field: "unknown", Operator: "=", Value: "x"}}},
	} {
		if _, err := compiler.Compile(context.Background(), uuid.New(), uuid.New(), dataset, query); err == nil {
			t.Fatalf("query %+v unexpectedly compiled", query)
		}
	}
}
