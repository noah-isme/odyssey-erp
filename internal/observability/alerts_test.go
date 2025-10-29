package observability

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type alertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type alertGroup struct {
	Name  string      `yaml:"name"`
	Rules []alertRule `yaml:"rules"`
}

type alertSpec struct {
	Groups []alertGroup `yaml:"groups"`
}

func TestFinanceAlertRules(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "prometheus", "alerts", "finance.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read alert file: %v", err)
	}

	var spec alertSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to unmarshal alert file: %v", err)
	}

	if len(spec.Groups) == 0 {
		t.Fatal("expected at least one alert group")
	}

	var financeGroup *alertGroup
	for i := range spec.Groups {
		if spec.Groups[i].Name == "finance" {
			financeGroup = &spec.Groups[i]
			break
		}
	}
	if financeGroup == nil {
		t.Fatal("finance alert group missing")
	}

	expected := map[string]struct {
		severity string
		runbook  string
	}{
		"HighErrorRate": {severity: "critical", runbook: "docs/runbook-ops-finance.md#high-error-rate"},
		"HighLatency":   {severity: "warning", runbook: "docs/runbook-ops-finance.md#high-latency"},
		"AnomalySpike":  {severity: "warning", runbook: "docs/runbook-ops-finance.md#anomaly-spike"},
	}

	if len(financeGroup.Rules) != len(expected) {
		t.Fatalf("expected %d rules, got %d", len(expected), len(financeGroup.Rules))
	}

	for _, rule := range financeGroup.Rules {
		want, ok := expected[rule.Alert]
		if !ok {
			t.Fatalf("unexpected rule %q", rule.Alert)
		}
		if rule.Labels["severity"] != want.severity {
			t.Fatalf("rule %s severity mismatch: %s", rule.Alert, rule.Labels["severity"])
		}
		if rule.Annotations["runbook"] != want.runbook {
			t.Fatalf("rule %s runbook mismatch: %s", rule.Alert, rule.Annotations["runbook"])
		}
		if rule.Annotations["summary"] == "" || rule.Annotations["description"] == "" {
			t.Fatalf("rule %s must include summary and description annotations", rule.Alert)
		}
		if rule.Expr == "" {
			t.Fatalf("rule %s must define an expression", rule.Alert)
		}
		if rule.For == "" {
			t.Fatalf("rule %s must define a hold duration", rule.Alert)
		}
	}
}
