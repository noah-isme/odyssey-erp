# Quality Management System (QMS)

## Current status

**Full Implementation.** The Odyssey ERP provides a robust QMS module covering major quality assurance processes. The core logic resides in `internal/qms/`.

## Supported scope

- **Non-Conformance Reports (NCR):** Tracking and disposition of non-conformances with severity, category, and source tracking.
- **Corrective and Preventive Actions (CAPA):** Lifecycle management for CAPAs including root cause analysis methods (5 Whys, Fishbone, etc.), team assignments, and effectiveness verification.
- **Audits & Findings:** Planning and execution of internal, supplier, and regulatory audits against specific standards (ISO9001, AS9100, etc.). Includes finding tracking and risk assessments.
- **Supplier Quality:** Supplier approval status, quality ratings, risk levels, and specific supplier audit tracking.
- **Quality Objectives:** Definition and tracking of KPIs (e.g., DPPM, First Pass Yield, On-Time Delivery) with measurement logging.
- **Inspections:** Execution of inspection plans recording expected vs. actual values for product characteristics.
- **Customer Complaints:** Logging, triage, and response tracking for customer issues.
- **Quality Holds:** Placing entities across the system on temporary quality hold to prevent progression until released.

## Gaps

Deep integration with automated production line test equipment (ATE), advanced Statistical Process Control (SPC) charting, and automated regulatory reporting generation are not currently supported.
