package treasury

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAmountJSONRequiresDecimalString(t *testing.T) {
	var amount Amount
	if err := json.Unmarshal([]byte(`"123.4500"`), &amount); err != nil {
		t.Fatalf("string amount should parse: %v", err)
	}
	if amount.String() != "123.4500" {
		t.Fatalf("amount = %q, want 123.4500", amount)
	}
	encoded, err := json.Marshal(amount)
	if err != nil {
		t.Fatalf("string amount should marshal: %v", err)
	}
	if string(encoded) != `"123.4500"` {
		t.Fatalf("encoded amount = %s, want a JSON string", encoded)
	}

	for _, input := range []string{`123.45`, `null`, `"1e2"`} {
		var parsed Amount
		if err := json.Unmarshal([]byte(input), &parsed); err == nil {
			t.Fatalf("input %s should be rejected as a non-decimal-string amount", input)
		}
	}
}

func TestParseAmountEnforcesNumericPrecisionAndScale(t *testing.T) {
	for _, input := range []string{
		"1.23456",               // more than NUMERIC(19,4) scale
		"1234567890123456.0000", // more than 15 integer digits
		"1.2e3",                 // exponent notation is not a wire decimal
		"",                      // empty values must not become zero
	} {
		if _, err := ParseAmount(input); !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("ParseAmount(%q) error = %v, want ErrInvalidAmount", input, err)
		}
	}

	for _, input := range []string{"0", "0.0000", "999999999999999.9999", "-12.3456"} {
		if _, err := ParseAmount(input); err != nil {
			t.Fatalf("ParseAmount(%q) unexpected error: %v", input, err)
		}
	}
}

func TestAmountAdditionIsExact(t *testing.T) {
	left := MustParseAmount("0.1")
	right := MustParseAmount("0.2")
	total, err := left.Add(right)
	if err != nil {
		t.Fatalf("Add() unexpected error: %v", err)
	}
	if total.String() != "0.3" {
		t.Fatalf("0.1 + 0.2 = %q, want 0.3", total)
	}

	total, err = MustParseAmount("1.2345").Add(MustParseAmount("2.0000"))
	if err != nil {
		t.Fatalf("scaled Add() unexpected error: %v", err)
	}
	if total.String() != "3.2345" {
		t.Fatalf("scaled total = %q, want 3.2345", total)
	}
}

func TestParseAmountCanonicalizesNegativeZero(t *testing.T) {
	for _, input := range []string{"-0", "-0.0", "-000.0000"} {
		amount, err := ParseAmount(input)
		if err != nil {
			t.Fatalf("ParseAmount(%q) unexpected error: %v", input, err)
		}
		if amount.String() != "0" && amount.String() != "0.0" && amount.String() != "0.0000" {
			t.Fatalf("ParseAmount(%q) = %q, want a positive zero", input, amount)
		}
		if amount.IsPositive() {
			t.Fatalf("ParseAmount(%q) should not be positive", input)
		}
	}
}

func TestAmountFromNumericPreservesDecimalText(t *testing.T) {
	var numeric pgtype.Numeric
	if err := numeric.Scan("900719925474099.9999"); err != nil {
		t.Fatalf("numeric scan failed: %v", err)
	}
	amount, err := amountFromNumeric(numeric)
	if err != nil {
		t.Fatalf("amountFromNumeric() unexpected error: %v", err)
	}
	if amount.String() != "900719925474099.9999" {
		t.Fatalf("mapped amount = %q, want exact NUMERIC text", amount)
	}

	var invalid pgtype.Numeric
	if _, err := amountFromNumeric(invalid); err == nil {
		t.Fatal("invalid NUMERIC should be rejected")
	}
}
