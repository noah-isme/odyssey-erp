package audit

import "context"

// ExportService mendefinisikan kontrak untuk ekspor CSV audit timeline.
type ExportService interface {
	ExportTimeline(ctx context.Context, filters TimelineFilters) ([]byte, error)
}
