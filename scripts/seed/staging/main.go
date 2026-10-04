package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
)

type Fixtures struct {
	CompanyID          int64
	BranchID           int64
	OtherCompanyID     int64
	OtherBranchID      int64
	CustomerID         int64
	SupplierID         int64
	ProductID          int64
	WarehouseID        int64
	GRNID              int64
	DocumentCategoryID int64
	DocumentClassID    int64
	Amount             string
	AdminEmail         string
	AdminPassword      string
	BranchEmail        string
	BranchPassword     string
	NoAccessEmail      string
	NoAccessPassword   string
}

func main() {
	var (
		dsnFlag    = flag.String("dsn", "", "PostgreSQL connection string (defaults to PG_DSN env var)")
		applyGH    = flag.Bool("apply-gh", false, "Automatically set variables in GitHub staging environment via gh CLI")
		exportEnv  = flag.Bool("export", false, "Print shell export statements for fixture variables and secrets")
		jsonOutput = flag.Bool("json", false, "Print JSON output of fixtures")

		iso004        = flag.Bool("iso004", false, "Create ISO-004 worker-injection fixtures only (never runs the base seeder; requires --confirm-staging, --iso004-key and APP_ENV=staging)")
		iso004Key     = flag.String("iso004-key", "", "Run key for ISO-004 fixtures (pass the ISO-004 --run-id); required with --iso004")
		confirmTarget = flag.String("confirm-staging", "", "<db-name>@<db-host> that must match the DSN host and current_database(); required by the base seeder and by --iso004")
		denyHostRegex = flag.String("deny-host-regex", stagingDefaultDenyHosts, "Refuse when the DSN host matches this case-insensitive regex (base seeder and --iso004)")
		dryRun        = flag.Bool("dry-run", false, "With --iso004: run the guard and SELECT-only resolution, print the planned inserts, write nothing")
	)
	flag.Parse()

	dsn := *dsnFlag
	if dsn == "" {
		dsn = os.Getenv("PG_DSN")
	}
	explicitDSN := dsn != ""

	if *dryRun && !*iso004 {
		log.Fatalf("--dry-run is only supported with --iso004")
	}
	if *iso004 {
		runISO004Main(iso004Options{
			Guard: iso004GuardInput{
				DSN:            dsn,
				Confirm:        *confirmTarget,
				AppEnv:         os.Getenv("APP_ENV"),
				DenyHostRegex:  *denyHostRegex,
				Key:            *iso004Key,
				ExplicitDSNSet: explicitDSN,
			},
			AdminEmail: getenv("STAGING_CERT_ADMIN_EMAIL", "admin@staging.odyssey.local"),
			DryRun:     *dryRun,
		}, *applyGH, *exportEnv, *jsonOutput)
		return
	}

	runBaseMain(baseOptions{
		Guard: stagingGuardInput{
			Label:          "the base seeder",
			DSN:            dsn,
			Confirm:        *confirmTarget,
			AppEnv:         os.Getenv("APP_ENV"),
			DenyHostRegex:  *denyHostRegex,
			ExplicitDSNSet: explicitDSN,
		},
		Creds: baseCredentialsFromEnv(),
	}, *applyGH, *exportEnv, *jsonOutput)
}

// runBaseMain runs the base seeder behind the staging guard and prints its
// outputs. The per-row created/reused report goes to stderr so --json and
// --export stdout stay machine readable.
func runBaseMain(opts baseOptions, applyGH, exportEnv, jsonOutput bool) {
	ctx := context.Background()
	res, err := runBaseSeed(ctx, opts)
	if err != nil {
		log.Fatalf("seed staging fixtures: %v", err)
	}
	printBaseSteps(os.Stderr, res)

	printSummary(res.Fixtures, applyGH, exportEnv, jsonOutput)

	if applyGH {
		if err := applyToGitHub(res.Fixtures); err != nil {
			log.Fatalf("apply to GitHub staging environment: %v", err)
		}
	}
}

