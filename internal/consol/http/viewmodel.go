package http

import "github.com/odyssey-erp/odyssey-erp/internal/consol"

// ConsolTBVM drives the consolidated TB page rendering.
type ConsolTBVM struct {
	Filters struct {
		GroupID  int64
		Period   string
		Entities []int64
	}
	Totals struct {
		Local    float64
		Group    float64
		Balanced bool
	}
	Lines []struct {
		GroupAccount string
		Name         string
		Local        float64
		Group        float64
	}
	Contribution []struct {
		Entity   string
		GroupAmt float64
		Pct      float64
	}
	Members      []consol.Member
	GroupName    string
	ReportingCCY string
	PeriodLabel  string
	Errors       map[string]string
}

// FromDomain maps service result into the view model.
func FromDomain(tb consol.TrialBalance) ConsolTBVM {
	var vm ConsolTBVM
	vm.Errors = make(map[string]string)
	vm.Filters.GroupID = tb.Filters.GroupID
	vm.Filters.Period = tb.Filters.Period
	vm.Filters.Entities = append(vm.Filters.Entities, tb.Filters.Entities...)
	vm.GroupName = tb.GroupName
	vm.ReportingCCY = tb.ReportingCCY
	vm.PeriodLabel = tb.PeriodDisplay
	vm.Totals.Local = tb.Totals.Local
	vm.Totals.Group = tb.Totals.Group
	vm.Totals.Balanced = tb.Totals.Balanced
	vm.Members = append(vm.Members, tb.Members...)
	vm.Lines = make([]struct {
		GroupAccount string
		Name         string
		Local        float64
		Group        float64
	}, len(tb.Lines))
	for i, line := range tb.Lines {
		vm.Lines[i] = struct {
			GroupAccount string
			Name         string
			Local        float64
			Group        float64
		}{
			GroupAccount: line.GroupAccountCode,
			Name:         line.GroupAccountName,
			Local:        line.LocalAmount,
			Group:        line.GroupAmount,
		}
	}
	vm.Contribution = make([]struct {
		Entity   string
		GroupAmt float64
		Pct      float64
	}, len(tb.Contributions))
	for i, contrib := range tb.Contributions {
		vm.Contribution[i] = struct {
			Entity   string
			GroupAmt float64
			Pct      float64
		}{
			Entity:   contrib.Entity,
			GroupAmt: contrib.Amount,
			Pct:      contrib.Percent,
		}
	}
	return vm
}
