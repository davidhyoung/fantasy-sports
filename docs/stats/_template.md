# <Technique Name>

## Problem it solves

One or two sentences. What naive approach fails, and how?

## Technique

The core idea in plain English. Then the formula:

```
<formula or pseudocode>
```

Define every symbol. If there are tuning parameters, name them and give a sane default.

## Assumptions

Bullet list. What must be true for this technique to be valid? (e.g. "observations are roughly i.i.d.", "prior is approximately correct", "sample size across units is comparable").

## When it applies in this codebase

- `path/to/file.go:func` — what gap it addresses and how it would plug in.
- Be specific. If there are multiple call sites, list them.

## Worked example

A small numeric example using real-ish values from this project (an NFL player, NBA category, etc.). Show the before/after difference.

## How to validate it's working

How would we know this technique is actually improving things? Point at a backtest, a metric in `nfl_backtest_results`, a visible UI effect, or a unit test.

## Tradeoffs

- What does this technique cost us? (complexity, opacity, compute, tuning burden)
- What does it break or obscure?

## References

- Paper / blog / chapter links. Include author and year.
