package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEngine(t *testing.T) {
	engine, err := NewEngine()
	assert.NoError(t, err, "Templates should parse without error")
	assert.NotNil(t, engine)
}
