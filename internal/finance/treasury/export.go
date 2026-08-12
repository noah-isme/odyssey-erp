package treasury

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"fmt"
)

type BankFormatEncoder interface {
	Encode(batch PaymentBatch, items []PaymentBatchItem) (payload []byte, hash string, err error)
}

type CSVEncoder struct{}

func (e *CSVEncoder) Encode(batch PaymentBatch, items []PaymentBatchItem) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"BatchRef", "SupplierID", "BankAccountID", "Amount", "Currency"}); err != nil {
		return nil, "", err
	}
	for _, item := range items {
		if err := writer.Write([]string{
			batch.ReferenceCode,
			fmt.Sprintf("%d", item.SupplierID),
			fmt.Sprintf("%d", item.BankAccountID),
			item.Amount.String(),
			batch.Currency,
		}); err != nil {
			return nil, "", err
		}
	}
	writer.Flush()
	payload := buffer.Bytes()
	return payload, fmt.Sprintf("%x", sha256.Sum256(payload)), nil
}
