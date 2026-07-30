# Tax Staff Note: Coretax Schema Validation and Release Sign-off

**Status:** Mandatory release gate  
**Owner:** Company tax staff / authorized tax consultant  
**Technical support:** Odyssey administrator or engineering  
**Last reviewed:** 2026-07-30

Do not activate a `tax_export_schemas` version for production filing until this
checklist is complete. Passing Odyssey's XML and checksum checks does not prove
that DJP Coretax will accept the file.

## Official references

Always obtain a fresh copy directly from DJP on the validation date:

- [DJP Coretax XML templates and converters](https://www.pajak.go.id/id/reformdjp/coretax/template-xml-dan-converter-excel-ke-xml)
- [PMK 11/2025: DPP Nilai Lain and specified PPN amounts](https://www.pajak.go.id/id/peraturan/ketentuan-nilai-lain-sebagai-dasar-pengenaan-pajak-dan-besaran-tertentu-pajak-pertambahan)

The DJP template page warns that its information can change. It identifies XML
as the Coretax import format and validates NPWP, NITKU, tax-object codes, rates,
and document-specific elements. Therefore, do not rely on an artifact retained
from an earlier release without comparing it to the current DJP download.

## 1. Record the official artifact

Complete this record before testing:

| Evidence | Value |
|---|---|
| Validation date and Jakarta time | |
| DJP page URL | |
| Document category | Faktur Keluaran / Retur / SPT Masa PPN / Bupot Unifikasi |
| Official filename | |
| Version or page update date | |
| Downloaded file SHA-256 | |
| Odyssey schema version code | |
| Reviewer name and user ID | |

Store the untouched official download, SHA-256 output, screenshots or logs, and
the reviewed Odyssey field mapping together in the release evidence folder.

## 2. Review tax configuration

- [ ] The company legal name, NPWP, NITKU, PKP identity, and registered address
      match the active Coretax taxpayer profile.
- [ ] Customer and supplier tax identifiers use the required length and format.
- [ ] Every exported tax code maps to the current official reference code.
- [ ] Every rate follows the selected tax-object code.
- [ ] Effective dates cover the filing period and do not overlap another
      reviewed version.
- [ ] The faktur range belongs to the company and contains no duplicate or
      previously consumed number.
- [ ] VAT output, VAT input, PPh 23, and PPh 4(2) map to the approved GL accounts.
- [ ] The stored schema-body SHA-256 exactly matches the reviewed artifact.
- [ ] The stored XML declaration (version, encoding, and standalone flag) matches
      the official template.
- [ ] Optional exported fields, including Odyssey's `Sign` field, are enabled
      only when the official schema explicitly permits them.

For PPN, separately confirm the statutory rate and DPP formula. PMK 11/2025
contains transaction-specific DPP Nilai Lain rules, including 11/12 factors for
specified transactions. Do not represent every transaction with one global
effective rate.

## 3. Validate representative cases

Use anonymized but realistic records for every case the company will file:

- [ ] Standard output invoice.
- [ ] Input invoice.
- [ ] Multiple invoice lines and tax codes.
- [ ] Non-luxury and luxury treatment when applicable.
- [ ] DPP Nilai Lain transaction when applicable.
- [ ] Sales return and AR credit note.
- [ ] Purchase return and AP debit note.
- [ ] Cancellation.
- [ ] Replacement/correction linked to the original document.
- [ ] Partial AP payment with PPh 23 or PPh 4(2) withholding.
- [ ] Values producing a half-rupiah or other rounding boundary.
- [ ] Month-end totals with at least one negative correction.

Unsupported tax treatments must not be approximated. Record them as release
blockers and add an effective-dated rule/schema implementation first.

## 4. Run the validation sequence

1. Rebuild the selected period from posted sources in `/tax`.
2. Confirm no editable UI total is used as a tax-ledger source.
3. Review all missing NPWP/NITKU, code, numbering, and rate errors.
4. Generate the Coretax XML using a non-production test period.
5. Validate it with the current official DJP template/converter or XSD.
6. Import the file into the available Coretax test/staging workflow. If DJP does
   not provide a separate test environment, use the approved company procedure
   that does not create an unintended filing.
7. Save the validation output and Coretax acceptance/rejection evidence.
8. Correct all warnings that can affect tax identity, classification, amount,
   numbering, or filing status; regenerate and repeat from step 4.

Export is intentionally submitted with POST because it persists an immutable
export audit record. Do not replace it with a bookmarkable GET endpoint.

## 5. Reconcile to the rupiah

For each category, compare all three values:

| Category | Export total | Tax ledger | Mapped GL | Difference |
|---|---:|---:|---:|---:|
| VAT output | | | | |
| VAT input | | | | |
| PPh 23 | | | | |
| PPh 4(2) | | | | |

The required difference is **Rp0 for every category**. Also confirm:

- [ ] Export record count equals the accepted Coretax record count.
- [ ] Credit/debit notes and correction reversals have the correct sign.
- [ ] Cancelled/replaced originals are not reported as active documents.
- [ ] Export SHA-256 and persisted totals match the file submitted for review.
- [ ] The period remained unlocked during corrections and was locked only after
      reconciliation and acceptance.

## 6. Sign-off

| Approval | Name | Date/time | Result / reference |
|---|---|---|---|
| Tax preparer | | | |
| Tax reviewer / consultant | | | |
| Finance/GL owner | | | |
| System administrator | | | |

Activate the reviewed schema only when all required reviewers approve it. Record
the official URL, checksum, reviewer, review timestamp, test export hash, and
Coretax evidence reference in the change ticket or release record.

## Stop conditions

Do not activate, lock, or file when any of these is true:

- The DJP artifact changed after validation.
- The official checksum or Odyssey schema checksum differs.
- Coretax rejects a record or produces unexplained warnings.
- NPWP, NITKU, object code, tax rate, or numbering is uncertain.
- Export, tax-ledger, and GL totals differ by Rp1 or more.
- A required business case was not tested.

Create a new schema/rule version for corrections. Never overwrite a reviewed
effective-dated version or an immutable tax document.
