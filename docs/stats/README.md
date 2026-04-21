# Sports Stats — Statistical Techniques Library

A growing reference of statistical methods relevant to fantasy sports ranking and projection. Each entry is grounded in a real gap or decision point in this codebase (`internal/handlers/analysis.go`, `cmd/projections/`, `internal/handlers/grades.go`).

This library is **not** a one-shot rewrite of the algorithms. It is a catalog we consult when improving them — so we can cite a technique, name its assumptions, and reuse the same vocabulary across conversations and commits.

## When to reach for each entry

| Problem you're facing | Entry |
|---|---|
| Small samples giving extreme z-scores; a player with 2 strong games ranked like a star | [bayesian-shrinkage.md](bayesian-shrinkage.md) |
| Projection gives a point estimate but no sense of how wide the plausible range is | [uncertainty-quantification.md](uncertainty-quantification.md) |
| Player ranked highly but their stats came against weak defenses | [strength-of-schedule.md](strength-of-schedule.md) |

## Adding a new entry

1. Copy `_template.md` to `<technique-name>.md`.
2. Fill every section — especially **When it applies in this codebase** (with file paths) and **Worked example**.
3. Add a row to the table above so it's discoverable.
4. If you cite a paper or external resource, link it at the bottom.

## Guiding principles

- **Every entry names a real gap.** If we can't point to a file and say "this is where the technique would help," the entry isn't ready.
- **Formulas beat hand-waving.** Write the math. Ambiguous prose ages badly.
- **State the assumptions.** Every method has them — shrinkage assumes a plausible prior, SOS assumes opponent strength is estimable, etc. Make them explicit so edge cases are obvious later.
- **Prefer incremental adoption.** New techniques should be introduced behind a flag or alongside the existing approach, then compared on backtests before replacing anything.
