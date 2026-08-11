# ADR-005: Gotenberg for PDF Reports

## Status
Accepted

## Date
2024-02-12

## Context
Financial reports (invoices, balance sheets) require precise PDF rendering. Generating PDFs natively in Go can be tedious, inflexible, and error-prone when dealing with complex layouts. HTML/CSS is a much better medium for designing layouts, and we already use HTML templates for the web UI.

## Decision
We will use Gotenberg, a Docker-powered stateless API for PDF generation, which converts HTML/CSS to PDF using Chromium.

## Alternatives Considered
- wkhtmltopdf: Deprecated and lacks modern CSS support.
- Native Go PDF libraries (gofpdf, maroto): Require rewriting layouts in Go code, losing HTML template reusability.
- Headless Chrome via Puppeteer (Node.js): Requires building our own microservice.

## Consequences
### Positive
- High-fidelity PDFs with modern CSS support.
- Reusable HTML/CSS templates across web and print.
- Stateless and scalable.

### Negative
- Requires a Docker sidecar container (Gotenberg) in the deployment architecture.

### Neutral
- Network call overhead for PDF generation.

## References
- [Gotenberg Documentation](https://gotenberg.dev/)
