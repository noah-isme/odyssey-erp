package ic

import (
	"context"
	"log/slog"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type stubRepo struct {
	periodID   int64
	exposures  []PairExposure
	headers    map[string]int64
	upserts    map[string]UpsertParams
	createdSeq int64
}

func (s *stubRepo) ResolvePeriodID(_ context.Context, code string) (int64, error) {
	if s.periodID == 0 {
		s.periodID = 77
	}
	return s.periodID, nil
}

func (s *stubRepo) ListPairExposures(_ context.Context, groupID, periodID int64, periodCode string) ([]PairExposure, error) {
	rows := make([]PairExposure, len(s.exposures))
	copy(rows, s.exposures)
	for i := range rows {
		rows[i].GroupID = groupID
		rows[i].PeriodID = periodID
		rows[i].PeriodCode = periodCode
	}
	return rows, nil
}

func (s *stubRepo) UpsertElimination(_ context.Context, params UpsertParams) (UpsertResult, error) {
	if s.headers == nil {
		s.headers = make(map[string]int64)
	}
	if s.upserts == nil {
		s.upserts = make(map[string]UpsertParams)
	}
	headerID, ok := s.headers[params.SourceLink]
	result := UpsertResult{}
	if !ok {
		s.createdSeq++
		headerID = 900 + s.createdSeq
		s.headers[params.SourceLink] = headerID
		result.Created = true
	}
	s.upserts[params.SourceLink] = params
	result.HeaderID = headerID
	return result, nil
}

type stubAudit struct {
	logs []shared.AuditLog
}

func (s *stubAudit) Record(_ context.Context, log shared.AuditLog) error {
	s.logs = append(s.logs, log)
	return nil
}

func TestEngineRunEliminatesPairs(t *testing.T) {
	repo := &stubRepo{exposures: []PairExposure{{
		CompanyAID:       10,
		CompanyAName:     "Alpha",
		CompanyBID:       20,
		CompanyBName:     "Beta",
		ARGroupAccountID: 101,
		APGroupAccountID: 202,
		ARAmount:         1500,
		APAmount:         900,
	}}}
	audit := &stubAudit{}
	engine := NewEngine(repo, audit, slog.Default(), EngineConfig{ActorID: 1})

	res, err := engine.Run(context.Background(), 55, "2024-01")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Eliminated != 1 {
		t.Fatalf("expected one elimination, got %d", res.Eliminated)
	}
	if res.HeadersCreated != 1 {
		t.Fatalf("expected header created, got %d", res.HeadersCreated)
	}
	if res.HeadersUpdated != 0 {
		t.Fatalf("expected no updates on first run")
	}
	if len(repo.upserts) != 1 {
		t.Fatalf("expected one upsert, got %d", len(repo.upserts))
	}
	params := repo.upserts["IC_ARAP|2024-01|10|20"]
	if len(params.Lines) != 2 {
		t.Fatalf("expected two lines, got %d", len(params.Lines))
	}
	debit := params.Lines[0]
	credit := params.Lines[1]
	if debit.GroupAccountID != 202 || debit.Debit != 900 {
		t.Fatalf("unexpected debit line: %+v", debit)
	}
	if credit.GroupAccountID != 101 || credit.Credit != 900 {
		t.Fatalf("unexpected credit line: %+v", credit)
	}
	if len(audit.logs) != 1 {
		t.Fatalf("expected audit log recorded, got %d", len(audit.logs))
	}
	if audit.logs[0].Action != AuditAction {
		t.Fatalf("unexpected audit action %s", audit.logs[0].Action)
	}
	if audit.logs[0].Meta["actor"] != "system/job" {
		t.Fatalf("expected actor system/job, got %v", audit.logs[0].Meta["actor"])
	}

	res2, err := engine.Run(context.Background(), 55, "2024-01")
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if res2.HeadersCreated != 0 {
		t.Fatalf("expected no new headers on rerun")
	}
	if res2.HeadersUpdated != 1 {
		t.Fatalf("expected header update on rerun, got %d", res2.HeadersUpdated)
	}
	if len(audit.logs) != 2 {
		t.Fatalf("expected second audit log, got %d", len(audit.logs))
	}
}
