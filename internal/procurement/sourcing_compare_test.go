package procurement

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateComparisonRanksEachRFQLineWithExactAmounts(t *testing.T) {
	entries, err := calculateComparison(DefaultRFQWeights(), []comparisonCandidate{
		{RFQLineID: 10, BidID: 1, BidLineID: 11, SupplierID: 100, TotalBaseAmount: "100.0000", LeadTimeDays: 5, CommercialScore: 80, SupplierRatingScore: 90},
		{RFQLineID: 10, BidID: 2, BidLineID: 12, SupplierID: 200, TotalBaseAmount: "120.0000", LeadTimeDays: 2, CommercialScore: 90, SupplierRatingScore: 80},
		{RFQLineID: 20, BidID: 3, BidLineID: 13, SupplierID: 100, TotalBaseAmount: "5.0000", LeadTimeDays: 1, CommercialScore: 70, SupplierRatingScore: 70},
		{RFQLineID: 20, BidID: 4, BidLineID: 14, SupplierID: 200, TotalBaseAmount: "6.0000", LeadTimeDays: 2, CommercialScore: 90, SupplierRatingScore: 90},
	})
	require.NoError(t, err)
	require.Len(t, entries, 4)
	require.Equal(t, int64(10), entries[0].RFQLineID)
	require.Equal(t, 1, entries[0].Rank)
	require.Equal(t, int64(2), entries[0].BidID)
	require.Equal(t, "86.6667", entries[0].Score)
	require.Equal(t, int64(20), entries[2].RFQLineID)
	require.Equal(t, 1, entries[2].Rank)
	require.Equal(t, int64(3), entries[2].BidID)
}

func TestCalculateComparisonHandlesZeroPricedBidsWithoutDivisionByZero(t *testing.T) {
	entries, err := calculateComparison(DefaultRFQWeights(), []comparisonCandidate{
		{RFQLineID: 10, BidID: 1, BidLineID: 11, SupplierID: 100, TotalBaseAmount: "0", LeadTimeDays: 0, CommercialScore: 100, SupplierRatingScore: 100},
		{RFQLineID: 10, BidID: 2, BidLineID: 12, SupplierID: 200, TotalBaseAmount: "10", LeadTimeDays: 3, CommercialScore: 0, SupplierRatingScore: 0},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), entries[0].BidID)
	require.Equal(t, "100.0000", entries[0].Score)
	require.Equal(t, "0.0000", entries[1].Score)
}

func TestSourcingDecimalHelpers(t *testing.T) {
	require.True(t, positiveDecimal("0.0001"))
	require.False(t, positiveDecimal("0"))
	require.True(t, lessOrEqualDecimal("3.125", "3.125"))
	require.True(t, lessOrEqualDecimal(sumDecimal("1.25", "1.75"), "3"))
	require.False(t, lessOrEqualDecimal("3.0001", "3"))
}
