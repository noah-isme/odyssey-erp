# Computerized Maintenance Management System (CMMS)

## Current status

**Full Implementation.** The Horizon foundation supports a comprehensive CMMS module for managing maintenance operations. The core logic is located in `internal/cmms/`.

## Supported scope

- **Work Orders:** Full lifecycle management (Draft, Planned, Scheduled, In Progress, On Hold, Completed, Cancelled, Closed). Includes prioritization, categorization, assignment, and tracking of estimated vs. actual hours.
- **Assets:** Hierarchical asset management (parent-child relationships) with tracking of asset types, specifications, warranties, manufacturers, and criticality levels.
- **Locations:** Hierarchical physical locations with GPS coordinates.
- **Preventive Maintenance (PM):** Scheduling based on calendar frequency (daily, weekly, monthly, etc.) or meter readings (hours, cycles, distance).
- **Task Templates:** Reusable task templates with step-by-step instructions and safety notes for standardized maintenance procedures.
- **Spare Parts:** Inventory tracking for maintenance spares, including reorder points, min/max quantities, and usage tracking on work orders.
- **Meter Readings:** Tracking of usage or environmental metrics (e.g., hours, temperature, pressure) against assets.

## Gaps

Advanced predictive maintenance algorithms, IoT sensor integrations for real-time telemetry, detailed project-based cost accounting, and a dedicated mobile offline-first execution app for field technicians are not currently supported out-of-the-box.
