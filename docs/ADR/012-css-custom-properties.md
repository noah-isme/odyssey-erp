# ADR-012: CSS Custom Properties over Utility Framework

## Status
Accepted

## Date
2024-04-25

## Context
We need full design control to easily build and maintain our dark theme as the default experience, while avoiding framework lock-in and excessive bundle sizes.

## Decision
We will use custom CSS with design tokens mapped to CSS custom properties (variables) instead of using a utility-first CSS framework like Tailwind.

## Alternatives
- TailwindCSS
- Bootstrap
- Styled Components / CSS-in-JS

## Consequences
### Positive
- Full control over styling and themes
- Smaller CSS bundle size
- No build step required for CSS processing
### Negative
- Requires more manual styling effort upfront
- Potential for inconsistency if design tokens are not strictly adhered to
