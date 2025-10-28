# Default Account Mapping – Phase 4

This document defines the mandatory Chart of Accounts (CoA) mappings for automated postings originating from Procurement, Invent
ory, and Accounts Payable modules. The mapping keys align with `account_mappings.module` and `account_mappings.key` fields. All m
appings must reference active leaf accounts.

## Modules & Keys

### Goods Receipt Note (GRN)
| Key | Description | Typical Account Type |
| --- | ----------- | -------------------- |
| `grn.inventory` | Inventory asset receiving the goods. Used when GRN immediately recognises stock. | ASSET |
| `grn.grir` | Goods Receipt / Invoice Receipt (GRIR) clearing to bridge GRN and AP invoice. | LIABILITY |
| `grn.accrual` | Accrued AP when inventory should not hit GRIR (direct accrual). Optional fallback. | LIABILITY |

### Accounts Payable Invoice
| Key | Description | Typical Account Type |
| --- | ----------- | -------------------- |
| `ap.invoice.ap` | Trade accounts payable liability. | LIABILITY |
| `ap.invoice.inventory` | Inventory or cost-of-goods-recognised for stock purchases. | ASSET / EXPENSE |
| `ap.invoice.expense` | Operating expense (services, non-stock). Used when GRN not linked. | EXPENSE |
| `ap.invoice.tax_input` | Input VAT / purchase tax receivable. Optional based on tax config. | ASSET |

### Accounts Payable Payment
| Key | Description | Typical Account Type |
| --- | ----------- | -------------------- |
| `ap.payment.cash` | Cash or bank account from which payment is issued. | ASSET |
| `ap.payment.ap` | Accounts payable to clear vendor liability. | LIABILITY |
| `ap.payment.discount` | Early payment discount or gain on payment. Optional. | REVENUE |

### Inventory Adjustment
| Key | Description | Typical Account Type |
| --- | ----------- | -------------------- |
| `inventory.adjustment.gain` | Inventory gain (quantity positive) offset. | REVENUE |
| `inventory.adjustment.loss` | Inventory shrinkage / loss. | EXPENSE |
| `inventory.adjustment.inventory` | Inventory asset account impacted by adjustment. | ASSET |

## Configuration Rules
* Finance administrators seed mappings via `samples/coa.csv` and `make seed-phase4`.
* Each key is mandatory unless marked optional. Posting service validates presence before accepting payloads.
* Integrations must pass explicit mapping keys when constructing journal requests; fallback logic is disallowed to avoid silent m
  iscodings.
* Mapping updates trigger audit logging and require `finance.gl.edit` permission.
* Introduce environment defaults via configuration (`config/accounting.yml`) to simplify initial setup; operators can override via
  UI.

## Future Extensions
* Accounts Receivable (AR) module will add `ar.invoice.*` and `ar.receipt.*` keys following the same pattern.
* Multi-entity deployments may extend mapping keys with dimension suffixes (e.g., `ap.invoice.inventory.branch_<code>`); the repo
  sitory structure supports this through composite keys.
