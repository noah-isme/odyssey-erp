package reporting

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// DatasetDefinition represents a certified, safe-to-query dataset.
type DatasetDefinition struct {
	ID          uuid.UUID
	Key         string
	Version     int
	SourceTable string
	Fields      map[string]DatasetField
}

type DatasetField struct {
	Name           string
	Type           string
	Classification string
	IsDimension    bool
	IsMeasure      bool
}

// IsDimensionAllowed checks if a dimension is registered.
func (d *DatasetDefinition) IsDimensionAllowed(name string) bool {
	f, ok := d.Fields[name]
	return ok && f.IsDimension
}

// IsMeasureAllowed checks if a measure is registered.
func (d *DatasetDefinition) IsMeasureAllowed(name string) bool {
	f, ok := d.Fields[name]
	return ok && f.IsMeasure
}

// IsFieldAllowed checks if a field exists for filtering.
func (d *DatasetDefinition) IsFieldAllowed(name string) bool {
	_, ok := d.Fields[name]
	return ok
}

// Catalog manages the registration and lookup of datasets.
type Catalog struct {
	q *sqlc.Queries
}

func NewCatalog(q *sqlc.Queries) *Catalog {
	return &Catalog{q: q}
}

// GetDataset loads a dataset definition from the database.
func (c *Catalog) GetDataset(ctx context.Context, companyID uuid.UUID, key string, version int) (*DatasetDefinition, error) {
	row, err := c.q.GetReportingDataset(ctx, sqlc.GetReportingDatasetParams{
		CompanyID: pgtype.UUID{Bytes: companyID, Valid: true},
		Key:       key,
		Version:   int32(version),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset: %w", err)
	}

	fieldsRow, err := c.q.ListReportingDatasetFields(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list dataset fields: %w", err)
	}

	fields := make(map[string]DatasetField)
	for _, f := range rowToFields(fieldsRow) {
		fields[f.Name] = f
	}

	// Assuming the source table is statically mapped by the dataset key or stored in a column
	// We'll map 'finance_analytics' to the real view name
	sourceTable := mapSourceTable(key)

	return &DatasetDefinition{
		ID:          uuid.UUID(row.ID.Bytes),
		Key:         row.Key,
		Version:     int(row.Version),
		SourceTable: sourceTable,
		Fields:      fields,
	}, nil
}

func rowToFields(rows []sqlc.ReportingDatasetField) []DatasetField {
	var res []DatasetField
	for _, r := range rows {
		res = append(res, DatasetField{
			Name:           r.FieldName,
			Type:           r.FieldType,
			Classification: r.Classification,
			IsDimension:    r.IsDimension,
			IsMeasure:      r.IsMeasure,
		})
	}
	return res
}

func mapSourceTable(key string) string {
	switch key {
	case "finance_analytics":
		return "analytics_finance_cube"
	default:
		return "unknown_table"
	}
}
