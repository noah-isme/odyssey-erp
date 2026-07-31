package tax

import "testing"

// This test is intentionally a release gate. It must be enabled only after
// the current official DJP/Coretax validator endpoint and approved
// representative-month fixtures are supplied by the release process.
func TestExternalCoretaxValidatorAndGLReconciliation_BLOCKED_RELEASE(t *testing.T) {
	t.Skip("blocked release test: official DJP/Coretax validator and approved representative month are not configured")
}
