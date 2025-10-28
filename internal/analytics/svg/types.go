package svg

// LineOpts customises the line chart renderer.
type LineOpts struct {
	Title       string
	Description string
	StrokeColor string
	FillColor   string
	AxisColor   string
	GridColor   string
	Padding     float64
	ShowDots    bool
	TickCount   int
}

// BarOpts customises the bar chart renderer.
type BarOpts struct {
	Title        string
	Description  string
	SeriesALabel string
	SeriesBLabel string
	ColorA       string
	ColorB       string
	AxisColor    string
	GridColor    string
	Padding      float64
	TickCount    int
}

// Defaults for the analytics charts.
const (
	DefaultWidth   = 720
	DefaultHeight  = 240
	DefaultPadding = 24.0
	DefaultTicks   = 6
)
