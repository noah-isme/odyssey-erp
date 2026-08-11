# Odyssey ERP Design System

This is the primary pointer document for the Odyssey ERP design system. 
For the **comprehensive, full design system documentation**, including component examples, CSS architecture, and interactivity patterns, please refer to:
👉 [docs/DESIGN.md](./docs/DESIGN.md)

## Core Design Principles

The Odyssey ERP interface is built on the following foundational principles:

*   **Dark-Themed & Premium**: A modern, dark aesthetic designed for extended daily use, reducing eye strain while maintaining high contrast.
*   **SSR-First with Progressive Enhancement**: Core functionality works with standard HTML/HTTP. JavaScript (HTMX, Alpine.js) is layered on top for a snappy feel.
*   **WCAG 2.2 AA Accessibility**: Commitment to keyboard navigability, screen reader support, and adequate color contrast.
*   **Strict Separation of Concerns**: Clean boundaries between HTML, CSS, and JS. No inline styles or scripts in templates.

## Design Token Categories

Our design tokens are implemented as CSS variables to maintain consistency across:
*   Colors (Brand, Surfaces, Status, Typography)
*   Typography (Inter font, tabular numeric data)
*   Spacing (4px grid base)
*   Borders & Radii
*   Transitions & Animation
*   Shadows

## For Contributors

If you are a contributor looking for CSS guidelines, HTML template hierarchies, or detailed component structures, please read the full specification at [docs/DESIGN.md](./docs/DESIGN.md). Always consult the comprehensive docs before writing new styles.
