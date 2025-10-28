# Testing – Phase 3 Inventory & Procurement

## Overview
Automated tests focus on inventory average moving cost logic and procurement lifecycle. Manual checks cover PDF output and RBAC enforcement.

## Automated Tests
* `go test ./internal/inventory` – table-driven test `TestAverageMovingCost` validates sequential inbound/outbound adjustments.
* `go test ./internal/procurement` – `TestProcurementFlow` covers PR→PO→GRN→AP Invoice→Payment integration and inventory hook stub.
* `go test ./...` – aggregate run ensures all packages compile with new dependencies.

## Manual Checks
1. **Schema Migration**
   - Run `make migrate-up` against a local PostgreSQL instance.
   - Verify new tables (`inventory_tx`, `pos`, `grns`, `ap_invoices`, dll.) terbentuk.

2. **Inventory UI**
   - Login sebagai admin, akses `/inventory/stock-card` dan pastikan render sukses.
   - Posting adjustment via `/inventory/adjustments` dan cek audit log.

3. **Procurement Flow**
   - Buat PR/PO/GRN via form SSR.
   - Posting GRN harus menambah stok (cek `inventory_balances`).
   - Buat invoice dan pembayaran untuk memastikan status berubah ke `POSTED`/`PAID`.

4. **PDF Reports**
   - Hit `/report/stock-card/pdf?warehouse_id=1&product_id=1` dan `/report/grn/pdf?number=GRN-001`.
   - Konfirmasi header `Content-Type: application/pdf` dan file > 1KB.

5. **Jobs**
   - Enqueue manual job `TaskInventoryRevaluation` dan `TaskProcurementReindex` menggunakan asynq CLI untuk memastikan handler tidak error.

6. **RBAC**
   - Ubah role user ke `viewer` dan coba akses endpoint POST; harus 403.

## Data Seed
Gunakan `make seed-phase3` untuk mengisi permission & role terbaru, termasuk permission inventory/procurement/finance.
