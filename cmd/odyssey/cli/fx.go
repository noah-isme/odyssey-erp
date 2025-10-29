package cli

import "errors"

// FXOpsCLI offers operational helpers to manage FX rates used by consolidation.
type FXOpsCLI struct{}

// NewFXOpsCLI constructs a new helper instance.
func NewFXOpsCLI() *FXOpsCLI {
	return &FXOpsCLI{}
}

// ErrFXOpsNotImplemented indicates that the helper is pending implementation.
var ErrFXOpsNotImplemented = errors.New("consol: fx ops cli not implemented")

// ImportRates ingests FX rates into the system.
func (c *FXOpsCLI) ImportRates(path string) error {
	return ErrFXOpsNotImplemented
}

// ValidateGaps inspects FX rate gaps for the configured policy.
func (c *FXOpsCLI) ValidateGaps() error {
	return ErrFXOpsNotImplemented
}
