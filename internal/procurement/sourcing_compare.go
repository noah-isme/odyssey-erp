package procurement

import (
	"fmt"
	"math/big"
	"sort"
)

type comparisonCandidate struct {
	RFQLineID, BidID, BidLineID, SupplierID            int64
	TotalBaseAmount                                    string
	LeadTimeDays, CommercialScore, SupplierRatingScore int
}

func calculateComparison(weights RFQWeights, candidates []comparisonCandidate) ([]ComparisonEntry, error) {
	if !weights.Valid() {
		return nil, fmt.Errorf("invalid RFQ comparison weights")
	}
	byLine := make(map[int64][]comparisonCandidate)
	for _, candidate := range candidates {
		if _, ok := new(big.Rat).SetString(candidate.TotalBaseAmount); !ok {
			return nil, fmt.Errorf("invalid base amount %q", candidate.TotalBaseAmount)
		}
		byLine[candidate.RFQLineID] = append(byLine[candidate.RFQLineID], candidate)
	}
	entries := make([]ComparisonEntry, 0, len(candidates))
	for _, lineCandidates := range byLine {
		minAmount := new(big.Rat).SetInt64(-1)
		minLead := -1
		for _, candidate := range lineCandidates {
			amount, _ := new(big.Rat).SetString(candidate.TotalBaseAmount)
			if minAmount.Sign() < 0 || amount.Cmp(minAmount) < 0 {
				minAmount = amount
			}
			if minLead < 0 || candidate.LeadTimeDays < minLead {
				minLead = candidate.LeadTimeDays
			}
		}
		lineEntries := make([]ComparisonEntry, 0, len(lineCandidates))
		for _, candidate := range lineCandidates {
			amount, _ := new(big.Rat).SetString(candidate.TotalBaseAmount)
			priceScore := big.NewRat(0, 1)
			if amount.Sign() == 0 && minAmount.Sign() == 0 {
				priceScore.SetInt64(100)
			} else if amount.Sign() != 0 {
				priceScore.Mul(new(big.Rat).Quo(minAmount, amount), big.NewRat(100, 1))
			}
			leadScore := big.NewRat(100, 1)
			if minLead > 0 {
				leadScore = new(big.Rat).Mul(new(big.Rat).Quo(big.NewRat(int64(minLead), 1), big.NewRat(int64(candidate.LeadTimeDays), 1)), big.NewRat(100, 1))
			} else if candidate.LeadTimeDays > 0 {
				leadScore.SetInt64(0)
			}
			score := weightedScore(priceScore, weights.Price)
			score.Add(score, weightedScore(leadScore, weights.LeadTime))
			score.Add(score, weightedScore(big.NewRat(int64(candidate.CommercialScore), 1), weights.Terms))
			score.Add(score, weightedScore(big.NewRat(int64(candidate.SupplierRatingScore), 1), weights.SupplierRating))
			lineEntries = append(lineEntries, ComparisonEntry{RFQLineID: candidate.RFQLineID, BidID: candidate.BidID, BidLineID: candidate.BidLineID, SupplierID: candidate.SupplierID, TotalBaseAmount: amount.FloatString(4), Score: score.FloatString(4)})
		}
		sort.Slice(lineEntries, func(i, j int) bool {
			a, _ := new(big.Rat).SetString(lineEntries[i].Score)
			b, _ := new(big.Rat).SetString(lineEntries[j].Score)
			if a.Cmp(b) == 0 {
				return lineEntries[i].TotalBaseAmount < lineEntries[j].TotalBaseAmount
			}
			return a.Cmp(b) > 0
		})
		for i := range lineEntries {
			lineEntries[i].Rank = i + 1
		}
		entries = append(entries, lineEntries...)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].RFQLineID == entries[j].RFQLineID {
			return entries[i].Rank < entries[j].Rank
		}
		return entries[i].RFQLineID < entries[j].RFQLineID
	})
	return entries, nil
}

func weightedScore(score *big.Rat, weight int) *big.Rat {
	return new(big.Rat).Quo(new(big.Rat).Mul(score, big.NewRat(int64(weight), 1)), big.NewRat(100, 1))
}
