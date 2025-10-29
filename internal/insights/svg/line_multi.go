package insightssvg

import (
	"errors"
	"html/template"
)

var errNotImplemented = errors.New("insights svg: not implemented")

// LineMulti adalah placeholder renderer multi-seri untuk tahap scaffolding.
func LineMulti(width, height int, seriesA, seriesB []float64, labels []string) (template.HTML, error) {
	return "", errNotImplemented
}
