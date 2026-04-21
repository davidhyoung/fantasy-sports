---
name: stats-lookup
description: Consult the statistical techniques library at docs/stats/ when working on ranking, projection, grading, or any code that turns raw stats into comparative player value. Trigger when editing internal/handlers/analysis.go, internal/handlers/grades.go, or anything under cmd/projections/ or cmd/backtest/; or when the user mentions small-sample bias, regression to the mean, confidence intervals, uncertainty, opponent adjustment, strength of schedule, shrinkage, priors, or "why does this player's z-score look wrong." Also trigger when proposing algorithmic changes to rankings or projections — before writing code, check whether the library already names a technique for the problem.
---

# stats-lookup

## What this skill is for

The project has a growing reference library of statistical techniques at `docs/stats/`, grounded in real gaps in the ranking and projection code. This skill makes sure we consult it — and extend it — instead of reinventing approaches each conversation.

## What to do when triggered

1. **Read `docs/stats/README.md` first.** It has an index table mapping problem → entry. Skim it before proposing any algorithmic change to rankings, projections, or grades.
2. **Open the relevant entry.** Each entry has a fixed shape (problem, technique, assumptions, where it applies in this codebase, worked example, validation plan, tradeoffs, references). Use that structure to frame the conversation with the user.
3. **Cite the entry by path** when recommending an approach — e.g. "per `docs/stats/bayesian-shrinkage.md`, the right fix here is to shrink YPC toward the position-group mean before z-scoring." This keeps reasoning traceable across sessions.
4. **If the library is silent on the problem, extend it.** Before writing code that introduces a new technique (Kalman filter, mixed-effects model, elo-style iterative rating, Dirichlet-multinomial, etc.), draft a new entry using `docs/stats/_template.md`. Then add it to the index table in `docs/stats/README.md`.
5. **Match technique to scope.** Not every small improvement needs a new library entry — only genuinely new techniques do. A tweak to an already-documented technique belongs in the existing entry (update the worked example, add a tradeoff).

## When to write an entry vs. just edit code

Write/update an entry when:
- You're introducing a named statistical technique the codebase hasn't used before.
- You're materially changing the assumptions behind an existing technique (e.g. moving from empirical-Bayes shrinkage to hierarchical Bayes).
- You make a non-obvious judgment call (e.g. "we chose `k=80` because…") that future-you will want to justify.

Don't write an entry for:
- Bug fixes, refactors, or mechanical changes.
- Tuning an existing parameter inside ranges the entry already discusses.
- One-off scripts or exploratory notebooks.

## Output shape

When the skill fires and you find a relevant entry, give the user:
- The entry name and path.
- A one-sentence summary of what it prescribes for their situation.
- The specific file(s) in this repo it would affect.
- The validation plan from the entry (so changes don't ship without a backtest or sanity check).

If no entry matches, say so explicitly, propose writing one, and offer a draft.

## Files this skill touches

- `docs/stats/README.md` — index (update when adding entries)
- `docs/stats/_template.md` — structure for new entries (do not modify casually)
- `docs/stats/<technique>.md` — individual entries
- Downstream code: `internal/handlers/analysis.go`, `internal/handlers/grades.go`, `cmd/projections/*`, `internal/handlers/projections.go`, `internal/handlers/draft_values.go`
