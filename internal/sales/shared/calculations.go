package shared

func CalculateLineTotals(quantity, unitPrice, discountPercent, taxPercent float64) (discountAmount, taxAmount, lineTotal float64) {
	grossAmount := quantity * unitPrice
	discountAmount = grossAmount * (discountPercent / 100)
	netAmount := grossAmount - discountAmount
	taxAmount = netAmount * (taxPercent / 100)
	lineTotal = netAmount + taxAmount
	return
}
