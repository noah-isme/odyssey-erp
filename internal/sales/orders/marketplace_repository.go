package orders

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// MappingRepository adapts connector object mappings for marketplace
// processing without exposing SQLC parameters to the event handler.
type MappingRepository struct {
	queries *sqlc.Queries
}

func NewMappingRepository(pool *pgxpool.Pool) *MappingRepository {
	return &MappingRepository{queries: sqlc.New(pool)}
}

func (r *MappingRepository) GetObjectMappingByRemote(ctx context.Context, query ObjectMappingQuery) (connectors.ObjectMapping, error) {
	row, err := r.queries.GetObjectMappingByRemote(ctx, sqlc.GetObjectMappingByRemoteParams{
		CompanyID:        query.CompanyID,
		ConnectionID:     query.ConnectionID,
		RemoteEntityType: query.RemoteEntityType,
		RemoteEntityID:   query.RemoteEntityID,
	})
	if err != nil {
		return connectors.ObjectMapping{}, err
	}
	return connectors.ObjectMapping{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		ConnectionID:     row.ConnectionID,
		LocalEntityType:  row.LocalEntityType,
		LocalEntityID:    row.LocalEntityID,
		RemoteEntityType: row.RemoteEntityType,
		RemoteEntityID:   row.RemoteEntityID,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}, nil
}
