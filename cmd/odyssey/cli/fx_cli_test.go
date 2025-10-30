package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
)

type stubFXRepo struct {
	reporting string
	members   map[int64]string
	quotes    map[string]fx.Quote
}

func (s stubFXRepo) GroupReportingCurrency(ctx context.Context, groupID int64) (string, error) {
	if s.reporting == "" {
		return "", consol.ErrGroupNotFound
	}
	return s.reporting, nil
}

func (s stubFXRepo) MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error) {
	return s.members, nil
}

func (s stubFXRepo) FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error) {
	quote, ok := s.quotes[strings.ToUpper(pair)]
	if !ok {
		return fx.Quote{}, consol.ErrFxRateNotFound
	}
	return quote, nil
}

func TestValidateCommandJSONSuccess(t *testing.T) {
	repo := stubFXRepo{
		reporting: "IDR",
		members:   map[int64]string{1: "USD"},
		quotes: map[string]fx.Quote{
			"USDIDR": {Average: 15500, Closing: 15450},
		},
	}
	cli, err := NewFXOpsCLI(repo)
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := cli.ValidateCommand(context.Background(), FXValidateOptions{
		GroupID:    1,
		Period:     "2024-01",
		JSONOutput: true,
		Stdout:     stdout,
		Stderr:     stderr,
	})
	require.Zero(t, exitCode)
	require.Empty(t, stderr.String())

	var summary FXValidateSummary
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	require.True(t, summary.OK)
	require.Empty(t, summary.Gaps)
	require.Len(t, summary.AvailableQuotes, 2)
}

func TestValidateCommandJSONGaps(t *testing.T) {
	repo := stubFXRepo{
		reporting: "IDR",
		members:   map[int64]string{1: "USD"},
		quotes:    map[string]fx.Quote{},
	}
	cli, err := NewFXOpsCLI(repo)
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := cli.ValidateCommand(context.Background(), FXValidateOptions{
		GroupID:    1,
		Period:     "2024-01",
		JSONOutput: true,
		Stdout:     stdout,
		Stderr:     stderr,
	})
	require.Equal(t, 10, exitCode)
	require.Empty(t, stderr.String())

	var summary FXValidateSummary
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	require.False(t, summary.OK)
	require.Len(t, summary.Gaps, 2)
}

func TestValidateCommandInvalidPeriod(t *testing.T) {
	repo := stubFXRepo{reporting: "IDR", members: map[int64]string{}}
	cli, err := NewFXOpsCLI(repo)
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := cli.ValidateCommand(context.Background(), FXValidateOptions{
		GroupID: 1,
		Period:  "202401",
		Stdout:  stdout,
		Stderr:  stderr,
	})
	require.Equal(t, 1, exitCode)
	require.Contains(t, stderr.String(), "invalid period")
}
