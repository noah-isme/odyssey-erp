package reporting

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ReportQuery defines what the user wants to query from a dataset.
type ReportQuery struct {
	DatasetID  uuid.UUID
	Dimensions []string
	Measures   []string
	Filters    []Filter
	RowLimit   int
}

type Filter struct {
	Field    string
	Operator string // "=", ">", "<", "IN", etc.
	Value    interface{}
}

// CompiledPlan represents the result of the safe compiler.
type CompiledPlan struct {
	SQL       string
	Args      []interface{}
	CostLimit int
}

// SafeCompiler validates a query against a dataset and generates safe SQL.
type SafeCompiler struct {
	// dependencies like catalog service, permission evaluator
}

func NewSafeCompiler() *SafeCompiler {
	return &SafeCompiler{}
}

// Compile validates the requested dimensions/measures against the allowed dataset fields,
// enforces tenant scope, and generates a parameterized SQL query.
func (c *SafeCompiler) Compile(ctx context.Context, companyID, actorID int64, dataset *DatasetDefinition, query ReportQuery) (*CompiledPlan, error) {
	if len(query.Dimensions) == 0 && len(query.Measures) == 0 {
		return nil, fmt.Errorf("must specify at least one dimension or measure")
	}

	var selectFields []string
	var groupByFields []string

	// Validate dimensions
	for _, dim := range query.Dimensions {
		if !dataset.IsDimensionAllowed(dim) {
			return nil, fmt.Errorf("dimension not allowed: %s", dim)
		}
		selectFields = append(selectFields, dim)
		groupByFields = append(groupByFields, dim)
	}

	// Validate measures
	for _, measure := range query.Measures {
		if !dataset.IsMeasureAllowed(measure) {
			return nil, fmt.Errorf("measure not allowed: %s", measure)
		}
		// A real implementation would apply the correct aggregation function defined in the catalog
		selectFields = append(selectFields, fmt.Sprintf("SUM(%s) as %s", measure, measure))
	}

	args := []interface{}{companyID}
	whereClauses := []string{"company_id = $1"}

	// Add user-requested filters
	for _, f := range query.Filters {
		if !dataset.IsFieldAllowed(f.Field) {
			return nil, fmt.Errorf("filter field not allowed: %s", f.Field)
		}
		args = append(args, f.Value)
		whereClauses = append(whereClauses, fmt.Sprintf("%s %s $%d", f.Field, f.Operator, len(args)))
	}

	sqlBuilder := strings.Builder{}
	sqlBuilder.WriteString("SELECT ")
	sqlBuilder.WriteString(strings.Join(selectFields, ", "))
	sqlBuilder.WriteString(" FROM ")
	sqlBuilder.WriteString(dataset.SourceTable)
	sqlBuilder.WriteString(" WHERE ")
	sqlBuilder.WriteString(strings.Join(whereClauses, " AND "))

	if len(groupByFields) > 0 {
		sqlBuilder.WriteString(" GROUP BY ")
		sqlBuilder.WriteString(strings.Join(groupByFields, ", "))
	}

	if query.RowLimit > 0 {
		if query.RowLimit > 10000 {
			query.RowLimit = 10000 // quota limit
		}
		sqlBuilder.WriteString(fmt.Sprintf(" LIMIT %d", query.RowLimit))
	} else {
		sqlBuilder.WriteString(" LIMIT 1000") // default quota
	}

	return &CompiledPlan{
		SQL:       sqlBuilder.String(),
		Args:      args,
		CostLimit: 1000, // example quota
	}, nil
}
