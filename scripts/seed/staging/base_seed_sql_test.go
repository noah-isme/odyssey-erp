package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// baseSeedSourceFiles are the files that hold base seeder SQL.
var baseSeedSourceFiles = []string{"base_seed.go", "base_seed_prereqs.go"}

// baseSQLLiterals returns the string literals of the base seeder files that
// start with a SQL keyword.
func baseSQLLiterals(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, file := range baseSeedSourceFiles {
		out = append(out, sqlLiteralsOf(t, file)...)
	}
	if len(out) < 20 {
		t.Fatalf("found only %d SQL literals in %v; the scan is not looking at the right files", len(out), baseSeedSourceFiles)
	}
	return out
}

func sqlLiteralsOf(t *testing.T, file string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
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
	return out
}

// TestBaseSeedSQLNeverModifiesRows is the static fail-closed check: the base
// seeder may only SELECT and INSERT ... ON CONFLICT DO NOTHING, so it can
// never rebind or change a row it did not create in this run. The single
// reviewed exception is baseRevokeRoleOneSQL, which must appear exactly once.
func TestBaseSeedSQLNeverModifiesRows(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)\b(update|delete|truncate|alter|drop|merge|grant|create)\b`)
	first := regexp.MustCompile(`(?is)^\s*(\w+)`)
	revokes := 0
	for _, lit := range baseSQLLiterals(t) {
		if lit == baseRevokeRoleOneSQL {
			revokes++
			continue
		}
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
	if revokes != 1 {
		t.Errorf("baseRevokeRoleOneSQL appears %d times; want exactly 1", revokes)
	}
	for _, file := range baseSeedSourceFiles {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToUpper(string(raw)), "DO UPDATE") {
			t.Errorf("%s contains DO UPDATE", file)
		}
	}
}

// TestBaseSeedRevokeIsScopedToRoleOne pins the shape of the only DELETE: role
// id 1, never an admin/administrator role, only cmms/qms/documents grants.
func TestBaseSeedRevokeIsScopedToRoleOne(t *testing.T) {
	for _, want := range []string{
		"DELETE FROM role_permissions rp",
		"rp.role_id = 1",
		"NOT IN ('admin', 'administrator')",
		"(p.name LIKE 'cmms.%' OR p.name LIKE 'qms.%' OR p.name LIKE 'documents.%')",
		"RETURNING r.name, p.name",
	} {
		if !strings.Contains(baseRevokeRoleOneSQL, want) {
			t.Errorf("baseRevokeRoleOneSQL lacks %q", want)
		}
	}
}

// baseCertifiedRouteHandlers are the route files whose RequireAny/RequireAll
// permissions the certification admin must hold.
var baseCertifiedRouteHandlers = []string{
	"internal/ar/handler.go",
	"internal/ap/handler.go",
	"internal/sales/customers/routes.go",
	"internal/sales/orders/routes.go",
	"internal/sales/quotations/routes.go",
	"internal/delivery/orders/handler.go",
	"internal/inventory/handler.go",
	"internal/documents/http/handler.go",
	"internal/cmms/http/handler.go",
	"internal/approvals/handler.go",
}

// TestBaseSeedAdminPermissionsMatchRouteHandlers re-derives the admin
// permission set from the RequireAny/RequireAll calls of the certified route
// handlers (string literals and internal/shared constants), so a route
// permission added later cannot be missed by the seeder silently.
func TestBaseSeedAdminPermissionsMatchRouteHandlers(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	consts := map[string]string{}
	sharedFiles, err := filepath.Glob(filepath.Join(root, "internal", "shared", "*.go"))
	if err != nil || len(sharedFiles) == 0 {
		t.Fatalf("glob internal/shared: %v (%d files)", err, len(sharedFiles))
	}
	for _, file := range sharedFiles {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
							v, _ := strconv.Unquote(bl.Value)
							consts[name.Name] = v
						}
					}
				}
			}
		}
	}

	want := map[string]bool{}
	for _, rel := range baseCertifiedRouteHandlers {
		file := filepath.Join(root, filepath.FromSlash(rel))
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		calls := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "RequireAny" && sel.Sel.Name != "RequireAll") {
				return true
			}
			calls++
			for _, arg := range call.Args {
				switch a := arg.(type) {
				case *ast.BasicLit:
					v, err := strconv.Unquote(a.Value)
					if err != nil {
						t.Fatalf("%s: unquote %s: %v", rel, a.Value, err)
					}
					want[v] = true
				case *ast.SelectorExpr:
					v, ok := consts[a.Sel.Name]
					if !ok {
						t.Fatalf("%s: cannot resolve permission constant %s", rel, a.Sel.Name)
					}
					want[v] = true
				default:
					t.Fatalf("%s: unsupported permission argument %T", rel, arg)
				}
			}
			return true
		})
		if calls == 0 {
			t.Fatalf("%s has no RequireAny/RequireAll calls; the scan is not looking at the right file", rel)
		}
	}

	got := map[string]bool{}
	for _, p := range baseAdminPermissions {
		if got[p] {
			t.Errorf("baseAdminPermissions lists %q twice", p)
		}
		got[p] = true
	}
	for p := range want {
		if !got[p] {
			t.Errorf("route permission %q is missing from baseAdminPermissions", p)
		}
	}
	for p := range got {
		if !want[p] {
			t.Errorf("baseAdminPermissions lists %q, which no certified route handler requires", p)
		}
	}
	if !sort.StringsAreSorted(baseAdminPermissions) {
		t.Error("baseAdminPermissions must stay sorted")
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
	if _, err := seedStagingFixtures(t.Context(), nil, creds, false); err == nil || !strings.Contains(err.Error(), "must be distinct") {
		t.Fatalf("expected duplicate email refusal, got %v", err)
	}
	creds.BranchEmail = ""
	if _, err := seedStagingFixtures(t.Context(), nil, creds, false); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("expected empty email refusal, got %v", err)
	}
}
