# ADR-0009: Asset capitalization and operations

**Status:** Proposed — requires controller and asset-custodian approval

## Decision

`internal/fixedassets/` remains the owner of active assets, depreciation, and disposal.
An eligible PO/AP/receipt creates a capitalization candidate, not an active asset. An
authorized asset acceptance confirms the source lineage, exact allocated cost, quantity,
unique tag, in-service date, category, location, custodian, useful life, and warranty
before it calls the existing asset activation path.

```text
Capitalizable PO/AP/receipt -> candidate -> accepted -> active asset
      -> location/custody -> transfer | warranty | maintenance -> disposal
```

Financial status and operational condition are independent. Transfers update
company-scoped location/custody history and require dispatch/receipt confirmation;
they do not post a journal unless a separately approved accounting policy says so.
Maintenance is expensed by default. A later cost may be capitalized only through an
explicit, auditable adjustment policy, never by editing depreciation state.

## Consequences

- Asset location is an effective-dated hierarchy with no cross-company movement.
- Warranties, claims, preventive plans, and corrective work orders remain available on
  fully depreciated assets but not on disposed assets.
- History remains readable after master-data changes and links PO, GRN, AP invoice,
  journal, transfer, maintenance, warranty, depreciation, and disposal records.
- Multi-quantity purchases allocate total cost with exact decimals and create one unique
  asset identity per accepted unit.
