package main

import (
	"context"
	"fmt"
)

// Reviewed tax configuration for staging (FIND-028, release-owner decision:
// seed it for staging, no product handling change). AR and AP posting
// capture a tax document after the posting commits
// (internal/tax/repository.go capture); on a database without reviewed tax
// configuration that capture fails with tax.ErrConfiguration ("tax: reviewed
// configuration is incomplete"). The capture needs, at the posting date:
//
//   - a company_tax_identities row of the posting company;
//   - a reviewed tax_rule_versions row with a tax_codes row of tax_kind
//     VAT_OUTPUT (AR) and VAT_INPUT (AP);
//   - a tax_account_mappings row of the company for VAT_OUTPUT and VAT_INPUT;
//   - for AR, a tax_invoice_number_ranges row of the company with capacity;
//   - an accounting_periods row covering the date (ensureLedgerPrerequisites);
//   - a non-empty tax_id on the customer and the supplier (the base customer
//     and supplier are created with one, see base_seed.go).
//
// The rule version is a staging certification fixture, not a regulatory
// review: its source_url and source_checksum say so, it has no VAT rate, and
// it is "reviewed" by the certification admin identity. Every row follows the
// base_seed.go rules: looked up first, reused when it already satisfies the
// requirement, never changed; a conflicting existing configuration (a
// partial reviewed VAT rule set, identities or mappings of the company that
// are not effective today, an exhausted seeded number range) is refused.
const (
	baseTaxRuleVersionCode = "STG-CERT-VAT-V010"
	baseTaxRuleSourceURL   = "urn:odyssey:staging-certification-fixture:not-a-regulatory-source"
	baseTaxRuleChecksum    = "staging-certification-fixture"
	baseTaxEffectiveFrom   = "2000-01-01"
	baseTaxNumberPrefix    = "STG-"
	baseTaxCompanyNPWP     = "0012345678901000"
	baseTaxCompanyNITKU    = "0012345678901000000000"
	baseCustomerTaxID      = "0098765432101000"
	baseSupplierTaxID      = "0087654321012000"
)

// baseTaxCodes are the reviewed tax codes the capture looks up by tax_kind.
var baseTaxCodes = []struct{ Code, Name, Kind string }{
	{"STG-VAT-OUT", "Staging VAT output (certification fixture)", "VAT_OUTPUT"},
	{"STG-VAT-IN", "Staging VAT input (certification fixture)", "VAT_INPUT"},
}

// baseTaxAccounts are the accounts the VAT tax account mappings point at,
// keyed by category. As in baseLedgerAccounts, an account is created or
// checked only when the mapping is absent, and an existing code is reused
// only when it is active and of the listed type.
var baseTaxAccounts = []struct{ Category, Code, Name, Type string }{
	{"VAT_OUTPUT", "2110", "VAT Output", "LIABILITY"},
	{"VAT_INPUT", "1410", "VAT Input", "ASSET"},
}