func printSummary(f *Fixtures, applied bool, export bool, jsonOut bool) {
	if jsonOut {
		fmt.Printf(`{
  "STAGING_CERT_COMPANY_ID": %d,
  "STAGING_CERT_BRANCH_ID": %d,
  "STAGING_CERT_OTHER_COMPANY_ID": %d,
  "STAGING_CERT_OTHER_BRANCH_ID": %d,
  "STAGING_CERT_CUSTOMER_ID": %d,
  "STAGING_CERT_SUPPLIER_ID": %d,
  "STAGING_CERT_PRODUCT_ID": %d,
  "STAGING_CERT_WAREHOUSE_ID": %d,
  "STAGING_CERT_GRN_ID": %d,
  "STAGING_CERT_DOCUMENT_CATEGORY_ID": %d,
  "STAGING_CERT_DOCUMENT_CLASSIFICATION_ID": %d,
  "STAGING_CERT_AMOUNT": "%s"
}
`,
			f.CompanyID, f.BranchID, f.OtherCompanyID, f.OtherBranchID,
			f.CustomerID, f.SupplierID, f.ProductID, f.WarehouseID,
			f.GRNID, f.DocumentCategoryID, f.DocumentClassID, f.Amount)
		return
	}

	fmt.Println("================================================================================")
	fmt.Println("             ODYSSEY v0.10-CORE STAGING CERTIFICATION FIXTURES                  ")
	fmt.Println("================================================================================")
	fmt.Printf("1.  STAGING_CERT_COMPANY_ID                 = %d\n", f.CompanyID)
	fmt.Printf("2.  STAGING_CERT_BRANCH_ID                  = %d\n", f.BranchID)
	fmt.Printf("3.  STAGING_CERT_OTHER_COMPANY_ID           = %d\n", f.OtherCompanyID)
	fmt.Printf("4.  STAGING_CERT_OTHER_BRANCH_ID            = %d\n", f.OtherBranchID)
	fmt.Printf("5.  STAGING_CERT_CUSTOMER_ID                = %d\n", f.CustomerID)
	fmt.Printf("6.  STAGING_CERT_SUPPLIER_ID                = %d\n", f.SupplierID)
	fmt.Printf("7.  STAGING_CERT_PRODUCT_ID                 = %d\n", f.ProductID)
	fmt.Printf("8.  STAGING_CERT_WAREHOUSE_ID               = %d\n", f.WarehouseID)
	fmt.Printf("9.  STAGING_CERT_GRN_ID                     = %d\n", f.GRNID)
	fmt.Printf("10. STAGING_CERT_DOCUMENT_CATEGORY_ID       = %d\n", f.DocumentCategoryID)
	fmt.Printf("11. STAGING_CERT_DOCUMENT_CLASSIFICATION_ID = %d\n", f.DocumentClassID)
	fmt.Printf("12. STAGING_CERT_AMOUNT                     = %s\n", f.Amount)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Test Identities Seeded:")
	fmt.Printf("  Admin:     %s / (configured password)\n", f.AdminEmail)
	fmt.Printf("  Branch:    %s / (configured password) [Expired ISO-003 Role + Completed Revoke]\n", f.BranchEmail)
	fmt.Printf("  No-Access: %s / (configured password) [No Company Membership]\n", f.NoAccessEmail)
	fmt.Println("================================================================================")

	if export {
		fmt.Println("\n# Shell export commands:")
		fmt.Printf("export STAGING_CERT_COMPANY_ID=\"%d\"\n", f.CompanyID)
		fmt.Printf("export STAGING_CERT_BRANCH_ID=\"%d\"\n", f.BranchID)
		fmt.Printf("export STAGING_CERT_OTHER_COMPANY_ID=\"%d\"\n", f.OtherCompanyID)
		fmt.Printf("export STAGING_CERT_OTHER_BRANCH_ID=\"%d\"\n", f.OtherBranchID)
		fmt.Printf("export STAGING_CERT_CUSTOMER_ID=\"%d\"\n", f.CustomerID)
		fmt.Printf("export STAGING_CERT_SUPPLIER_ID=\"%d\"\n", f.SupplierID)
		fmt.Printf("export STAGING_CERT_PRODUCT_ID=\"%d\"\n", f.ProductID)
		fmt.Printf("export STAGING_CERT_WAREHOUSE_ID=\"%d\"\n", f.WarehouseID)
		fmt.Printf("export STAGING_CERT_GRN_ID=\"%d\"\n", f.GRNID)
		fmt.Printf("export STAGING_CERT_DOCUMENT_CATEGORY_ID=\"%d\"\n", f.DocumentCategoryID)
		fmt.Printf("export STAGING_CERT_DOCUMENT_CLASSIFICATION_ID=\"%d\"\n", f.DocumentClassID)
		fmt.Printf("export STAGING_CERT_AMOUNT=\"%s\"\n", f.Amount)
	}

	if !applied {
		fmt.Println("\nTo apply these 12 fixture variables to GitHub staging environment:")
		fmt.Printf("gh variable set STAGING_CERT_COMPANY_ID --body \"%d\" --env staging\n", f.CompanyID)
		fmt.Printf("gh variable set STAGING_CERT_BRANCH_ID --body \"%d\" --env staging\n", f.BranchID)
		fmt.Printf("gh variable set STAGING_CERT_OTHER_COMPANY_ID --body \"%d\" --env staging\n", f.OtherCompanyID)
		fmt.Printf("gh variable set STAGING_CERT_OTHER_BRANCH_ID --body \"%d\" --env staging\n", f.OtherBranchID)
		fmt.Printf("gh variable set STAGING_CERT_CUSTOMER_ID --body \"%d\" --env staging\n", f.CustomerID)
		fmt.Printf("gh variable set STAGING_CERT_SUPPLIER_ID --body \"%d\" --env staging\n", f.SupplierID)
		fmt.Printf("gh variable set STAGING_CERT_PRODUCT_ID --body \"%d\" --env staging\n", f.ProductID)
		fmt.Printf("gh variable set STAGING_CERT_WAREHOUSE_ID --body \"%d\" --env staging\n", f.WarehouseID)
		fmt.Printf("gh variable set STAGING_CERT_GRN_ID --body \"%d\" --env staging\n", f.GRNID)
		fmt.Printf("gh variable set STAGING_CERT_DOCUMENT_CATEGORY_ID --body \"%d\" --env staging\n", f.DocumentCategoryID)
		fmt.Printf("gh variable set STAGING_CERT_DOCUMENT_CLASSIFICATION_ID --body \"%d\" --env staging\n", f.DocumentClassID)
		fmt.Printf("gh variable set STAGING_CERT_AMOUNT --body \"%s\" --env staging\n", f.Amount)
		fmt.Println("\nOr re-run the seeder with --apply-gh (same --confirm-staging <db-name>@<db-host> and APP_ENV=staging)")
	}
}

