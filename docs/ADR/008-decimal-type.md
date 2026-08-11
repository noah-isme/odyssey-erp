# ADR-008: Decimal Type for Financial Data

## Status
Accepted

## Date
2024-03-10

## Context
Financial applications require strict accuracy for monetary values. Using standard IEEE 754 floating-point numbers (`float64`) introduces precision errors (e.g., `0.1 + 0.2 = 0.30000000000000004`), which is unacceptable for accounting and financial reporting.

## Decision
We will use the `shopspring/decimal` package for all monetary amounts in Go, mapping to the `DECIMAL`/`NUMERIC` type in PostgreSQL.

## Alternatives Considered
- Integer Cents (representing $10.00 as 1000): Avoids floating-point issues but complicates logic when dealing with fractions of a cent (e.g., interest rates, tax calculations) or multi-currency scenarios.
- Standard `float64`: Strongly rejected due to rounding errors.

## Consequences
### Positive
- Exact arithmetic operations.
- Correct financial totals and rounding behavior.
- Direct mapping to PostgreSQL `NUMERIC` type.

### Negative
- More verbose code for arithmetic operations (e.g., `a.Add(b)` instead of `a + b`).
- Slight performance overhead compared to primitive types (negligible for our use case).

### Neutral
- Requires developers to be mindful of using the library for all financial calculations.

## References
- [shopspring/decimal GitHub](https://github.com/shopspring/decimal)
- [PostgreSQL Numeric Types](https://www.postgresql.org/docs/current/datatype-numeric.html)
