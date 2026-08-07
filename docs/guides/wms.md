# Warehouse Management System (WMS)

## Current status

**Implemented.** The Horizon foundation includes a functional WMS module for barcode-driven operations. The logic is found in `internal/wms/`.

## Supported scope

- **Bins & Locations:** Management of physical storage bins within warehouses, including capacity limits.
- **Pick Tasks:** Wave-based picking tasks associated with specific products and requested quantities. Supports workflow transitions (Open -> Picking -> Picked -> Packed -> Shipped).
- **Barcode Scanning:** Idempotent barcode scanning operations connecting product/bin barcodes to active tasks, ensuring precise and duplicate-proof inventory movements.

## Gaps

Advanced wave planning logic, intelligent put-away strategies, automated cross-docking, native integration with Material Handling Equipment (MHE)/conveyors, and 3D warehouse capacity visualization are not currently implemented. Additional operations like directed cycle counting may be expanded in future phases.
