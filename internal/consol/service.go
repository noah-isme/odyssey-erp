package consol

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// DBRepository defines the required persistence behaviour for the service.
type DBRepository interface {
	FindPeriodID(ctx context.Context, code string) (int64, error)
	GetGroup(ctx context.Context, groupID int64) (string, string, error)
	Members(ctx context.Context, groupID int64) ([]MemberRow, error)
	RebuildConsolidation(ctx context.Context, groupID, periodID int64) error
	Balances(ctx context.Context, groupID, periodID int64) ([]BalanceRow, error)
}

// Service orchestrates consolidation operations.
type Service struct {
	repo DBRepository
	now  func() time.Time
}

// NewService constructs a consolidation service instance.
func NewService(repo DBRepository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// WithClock overrides the clock for deterministic tests.
func (s *Service) WithClock(clock func() time.Time) {
	if clock != nil {
		s.now = clock
	}
}

// RebuildConsolidation refreshes the materialised view for the given period code.
func (s *Service) RebuildConsolidation(ctx context.Context, groupID int64, periodCode string) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("consol service not initialised")
	}
	if groupID <= 0 {
		return fmt.Errorf("invalid group id")
	}
	if periodCode == "" {
		return fmt.Errorf("period code is required")
	}
	periodID, err := s.repo.FindPeriodID(ctx, periodCode)
	if err != nil {
		return err
	}
	return s.repo.RebuildConsolidation(ctx, groupID, periodID)
}

// GetConsolidatedTB composes the consolidated trial balance for filters.
func (s *Service) GetConsolidatedTB(ctx context.Context, filter Filters) (TrialBalance, error) {
	if s == nil || s.repo == nil {
		return TrialBalance{}, fmt.Errorf("consol service not initialised")
	}
	if filter.GroupID <= 0 {
		return TrialBalance{}, fmt.Errorf("group id wajib diisi")
	}
	if filter.Period == "" {
		return TrialBalance{}, fmt.Errorf("periode wajib diisi")
	}
	if _, err := time.Parse("2006-01", filter.Period); err != nil {
		return TrialBalance{}, fmt.Errorf("format periode tidak valid")
	}
	periodID, err := s.repo.FindPeriodID(ctx, filter.Period)
	if err != nil {
		return TrialBalance{}, err
	}
	groupName, ccy, err := s.repo.GetGroup(ctx, filter.GroupID)
	if err != nil {
		return TrialBalance{}, err
	}
	members, err := s.repo.Members(ctx, filter.GroupID)
	if err != nil {
		return TrialBalance{}, err
	}
	memberSet := make(map[int64]MemberRow, len(members))
	for _, m := range members {
		memberSet[m.CompanyID] = m
	}
	if len(filter.Entities) > 0 {
		for _, id := range filter.Entities {
			if _, ok := memberSet[id]; !ok {
				return TrialBalance{}, fmt.Errorf("entitas %d tidak terdaftar", id)
			}
		}
	}
	rows, err := s.repo.Balances(ctx, filter.GroupID, periodID)
	if err != nil {
		return TrialBalance{}, err
	}
	includeAll := len(filter.Entities) == 0
	include := make(map[int64]struct{})
	for _, id := range filter.Entities {
		include[id] = struct{}{}
	}
	var totalLocal float64
	var totalGroup float64
	contributions := make(map[int64]Contribution)
	balances := make([]GroupAccountBalance, 0, len(rows))
	for _, row := range rows {
		membersShare, err := ParseMembers(row.MembersJSON)
		if err != nil {
			return TrialBalance{}, err
		}
		filtered := make([]MemberShare, 0, len(membersShare))
		var lineTotal float64
		for _, member := range membersShare {
			if !includeAll {
				if _, ok := include[member.CompanyID]; !ok {
					continue
				}
			}
			filtered = append(filtered, member)
			lineTotal += member.LocalAmount
			c := contributions[member.CompanyID]
			c.Entity = member.CompanyName
			c.Amount += member.LocalAmount
			contributions[member.CompanyID] = c
		}
		if len(filtered) == 0 {
			continue
		}
		totalLocal += lineTotal
		totalGroup += lineTotal
		balances = append(balances, GroupAccountBalance{
			GroupAccountID:   row.GroupAccountID,
			GroupAccountCode: row.GroupAccountCode,
			GroupAccountName: row.GroupAccountName,
			LocalAmount:      lineTotal,
			GroupAmount:      lineTotal,
			Members:          filtered,
		})
	}
	contribList := make([]Contribution, 0, len(contributions))
	for _, c := range contributions {
		contribList = append(contribList, c)
	}
	sort.SliceStable(contribList, func(i, j int) bool {
		return math.Abs(contribList[i].Amount) > math.Abs(contribList[j].Amount)
	})
	if totalGroup != 0 {
		for i := range contribList {
			contribList[i].Percent = (contribList[i].Amount / totalGroup) * 100
		}
	}
	tbMembers := make([]Member, 0, len(members))
	for _, m := range members {
		tbMembers = append(tbMembers, Member{CompanyID: m.CompanyID, Name: m.Name, Enabled: m.Enabled})
	}
	return TrialBalance{
		Filters: Filters{
			GroupID:  filter.GroupID,
			Period:   filter.Period,
			Entities: filter.Entities,
		},
		GroupName:     groupName,
		ReportingCCY:  ccy,
		PeriodDisplay: filter.Period,
		Totals: Totals{
			Local:     totalLocal,
			Group:     totalGroup,
			Balanced:  math.Abs(totalGroup) <= 0.01,
			Refreshed: s.now().UTC(),
		},
		Lines:         balances,
		Contributions: contribList,
		Members:       tbMembers,
	}, nil
}
