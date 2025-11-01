//go:build !prod

package http

import "errors"

var (
	// ErrPDFTimeout mirrors the production sentinel to keep call sites consistent.
	ErrPDFTimeout = errors.New("consol pdf exporter: timeout")
	// ErrPDFInvalidResponse mirrors the production sentinel to keep call sites consistent.
	ErrPDFInvalidResponse = errors.New("consol pdf exporter: invalid response")
	// ErrPDFTooSmall mirrors the production sentinel to keep call sites consistent.
	ErrPDFTooSmall = errors.New("consol pdf exporter: pdf below minimum size")
)

// NewPDFRenderClient returns a disabled PDF client for non-production builds.
func NewPDFRenderClient(string) (PDFRenderClient, error) {
	return nil, nil
}
