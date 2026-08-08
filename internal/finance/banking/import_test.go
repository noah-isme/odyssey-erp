package banking

import "testing"

func TestParseCSVStatement(t *testing.T) {
	entries, err := parseStatement("statement.csv", []byte("date,amount,description,reference\n2026-07-01,150000,Customer payment,REF-1\n2026-07-02,-25000,Bank fee,REF-2\n"), "IDR", 1)
	if err != nil {
		t.Fatalf("parseStatement() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Amount.Amount.Amount != "150000" || entries[1].Amount.Amount.Amount != "-25000" || entries[0].Reference != "REF-1" {
		t.Fatalf("parseStatement() = %#v", entries)
	}
}

func TestParseOFXStatement(t *testing.T) {
	content := []byte("<OFX><BANKTRANLIST><STMTTRN><DTPOSTED>20260703<TRNAMT>12000.50<FITID>OFX-1<NAME>Interest</STMTTRN></BANKTRANLIST></OFX>")
	entries, err := parseStatement("statement.ofx", content, "IDR", 1)
	if err != nil {
		t.Fatalf("parseStatement() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Amount.Amount.Amount != "12000.50" || entries[0].Reference != "OFX-1" {
		t.Fatalf("parseStatement() = %#v", entries)
	}
}
