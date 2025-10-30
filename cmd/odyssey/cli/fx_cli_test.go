package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	upserts   []consol.FxRateInput
}

func (s *stubFXRepo) GroupReportingCurrency(ctx context.Context, groupID int64) (string, error) {
	if s.reporting == "" {
		return "", consol.ErrGroupNotFound
	}
	return s.reporting, nil
}

func (s *stubFXRepo) MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error) {
	return s.members, nil
}

func (s *stubFXRepo) FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error) {
	key := strings.ToUpper(pair)
	periodKey := key + "|" + asOf.Format("2006-01")
	if quote, ok := s.quotes[periodKey]; ok {
		return quote, nil
	}
	if quote, ok := s.quotes[key]; ok {
		return quote, nil
	}
	return fx.Quote{}, consol.ErrFxRateNotFound
}

func (s *stubFXRepo) UpsertFxRates(ctx context.Context, rows []consol.FxRateInput) error {
	s.upserts = append(s.upserts, rows...)
	return nil
}

func TestValidateCommandJSONSuccess(t *testing.T) {
	repo := &stubFXRepo{
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
	repo := &stubFXRepo{
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
	repo := &stubFXRepo{reporting: "IDR", members: map[int64]string{}}
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

func TestBackfillCommandDry(t *testing.T) {
	repo := &stubFXRepo{reporting: "IDR", members: map[int64]string{}, quotes: map[string]fx.Quote{}}
	cli, err := NewFXOpsCLI(repo)
	require.NoError(t, err)

	csvData := "period,pair,average,closing\n2024-01,USDIDR,15500,15450\n"
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := cli.BackfillCommand(context.Background(), FXBackfillOptions{
		Pair:         "usdidr",
		From:         "2024-01",
		To:           "2024-02",
		Mode:         FXBackfillModeDry,
		JSONOutput:   true,
		Stdout:       stdout,
		Stderr:       stderr,
		SourceReader: strings.NewReader(csvData),
	})
	require.Equal(t, 10, exitCode)
	require.Empty(t, stderr.String())

	var summary FXBackfillSummary
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	require.Equal(t, FXBackfillModeDry, summary.Mode)
	require.Equal(t, "USDIDR", summary.Pair)
	require.Len(t, summary.Missing, 2)
	require.Len(t, summary.Candidates, 1)
	require.Empty(t, summary.Applied)
	require.Empty(t, repo.upserts)
}

func TestBackfillCommandApply(t *testing.T) {
	repo := &stubFXRepo{reporting: "IDR", members: map[int64]string{}, quotes: map[string]fx.Quote{}}
	cli, err := NewFXOpsCLI(repo)
	require.NoError(t, err)

	csvData := strings.NewReader("period,pair,average,closing\n2024-02,USDIDR,15600,15580\n2024-03,USDIDR,15650,15610\n")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := cli.BackfillCommand(context.Background(), FXBackfillOptions{
		Pair:         "USDIDR",
		From:         "2024-02",
		To:           "2024-03",
		Mode:         FXBackfillModeApply,
		JSONOutput:   true,
		Stdout:       stdout,
		Stderr:       stderr,
		SourceReader: csvData,
		Confirm: func(io.Reader, io.Writer) (bool, error) {
			return true, nil
		},
	})
	require.Zero(t, exitCode)
	require.Empty(t, stderr.String())

	var summary FXBackfillSummary
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	require.Equal(t, FXBackfillModeApply, summary.Mode)
	require.Len(t, summary.Missing, 2)
	require.Len(t, summary.Candidates, 2)
	require.Len(t, summary.Applied, 2)
	require.Len(t, repo.upserts, 2)
	require.Equal(t, "2024-02", summary.Applied[0].Period)
	require.Equal(t, "2024-03", summary.Applied[1].Period)
}
