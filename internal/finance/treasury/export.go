package treasury

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"fmt"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type BankFormatEncoder interface {
	Encode(batch sqlc.TreasuryPaymentBatch, items []sqlc.TreasuryPaymentBatchItem) (payload []byte, hash string, err error)
}

type CSVEncoder struct{}

func (e *CSVEncoder) Encode(batch sqlc.TreasuryPaymentBatch, items []sqlc.TreasuryPaymentBatchItem) ([]byte, string, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)

	// Header
	if err := w.Write([]string{"BatchRef", "SupplierID", "BankAccountID", "Amount", "Currency"}); err != nil {
		return nil, "", err
	}

	for _, item := range items {
		// Convert Numeric to string - using a simplistic string representation here for CSV
		amtStr := ""
		if val, err := item.Amount.Float64Value(); err == nil {
			amtStr = fmt.Sprintf("%.2f", val.Float64)
		} else {
			amtStr = "0.00" // Fallback
		}

		if err := w.Write([]string{
			batch.ReferenceCode,
			fmt.Sprintf("%d", item.SupplierID),
			fmt.Sprintf("%d", item.BankAccountID),
			amtStr,
			batch.Currency,
		}); err != nil {
			return nil, "", err
		}
	}
	w.Flush()

	payload := b.Bytes()
	hash := fmt.Sprintf("%x", sha256.Sum256(payload))
	return payload, hash, nil
}
