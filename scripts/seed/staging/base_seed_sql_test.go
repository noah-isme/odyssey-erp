package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// baseSQLLiterals returns the string literals of base_seed.go that start with
// a SQL keyword.
func baseSQLLiterals(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "base_seed.go", nil, 0)
	if err != nil {
		t.Fatalf("parse base_seed.go: %v", err)
	}
	sqlStart := regexp.MustCompile(`(?is)^\s*(select|insert|update|delete|with|alter|drop|truncate|merge|grant|create|set)\b`)
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		bl, ok := n.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return true
		}
		v, err := strconv.Unquote(bl.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", bl.Value, err)
		}
		// Error and log messages can start with "set"/"create"; SQL never
		// contains a printf verb.
		if sqlStart.MatchString(v) && !regexp.MustCompile(`%[wqdsv]`).MatchString(v) {
			out = append(out, v)
		}
		return true
	})
	if len(out) < 20 {
		t.Fatalf("found only %d SQL literals in base_seed.go; the scan is not looking at the right file", len(out))
	}
	return out
}

// TestBaseSeedSQLNeverModifiesRows is the static fail-closed check: the base
// seeder may only SELECT and INSERT ... ON CONFLICT DO NOTHING, so it can
// never rebind or change a row it did not create in this run.
func TestBaseSeedSQLNeverModifiesRows(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)\b(update|delete|truncate|alter|drop|merge|grant|create)\b`)
	first := regexp.MustCompile(`(?is)^\s*(\w+)`)
	for _, lit := range baseSQLLiterals(t) {
		if m := forbidden.FindString(lit); m != "" {
			t.Errorf("SQL literal contains %q: %s", m, lit)
		}
		switch kw := strings.ToLower(first.FindStringSubmatch(lit)[1]); kw {
		case "select", "insert":
		case "set":
			if !regexp.MustCompile(`(?i)^\s*set\s+local\b`).MatchString(lit) {
				t.Errorf("only SET LOCAL is allowed: %s", lit)
			}
		default:
			t.Errorf("unexpected SQL statement kind %q: %s", kw, lit)
		}
	}
	raw, err := os.ReadFile("base_seed.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(string(raw)), "DO UPDATE") {
		t.Error("base_seed.go contains DO UPDATE")
	}
}

func TestBaseSeedInsertsAreDoNothingReturning(t *testing.T) {
	insert := regexp.MustCompile(`(?i)\binsert\s+into\b`)
	doNothing := regexp.MustCompile(`(?is)ON\s+CONFLICT\s+DO\s+NOTHING\s+RETURNING\s+\w+`)
	found := 0
	for _, lit := range baseSQLLiterals(t) {
		if !insert.MatchString(lit) {
			continue
		}
		found++
		if !doNothing.MatchString(lit) {
			t.Errorf("INSERT without ON CONFLICT DO NOTHING RETURNING: %s", lit)
		}
	}
	if found < 15 {
		t.Fatalf("found only %d INSERT statements; the scan is not looking at the right file", found)
	}
}

// TestBaseSeederOnlyReachedThroughGuard keeps the unguarded seeding function
// out of main.go: the only caller must be runBaseSeed, which runs the guard.
func TestBaseSeederOnlyReachedThroughGuard(t *testing.T) {
	mainFile, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(mainFile, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "seedStagingFixtures" {
			t.Error("main.go must reach the base seeder only through runBaseSeed (staging guard)")
		}
		return true
	})
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "base_seed.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "runBaseSeed" {
			continue
		}
		var guard, seed token.Pos
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				switch id.Name {
				case "checkStagingStatic":
					guard = id.Pos()
				case "seedStagingFixtures":
					seed = id.Pos()
				}
			}
			return true
		})
		if guard == token.NoPos || seed == token.NoPos || guard > seed {
			t.Error("runBaseSeed must run checkStagingStatic before seedStagingFixtures")
		}
		return
	}
	t.Fatal("runBaseSeed not found in base_seed.go")
}

func TestBaseSeedRejectsDuplicateIdentityEmails(t *testing.T) {
	// The check runs before any database access, so a nil pool is safe.
	creds := baseCredentials{
		AdminEmail: "Same@staging.local", BranchEmail: "same@staging.local", NoAccessEmail: "other@staging.local",
	}
	if _, _, err := seedStagingFixtures(t.Context(), nil, creds); err == nil || !strings.Contains(err.Error(), "must be distinct") {
		t.Fatalf("expected duplicate email refusal, got %v", err)
	}
	creds.BranchEmail = ""
	if _, _, err := seedStagingFixtures(t.Context(), nil, creds); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("expected empty email refusal, got %v", err)
	}
}
