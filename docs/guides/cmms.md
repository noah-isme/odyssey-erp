# Computerized Maintenance Management System (CMMS)

## Current status

**Core implementation with advanced telemetry foundations.** The Horizon foundation
supports maintenance operations in `internal/cmms/`. IoT readings, predictive-model
metadata, and idempotent anomaly alerts are now persisted and exposed through the
service boundary; these are deterministic foundations, not a replacement for a
calibrated ML platform.

## Supported scope

- **Work Orders:** Full lifecycle management (Draft, Planned, Scheduled, In Progress, On Hold, Completed, Cancelled, Closed). Includes prioritization, categorization, assignment, and tracking of estimated vs. actual hours.
- **Assets:** Hierarchical asset management (parent-child relationships) with tracking of asset types, specifications, warranties, manufacturers, and criticality levels.
- **Locations:** Hierarchical physical locations with GPS coordinates.
- **Preventive Maintenance (PM):** Scheduling based on calendar frequency (daily, weekly, monthly, etc.) or meter readings (hours, cycles, distance).
- **Task Templates:** Reusable task templates with step-by-step instructions and safety notes for standardized maintenance procedures.
- **Spare Parts:** Inventory tracking for maintenance spares, including reorder points, min/max quantities, and usage tracking on work orders.
- **Meter Readings:** Tracking of usage or environmental metrics (e.g., hours, temperature, pressure) against assets.
- **IoT Readings:** Company-checked sensor readings are persisted with their event
  timestamp and update the sensor's latest state.
- **Predictive Models and Alerts:** Active model metadata is stored per company. The
  scheduled/batch evaluator selects the latest reading and creates at most one open
  critical alert per sensor/model anomaly, so retries are safe.

## Gaps

Advanced predictive inference, model training, configurable per-sensor thresholds,
real-time streaming dashboards, detailed project-based cost accounting, and a
dedicated mobile offline-first execution app for field technicians are not currently
supported out-of-the-box. The current evaluator is a deliberately explicit heuristic
(`reading > 1000`) until a real model/rules provider is configured.
