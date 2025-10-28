package ui

import (
	"html/template"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/svg"
)

// DashboardFilters represents sanitized query filters used by the dashboard.
type DashboardFilters struct {
	Period    string
	CompanyID int64
	BranchID  *int64
}

// DashboardKPI exposes headline metrics for the dashboard cards.
type DashboardKPI struct {
	Revenue       float64
	Opex          float64
	NetProfit     float64
	CashIn        float64
	CashOut       float64
	COGS          float64
	AROutstanding float64
	APOutstanding float64
}

// PLTrendPoint represents a month entry displayed on the P&L trend chart/table.
type PLTrendPoint struct {
	Month   string
	Revenue float64
	COGS    float64
	Opex    float64
	Net     float64
}

// CashflowPoint reflects the cash movement for a month.
type CashflowPoint struct {
	Month string
	In    float64
	Out   float64
}

// AgingBucket groups receivable or payable outstanding balances.
type AgingBucket struct {
	Bucket string
	Amount float64
}

// DashboardViewModel combines all dashboard data for rendering.
type DashboardViewModel struct {
	Filters       DashboardFilters
	KPI           DashboardKPI
	PLTrend       []PLTrendPoint
	CashflowTrend []CashflowPoint
	AgingAR       []AgingBucket
	AgingAP       []AgingBucket
	PLTrendSVG    template.HTML
	CashflowSVG   template.HTML
}

// LineRenderer abstracts SVG line chart rendering for the dashboard.
type LineRenderer interface {
	Line(width, height int, series []float64, labels []string, opts svg.LineOpts) (template.HTML, error)
}

// BarRenderer abstracts SVG bar chart rendering for the dashboard.
type BarRenderer interface {
	Bars(width, height int, seriesA, seriesB []float64, labels []string, opts svg.BarOpts) (template.HTML, error)
}

// ToPLTrendPoints converts analytics domain data into UI points.
func ToPLTrendPoints(points []analytics.PLTrendPoint) []PLTrendPoint {
	uiPoints := make([]PLTrendPoint, 0, len(points))
	for _, point := range points {
		uiPoints = append(uiPoints, PLTrendPoint{
			Month:   point.Period,
			Revenue: point.Revenue,
			COGS:    point.COGS,
			Opex:    point.Opex,
			Net:     point.Net,
		})
	}
	return uiPoints
}

// ToCashflowPoints converts analytics domain cashflow to UI representation.
func ToCashflowPoints(points []analytics.CashflowTrendPoint) []CashflowPoint {
	uiPoints := make([]CashflowPoint, 0, len(points))
	for _, point := range points {
		uiPoints = append(uiPoints, CashflowPoint{
			Month: point.Period,
			In:    point.In,
			Out:   point.Out,
		})
	}
	return uiPoints
}

// ToAgingBuckets converts analytics domain aging buckets to UI buckets.
func ToAgingBuckets(buckets []analytics.AgingBucket) []AgingBucket {
	uiBuckets := make([]AgingBucket, 0, len(buckets))
	for _, bucket := range buckets {
		uiBuckets = append(uiBuckets, AgingBucket{Bucket: bucket.Bucket, Amount: bucket.Amount})
	}
	return uiBuckets
}