// ensureTaxConfiguration seeds the reviewed tax configuration AR and AP tax
// capture need for the primary company.
func (s *baseSeeder) ensureTaxConfiguration(ctx context.Context, companyID, adminUserID int64) error {
	if err := s.ensureTaxCodes(ctx, adminUserID); err != nil {
		return err
	}

	// Company tax identity: one effective today is reused. The seeded one
	// is open-ended from baseTaxEffectiveFrom, so any other identity of the
	// company would overlap it (company_tax_identities_no_overlap).
	identity := baseFixture{
		Name: "company_tax_identity_primary", Table: "company_tax_identities",
		Insert: `INSERT INTO company_tax_identities (company_id, legal_name, npwp, nitku, registered_address, effective_from, created_by)
			VALUES ($1, $2, $3, $4, $5, $6::date, $7) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{companyID, baseCompanyName, baseTaxCompanyNPWP, baseTaxCompanyNITKU, "Jl. Sudirman No. 100, Jakarta", baseTaxEffectiveFrom, adminUserID},
		Lookup: `SELECT id, 'ok' FROM company_tax_identities
			WHERE company_id = $1 AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
			ORDER BY id LIMIT 1`,
		LookupArgs:  []any{companyID},
		Want:        "ok",
		LookupFirst: true,
	}
	if err := s.refuseIfOthers(ctx, identity, `SELECT COUNT(*) FROM company_tax_identities WHERE company_id = $1`, companyID); err != nil {
		return err
	}
	if _, err := s.mustEnsure(ctx, identity); err != nil {
		return err
	}

	// Tax account mappings per category.
	for _, a := range baseTaxAccounts {
		mapping := baseFixture{
			Name: "tax_account_mapping " + a.Category, Table: "tax_account_mappings",
			Insert: `INSERT INTO tax_account_mappings (company_id, category, account_id, effective_from, created_by)
				VALUES ($1, $2, $3, $4::date, $5) ON CONFLICT DO NOTHING RETURNING id`,
			Lookup: `SELECT id, 'ok' FROM tax_account_mappings
				WHERE company_id = $1 AND category = $2 AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
				ORDER BY id LIMIT 1`,
			LookupArgs:  []any{companyID, a.Category},
			Want:        "ok",
			LookupFirst: true,
		}
		if id, state, found, err := s.lookup(ctx, mapping); err != nil {
			return err
		} else if found {
			if _, _, err := s.reuse(mapping, id, state); err != nil {
				return err
			}
			continue
		}
		if err := s.refuseIfOthers(ctx, mapping, `SELECT COUNT(*) FROM tax_account_mappings WHERE company_id = $1 AND category = $2`, companyID, a.Category); err != nil {
			return err
		}
		accountID, err := s.mustEnsure(ctx, baseFixture{
			Name: "account " + a.Code, Table: "accounts",
			Insert:     `INSERT INTO accounts (code, name, type, is_active) VALUES ($1, $2, $3::account_type, TRUE) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: []any{a.Code, a.Name, a.Type},
			Lookup: `SELECT id, type::text || CASE WHEN is_active THEN '' ELSE ' inactive' END
				FROM accounts WHERE code = $1`,
			LookupArgs:  []any{a.Code},
			Want:        a.Type,
			LookupFirst: true,
		})
		if err != nil {
			return fmt.Errorf("tax account mapping %s needs an active %s account %s: %w", a.Category, a.Type, a.Code, err)
		}
		mapping.InsertArgs = []any{companyID, a.Category, accountID, baseTaxEffectiveFrom, adminUserID}
		if _, err := s.mustEnsure(ctx, mapping); err != nil {
			return err
		}
	}

	// Tax invoice number range for AR: any range of the company effective
	// today with numbers left is reused; otherwise the seeded range is
	// created, and refused when it exists but is exhausted.
	usable := baseFixture{
		Name: "tax_invoice_number_range_primary", Table: "tax_invoice_number_ranges",
		Lookup: `SELECT id, 'ok' FROM tax_invoice_number_ranges
			WHERE company_id = $1 AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE) AND next_number <= range_end
			ORDER BY effective_from DESC, id LIMIT 1`,
		LookupArgs: []any{companyID},
		Want:       "ok",
	}
	if id, state, found, err := s.lookup(ctx, usable); err != nil {
		return err
	} else if found {
		_, _, err := s.reuse(usable, id, state)
		return err
	}
	_, err := s.mustEnsure(ctx, baseFixture{
		Name: usable.Name, Table: usable.Table,
		Insert: `INSERT INTO tax_invoice_number_ranges (company_id, prefix, range_start, range_end, next_number, effective_from, created_by)
			VALUES ($1, $2, 1, 99999999, 1, $3::date, $4) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{companyID, baseTaxNumberPrefix, baseTaxEffectiveFrom, adminUserID},
		Lookup: `SELECT id, CASE WHEN next_number <= range_end AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
				THEN 'ok' ELSE 'exhausted or not effective today' END
			FROM tax_invoice_number_ranges WHERE company_id = $1 AND prefix = $2 AND range_start = 1 AND range_end = 99999999`,
		LookupArgs:  []any{companyID, baseTaxNumberPrefix},
		Want:        "ok",
		LookupFirst: true,
	})
	return err
}

// ensureTaxCodes provides one reviewed tax code per kind in baseTaxCodes,
// effective today. The seeded rule version is reused (and completed with its
// own codes); without it, a reviewed operator configuration that already has
// every kind is reused as is. A partial operator configuration, or a
// reviewed VAT rule version the seeded one would overlap
// (tax_rule_versions_no_reviewed_overlap), is refused: the seeder never adds
// codes to an operator's rule version.
func (s *baseSeeder) ensureTaxCodes(ctx context.Context, adminUserID int64) error {
	rule := baseFixture{
		Name: "tax_rule_version_vat", Table: "tax_rule_versions",
		Insert: `INSERT INTO tax_rule_versions (rule_kind, version_code, effective_from, source_url, source_checksum, reviewed_by, reviewed_at, created_by)
			VALUES ('VAT', $1, $2::date, $3, $4, $5, NOW(), $5) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseTaxRuleVersionCode, baseTaxEffectiveFrom, baseTaxRuleSourceURL, baseTaxRuleChecksum, adminUserID},
		Lookup: `SELECT id, CASE WHEN reviewed_at IS NOT NULL AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
				THEN 'ok' ELSE 'not reviewed or not effective today' END
			FROM tax_rule_versions WHERE rule_kind = 'VAT' AND version_code = $1`,
		LookupArgs:  []any{baseTaxRuleVersionCode},
		Want:        "ok",
		LookupFirst: true,
	}
	ruleID, state, found, err := s.lookup(ctx, rule)
	if err != nil {
		return err
	}
	if found {
		if ruleID, _, err = s.reuse(rule, ruleID, state); err != nil {
			return err
		}
	} else {
		// Without the seeded rule version: reuse a complete reviewed
		// operator configuration, refuse a partial one.
		var present, missing []string
		codeIDs := map[string]int64{}
		for _, c := range baseTaxCodes {
			var id int64
			err := s.tx.QueryRow(ctx, `SELECT COALESCE(MAX(tc.id), 0) FROM tax_codes tc JOIN tax_rule_versions rv ON rv.id = tc.rule_version_id
				WHERE tc.tax_kind = $1 AND rv.reviewed_at IS NOT NULL AND rv.effective_from <= CURRENT_DATE AND (rv.effective_to IS NULL OR rv.effective_to >= CURRENT_DATE)`, c.Kind).Scan(&id)
			if err != nil {
				return fmt.Errorf("look up reviewed %s tax code: %w", c.Kind, err)
			}
			if id > 0 {
				present, codeIDs[c.Kind] = append(present, c.Kind), id
			} else {
				missing = append(missing, c.Kind)
			}
		}
		switch {
		case len(missing) == 0:
			for _, c := range baseTaxCodes {
				s.record("tax_code "+c.Kind, "tax_codes", "reused", codeIDs[c.Kind])
			}
			return nil
		case len(present) > 0:
			return fmt.Errorf("the reviewed tax configuration effective today has tax codes for %v but not for %v; the seeder never adds codes to an existing rule version, so refusing", present, missing)
		}
		var overlapping string
		if err := s.tx.QueryRow(ctx, `SELECT COALESCE(string_agg(version_code, ', ' ORDER BY id), '') FROM tax_rule_versions
			WHERE rule_kind = 'VAT' AND reviewed_at IS NOT NULL AND (effective_to IS NULL OR effective_to >= $1::date)`, baseTaxEffectiveFrom).Scan(&overlapping); err != nil {
			return fmt.Errorf("check reviewed VAT rule versions: %w", err)
		}
		if overlapping != "" {
			return fmt.Errorf("reviewed VAT rule version(s) %s have no VAT_OUTPUT/VAT_INPUT tax codes effective today and would overlap the seeded %s; the seeder never changes existing rule versions, so refusing", overlapping, baseTaxRuleVersionCode)
		}
		if ruleID, err = s.mustEnsure(ctx, rule); err != nil {
			return err
		}
	}

	for _, c := range baseTaxCodes {
		if _, err := s.mustEnsure(ctx, baseFixture{
			Name: "tax_code " + c.Kind, Table: "tax_codes",
			Insert:     `INSERT INTO tax_codes (rule_version_id, code, name, tax_kind) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: []any{ruleID, c.Code, c.Name, c.Kind},
			Lookup:     `SELECT id, tax_kind FROM tax_codes WHERE rule_version_id = $1 AND code = $2`,
			LookupArgs: []any{ruleID, c.Code},
			Want:       c.Kind,
		}); err != nil {
			return err
		}
	}
	return nil
}

// refuseIfOthers refuses when count (a COUNT(*) query) finds rows although
// f's lookup found none effective today: the seeded open-ended row would
// overlap them, and the seeder never changes existing rows.
func (s *baseSeeder) refuseIfOthers(ctx context.Context, f baseFixture, count string, args ...any) error {
	if _, _, found, err := s.lookup(ctx, f); err != nil || found {
		return err
	}
	var n int64
	if err := s.tx.QueryRow(ctx, count, args...).Scan(&n); err != nil {
		return fmt.Errorf("count existing %s (%s): %w", f.Name, f.Table, err)
	}
	if n != 0 {
		return fmt.Errorf("%s (%s): %d existing row(s) but none effective today; the seeded row (effective from %s, open-ended) would overlap them and the seeder never changes existing rows, so refusing", f.Name, f.Table, n, baseTaxEffectiveFrom)
	}
	return nil
}
