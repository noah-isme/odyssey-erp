package tax

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// ReviewedSchemaValidator verifies the reviewed schema artifact has not changed
// and applies structural validation to the generated XML. Official XSD/portal
// acceptance remains a documented release gate for every new schema version.
type ReviewedSchemaValidator struct{}

func (ReviewedSchemaValidator) Validate(schema ExportSchema, payload []byte) error {
	digest := sha256.Sum256([]byte(schema.Body))
	if !strings.EqualFold(hex.EncodeToString(digest[:]), strings.TrimSpace(schema.OfficialChecksum)) {
		return fmt.Errorf("tax: reviewed schema checksum mismatch")
	}
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	var root bool
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tax: invalid XML: %w", err)
		}
		if start, ok := token.(xml.StartElement); ok && !root {
			if start.Name.Local != "TaxExport" {
				return fmt.Errorf("tax: unexpected XML root")
			}
			root = true
		}
	}
	if !root {
		return fmt.Errorf("tax: empty XML export")
	}
	return nil
}