func applyToGitHub(f *Fixtures) error {
	vars := map[string]string{
		"STAGING_CERT_COMPANY_ID":                 strconv.FormatInt(f.CompanyID, 10),
		"STAGING_CERT_BRANCH_ID":                  strconv.FormatInt(f.BranchID, 10),
		"STAGING_CERT_OTHER_COMPANY_ID":           strconv.FormatInt(f.OtherCompanyID, 10),
		"STAGING_CERT_OTHER_BRANCH_ID":            strconv.FormatInt(f.OtherBranchID, 10),
		"STAGING_CERT_CUSTOMER_ID":                strconv.FormatInt(f.CustomerID, 10),
		"STAGING_CERT_SUPPLIER_ID":                strconv.FormatInt(f.SupplierID, 10),
		"STAGING_CERT_PRODUCT_ID":                 strconv.FormatInt(f.ProductID, 10),
		"STAGING_CERT_WAREHOUSE_ID":               strconv.FormatInt(f.WarehouseID, 10),
		"STAGING_CERT_GRN_ID":                     strconv.FormatInt(f.GRNID, 10),
		"STAGING_CERT_DOCUMENT_CATEGORY_ID":       strconv.FormatInt(f.DocumentCategoryID, 10),
		"STAGING_CERT_DOCUMENT_CLASSIFICATION_ID": strconv.FormatInt(f.DocumentClassID, 10),
		"STAGING_CERT_AMOUNT":                     f.Amount,
	}

	fmt.Println("\n→ Applying 12 variables to GitHub environment 'staging'...")
	for name, val := range vars {
		cmd := exec.Command("gh", "variable", "set", name, "--body", val, "--env", "staging")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("gh variable set %s: %v, output: %s", name, err, string(output))
		}
		fmt.Printf("  ✓ Set %s = %s\n", name, val)
	}
	fmt.Println("All 12 variables successfully set in GitHub staging environment!")
	return nil
}

// runISO004Main runs the --iso004 extension and prints its outputs. It never
// calls seedStagingFixtures.
func runISO004Main(opts iso004Options, applyGH, exportEnv, jsonOutput bool) {
	ctx := context.Background()
	res, err := runISO004(ctx, opts)
	if err != nil {
		log.Fatalf("iso004 fixtures: %v", err)
	}
	if opts.DryRun {
		printISO004Plan(os.Stdout, res)
		return
	}
	if err := printISO004Summary(os.Stdout, os.Stderr, res, applyGH, exportEnv, jsonOutput); err != nil {
		log.Fatalf("print iso004 summary: %v", err)
	}
	if applyGH {
		progress := os.Stdout
		if jsonOutput {
			progress = os.Stderr
		}
		if err := applyISO004ToGitHub(progress, res); err != nil {
			log.Fatalf("apply ISO-004 variables to GitHub staging environment: %v", err)
		}
	}
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
