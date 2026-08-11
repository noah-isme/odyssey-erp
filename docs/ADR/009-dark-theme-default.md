# ADR-009: Dark Theme as Default

## Status
Accepted

## Date
2024-03-22

## Context
The ERP system will be used extensively by users throughout their workday. Long hours interacting with the software require considerations for user comfort. Additionally, we want to establish a modern, premium brand aesthetic.

## Decision
We will implement a Dark Theme as the default and primary theme for the Odyssey ERP application.

## Alternatives Considered
- Light Theme Default: Traditional approach, but can cause more eye strain during extended use.
- System Preference Default: Respects OS settings, but requires implementing and maintaining both themes from day one, which increases initial development scope.

## Consequences
### Positive
- Reduces eye strain for extended use.
- Modern aesthetic and premium feel.
- Consistent branding out of the box.

### Negative
- Some users prefer light mode for readability in bright environments.

### Neutral
- Initially, there will be no light mode option to reduce design and CSS maintenance overhead during the MVP phase.

## References
- [Material Design: Dark Theme](https://m3.material.io/styles/color/system/dark-theme)
