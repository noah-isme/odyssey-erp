package tax

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type coretaxEnvelope struct {
	XMLName xml.Name        `xml:"TaxExport"`
	Schema  string          `xml:"schemaVersion,attr"`
	Records []coretaxRecord `xml:"Documents>Document"`
}
type coretaxRecord struct {
	TaxNumber, DocumentNumber, CounterpartyName, CounterpartyTaxID string
	IssueDate, TaxableBase, TaxAmount                              string
	Sign                                                           *int `xml:"Sign,omitempty"`
}

// renderCoretaxXML emits Odyssey's canonical XML projection. Activation still
// requires a reviewed official schema and a validator for that exact version;
// this function must not be treated as proof of Coretax acceptance by itself.
func renderCoretaxXML(schema ExportSchema, records []ExportRecord) ([]byte, Money, Money, error) {
	if schema.Version == "" {
		return nil, 0, 0, ErrConfiguration
	}
	envelope := coretaxEnvelope{Schema: schema.Version, Records: make([]coretaxRecord, 0, len(records))}
	var base, amount Money
	for _, record := range records {
		if record.Sign != 1 && record.Sign != -1 {
			return nil, 0, 0, fmt.Errorf("tax: invalid export sign")
		}
		var sign *int
		if schema.IncludeSignElement {
			value := record.Sign
			sign = &value
		}
		envelope.Records = append(envelope.Records, coretaxRecord{
			TaxNumber: record.TaxNumber, DocumentNumber: record.DocumentNumber,
			CounterpartyName: record.CounterpartyName, CounterpartyTaxID: record.CounterpartyTaxID,
			IssueDate: record.IssueDate.Format(time.DateOnly), TaxableBase: strconv.FormatInt(int64(record.TaxableBase)*int64(record.Sign), 10),
			TaxAmount: strconv.FormatInt(int64(record.TaxAmount)*int64(record.Sign), 10), Sign: sign,
		})
		base += record.TaxableBase * Money(record.Sign)
		amount += record.TaxAmount * Money(record.Sign)
	}
	var out bytes.Buffer
	declaration := strings.TrimSpace(schema.XMLDeclaration)
	if declaration == "" {
		declaration = strings.TrimSpace(xml.Header)
	}
	out.WriteString(declaration)
	out.WriteByte('\n')
	encoder := xml.NewEncoder(&out)
	encoder.Indent("", "  ")
	if err := encoder.Encode(envelope); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), base, amount, nil
}
