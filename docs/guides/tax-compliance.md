# Tax Compliance

Odyssey provides immutable Indonesian tax documents and ledgers under `/tax`.
Migration `000048_tax_compliance` adds effective-dated rule versions, PPN rates,
PPh 23/PPh 4(2) withholding types, tax codes, company NPWP/NITKU identity,
faktur ranges, account mappings, reporting periods, audit events, and versioned
Coretax export schemas.
Migration `000049_tax_capture_outbox` adds a transactionally written capture
outbox and reviewed per-schema XML declaration/field controls.

## Regulatory configuration

No current rate is silently seeded. Production configuration is usable only
when its version contains the official source URL and checksum plus
`reviewed_by`/`reviewed_at`. Reviewed effective ranges cannot overlap. This is
important because the 2025 PPN treatment may combine a 12% statutory rate with
an 11/12 DPP factor; it must not be flattened into an unversioned 11% formula.

Review configuration against the official DJP references:

- [Coretax XML templates and converters](https://www.pajak.go.id/id/reformdjp/coretax/template-xml-dan-converter-excel-ke-xml)
- [DJP PPh 23/26 guidance](https://www.pajak.go.id/id/pph-pasal-2326)
- [DJP PPh Pasal 4 ayat (2) guidance](https://www.pajak.go.id/id/pph-pasal-4-ayat-2)
- [PMK 11/2025 DPP Nilai Lain](https://www.pajak.go.id/id/peraturan/ketentuan-nilai-lain-sebagai-dasar-pengenaan-pajak-dan-besaran-tertentu-pajak-pertambahan)

Before posting taxable documents, configure:

1. A reviewed VAT or withholding rule version and its rates/codes.
2. Effective company NPWP, NITKU, PKP identity, and registered address.
3. An active faktur numbering range for output VAT.
4. Tax-to-GL mappings for `VAT_OUTPUT`, `VAT_INPUT`, `PPh23`, and `PPh4(2)`.
5. An official export schema artifact whose stored SHA-256 matches its reviewed
   checksum.

## Document and ledger lifecycle

Posting an AR/AP invoice snapshots posted source totals, counterparty tax ID,
the exact effective rule version, DPP, PPN, and source hash. Posted credit/debit
notes use a negative ledger sign. AP withholding is recognized at the configured
invoice or payment event; partial payments prorate the invoice DPP and round to
the rupiah.

Tax documents, withholding records, ledger rows, correction events, and export
records reject update/delete operations in PostgreSQL. Cancellation and
replacement append an event and a reversing ledger row. The original record is
never rewritten. Duplicate source documents and faktur numbers are prevented by
unique constraints.

AR/AP posting writes a tax-capture outbox row in the same database transaction
as the source status change. The worker retries incomplete captures every five
minutes, so an accounting integration succeeding while the immediate tax hook
fails cannot permanently omit the tax document. **Rebuild from posted sources**
remains an explicit recovery/audit control. Both paths use idempotent source
keys and never read editable browser totals.

## Reconciliation, locking, and export

The monthly recap groups each tax category by its effective mapped GL account.
The tax period cannot lock while any category differs from posted GL activity by
even one rupiah. Once locked, database triggers reject new documents,
withholding, or ledger rows for that period.

Coretax XML generation is a POST action and requires `tax.report.export`. The output stores schema
version, content SHA-256, record count, DPP, and tax total. The application checks
the reviewed schema artifact checksum and XML structure. XML declaration and
optional elements such as `Sign` are effective schema-version data, not
hard-coded assumptions; `Sign` is omitted unless that reviewed schema enables it.

The local release suite also exercises the configured Coretax HTTP contract:
reviewed XML is posted to an explicit validator path, non-2xx or non-accepted
responses fail closed, and returned record counts/totals are compared with the
immutable export before a zero-difference GL recap is locked. This deterministic
contract test does not replace the official DJP XSD/converter and Coretax
staging/import evidence below.

### Release gate

Structural validation is not proof that DJP will accept a file. For every schema
version, tax staff must validate representative invoice, return, credit/debit
note, cancellation, replacement, partial-payment, and rounding files against
the current official XSD/converter and complete a Coretax staging/portal import.
Record the reviewed official artifact and checksum only after that validation.
Production export remains blocked when no reviewed schema is active.

Tax staff must complete and retain the
[Coretax schema validation and release sign-off](tax-staff-coretax-validation.md)
before activating each schema version for production filing.

## Permissions

- `tax.view`: documents and recap.
- `tax.config.manage`: reviewed configuration and posted-source rebuild.
- `tax.period.lock`: close a reconciled tax period.
- `tax.document.correct`: cancellation and replacement events.
- `tax.report.export`: generate and download authority exports.
