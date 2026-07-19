# Inventory traceability and replenishment

## Product setup

Each product has an inventory policy configured in **Master Data → Products**:

* **Cost method:** weighted average (`AVG`) or FIFO. LIFO is deliberately not
  offered because it is not the selected accounting policy for this application.
* **Minimum stock** and **reorder target:** determine when an item is shown in
  the reorder alert and how much is requested.
* **Preferred supplier:** required for automatic replenishment.
* **Traceability:** choose either batch/lot tracking or serial-number tracking;
  a product cannot use both modes.

## Receiving stock

Create a Goods Receipt (GRN) as usual. For a batch-tracked product, enter a lot
number and (when applicable) an expiry date. For a serial-tracked product,
enter one comma-separated serial number per received unit. Posting validates
these requirements and writes the lot/serial record together with the inbound
inventory movement.

Duplicate serial numbers are rejected by the database. A failed posting leaves
the GRN unposted; correct the receipt data and retry it.

## Replenishment workflow

The Inventory dashboard lists balances below the minimum stock. Select
**Create reorder requests** to create draft Purchase Requests grouped by the
configured preferred supplier. Quantity is `reorder target − current stock`
(or `minimum stock − current stock` if no higher target is configured).

The action never issues a Purchase Order directly. Draft PRs continue through
the normal review and approval flow before a PO is created, preserving the
procurement control point.

## Reporting dimensions

Departments and cost centers are available as optional dimensions on new
journal lines. Existing entries remain valid. The current migration establishes
the hierarchy and journal storage; department/cost-center filters, Excel
exports, schedules, report builder, and dashboard widgets are follow-up work.

## Deployment

Apply migrations before enabling these workflows:

```bash
make migrate-up
```

Run the focused verification suite:

```bash
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 \
  go test ./internal/inventory ./internal/procurement ./internal/accounting/journals ./internal/accounting/reports
```
