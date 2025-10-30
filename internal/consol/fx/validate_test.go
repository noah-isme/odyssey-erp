package fx

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProvider struct {
	quotes map[string]Quote
	err    error
}

func (f fakeProvider) QuoteForPeriod(ctx context.Context, asOf time.Time, pair string) (Quote, bool, error) {
	if f.err != nil {
		return Quote{}, false, f.err
	}
	quote, ok := f.quotes[pair]
	return quote, ok, nil
}

func TestValidate_AllRatesAvailable(t *testing.T) {
	provider := fakeProvider{quotes: map[string]Quote{
		"IDRUSD": {Average: 1.2, Closing: 1.25},
	}}
	period := time.Date(2025, 8, 7, 12, 0, 0, 0, time.UTC)
	reqs := []Requirement{
		{Pair: "idrusd", Methods: []Method{MethodAverage, MethodClosing}},
	}
	res, err := Validate(context.Background(), provider, period, reqs)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(res.Gaps) != 0 {
		t.Fatalf("expected no gaps, got %+v", res.Gaps)
	}
	if res.Checked != 1 {
		t.Fatalf("expected 1 pair checked, got %d", res.Checked)
	}
	if res.Available["IDRUSD"].Average != 1.2 {
		t.Fatalf("unexpected quote stored: %+v", res.Available["IDRUSD"])
	}
}

func TestValidate_MissingAverageAndClosing(t *testing.T) {
	provider := fakeProvider{}
	period := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	reqs := []Requirement{{Pair: "IDRUSD", Methods: []Method{MethodAverage, MethodClosing}}}
	res, err := Validate(context.Background(), provider, period, reqs)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(res.Gaps) != 1 {
		t.Fatalf("expected one gap, got %d", len(res.Gaps))
	}
	gap := res.Gaps[0]
	if gap.Pair != "IDRUSD" {
		t.Fatalf("unexpected pair %s", gap.Pair)
	}
	if len(gap.Methods) != 2 {
		t.Fatalf("expected two missing methods, got %d", len(gap.Methods))
	}
}

func TestValidate_PartialMissing(t *testing.T) {
	provider := fakeProvider{quotes: map[string]Quote{
		"IDRUSD": {Average: 0, Closing: 1.3},
	}}
	period := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	reqs := []Requirement{{Pair: "IDRUSD", Methods: []Method{MethodAverage, MethodClosing}}}
	res, err := Validate(context.Background(), provider, period, reqs)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(res.Gaps) != 1 {
		t.Fatalf("expected one gap, got %d", len(res.Gaps))
	}
	gap := res.Gaps[0]
	if len(gap.Methods) != 1 || gap.Methods[0] != MethodAverage {
		t.Fatalf("unexpected missing methods: %+v", gap.Methods)
	}
}

func TestValidate_InvalidRequirement(t *testing.T) {
	provider := fakeProvider{}
	period := time.Now()
	_, err := Validate(context.Background(), provider, period, []Requirement{{Pair: "", Methods: []Method{MethodAverage}}})
	if err == nil {
		t.Fatalf("expected error for empty pair")
	}
}

func TestValidate_UnsupportedMethod(t *testing.T) {
	provider := fakeProvider{}
	period := time.Now()
	_, err := Validate(context.Background(), provider, period, []Requirement{{Pair: "IDRUSD", Methods: []Method{"SPOT"}}})
	if err == nil {
		t.Fatalf("expected error for unsupported method")
	}
}

func TestValidate_PropagatesProviderError(t *testing.T) {
	wantErr := errors.New("boom")
	provider := fakeProvider{err: wantErr}
	period := time.Now()
	_, err := Validate(context.Background(), provider, period, []Requirement{{Pair: "IDRUSD", Methods: []Method{MethodAverage}}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestValidate_RequiresProvider(t *testing.T) {
	period := time.Now()
	_, err := Validate(context.Background(), nil, period, []Requirement{{Pair: "IDRUSD", Methods: []Method{MethodAverage}}})
	if err == nil {
		t.Fatalf("expected error when provider nil")
	}
}

func TestValidate_RequiresPeriod(t *testing.T) {
	provider := fakeProvider{}
	_, err := Validate(context.Background(), provider, time.Time{}, []Requirement{{Pair: "IDRUSD", Methods: []Method{MethodAverage}}})
	if err == nil {
		t.Fatalf("expected error when period empty")
	}
}
