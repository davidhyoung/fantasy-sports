# Consensus Sources & Calculations — Reference Catalog

> **Status: reference doc (August 2026).** This is the single place that names *which*
> external sources feed our consensus numbers and *exactly* how the two consensus
> calculations work. The reasoning for why we build it this way — median vs. mean,
> within-position ranking, format-matching — lives in
> [consensus-ensemble.md](consensus-ensemble.md) (the divergence layer) and
> [auction-values.md](auction-values.md) (the dollar layer). This doc doesn't repeat
> that reasoning; it's the lookup table + the formulas, kept current as sources are
> added or dropped.

## What "consensus" means here

Every external ranking site disagrees with every other one, and none of them is "the
market" by itself. We don't pick a favorite source — we pull rank/ADP/value from
several, resolve each row to a player (`gsis_id`), and take the **median** within
**position group** (`docs/stats/consensus-ensemble.md`). That single number feeds two
different consumers:

1. **Divergence** (`nfl_projection_divergences`) — our projection rank vs. the
   consensus rank, surfaced on `/divergences` and Home's Player Outlooks.
2. **Draft dollars** (`draft_consensus.go`) — the consensus rank read against *our own*
   auction-value curve, surfaced as `Cons $` / `Edge` on `/draft-prep`.

Both read the same underlying table (`nfl_consensus_rankings`), so adding a source
here improves both automatically — no separate wiring per consumer.

## Source catalog

Two snapshot files exist under `backend/cmd/projections/testdata/consensus/`.
Ingestion is manual/file-based — see "Why hand-curated" below — via
`make project-nfl ARGS="-import-consensus f.json -season 2026"`.

**`rankings_2026-08-04.json` — imported, live in `nfl_consensus_rankings` today:**

| `source` key | Outlet | Publishes | `metric_type` | `format` | Rows in snapshot | Notes |
|---|---|---|---|---|---|---|
| `espn` | ESPN Fantasy | Expert consensus rank | `rank` | `ppr` | 63 | Overall + positional expert rankings |
| `si_fabiano` | Sports Illustrated (Michael Fabiano) | Expert rank | `rank` | `ppr` | 63 | Named-columnist rankings, not SI's aggregate |
| `si_onsi` | Sports Illustrated / On3 (OnSI network) | Expert rank | `rank` | `ppr` | 59 | Distinct SI vertical from `si_fabiano` — kept separate since they're independently written |
| `bleacher_report` | Bleacher Report | Expert rank | `rank` | `ppr` | 59 | |
| `fantasypros_adp` | FantasyPros | Aggregated ADP | `adp` | `ppr` | 61 | FantasyPros' *own* ADP aggregate (they already average many draft sites); we treat their output as one source, not a shortcut to many |
| `sleeper_adp` | Sleeper | Live draft ADP | `adp` | `ppr` | 60 | Pulled from actual app drafts, not an expert opinion |
| `espn_adp` | ESPN | Live draft ADP | `adp` | `ppr` | 60 | |
| `underdog_adp` | Underdog Fantasy | Live draft ADP | `adp` | `half_ppr` | 60 | Underdog's default format is half-PPR, not PPR — the only non-PPR source we have |
| `keeptradecut` | KeepTradeCut | Crowdsourced dynasty trade value | `dynasty_value` | `dynasty` | 30 | Not diffed against our projections (dynasty has no comparable model output — see below); kept for future dynasty features |

**Coverage is intentionally shallow.** Every source publishes roughly their top ~60
per list (top-100 overall, thinning fast by position: QB 8 deep, TE 6, RB/WR ~24
each — measured in `docs/algorithm-review.md` §7.1). "Not in the list" means the
source didn't rank that far, not that they'd rank the player last.

**8 of 9 sources are PPR-only; `underdog_adp` is the sole half-PPR data point; there
is no `standard`-format source at all.** A `standard`-format divergence or consensus
$ column today will always come back empty (`—`), regardless of the league's actual
format.

**Dynasty is imported but not diffed.** `projectionRankColumn()`
(`cmd/projections/consensus.go`) explicitly rejects `dynasty`/`superflex` for the
divergence computation — our engine produces a one-season redraft projection with no
multi-year or QB-premium output to compare a dynasty/superflex rank against. The
`keeptradecut` rows exist for a future dynasty feature, not today's divergence table.
The same restriction will apply to `4for4_superflex_adp` below once it's imported —
it's real data, but there's still no model-native superflex rank to diff it against.

**`rankings_2026-08-14.json` — captured, ready to import, not yet run through
`-import-consensus`:** built from the web-research pass below, specifically to close
the `standard`-format and `superflex` gaps the 2026-08-04 snapshot left open.

| `source` key | Outlet | Publishes | `metric_type` | `format` | Rows | Notes |
|---|---|---|---|---|---|---|
| `cbs_standard` | CBS Sports | Expert overall rank, top 200 | `rank` | `standard` | 100 | **Closes our only `standard` gap** |
| `cbs_ppr` | CBS Sports | Expert overall rank, top 200 | `rank` | `ppr` | 60 | Tenth source for PPR corroboration depth |
| `ffc_adp` | FantasyFootballCalculator | Crowd ADP from mock drafts (12-team) | `adp` | `standard` | 60 | `value` is the site's own decimal ADP (e.g. `2.3`) |
| `4for4_superflex_adp` | 4for4 | Pick-format ADP | `adp` | `superflex` | 50 | **Closes our only `superflex` gap.** `value` is overall pick number, converted from the page's `round.pick` display (`(round-1)×10 + pick`, confirmed 10-team format) — not diffable yet per the dynasty/superflex restriction above, but usable once the engine produces a superflex-comparable output |

**Data-quality note on this snapshot:** CBS Sports' scraped `team` field was
unreliable for recently-traded players and rookies (returned stale pre-trade teams,
or a college name for a couple of rookies). Corrected by cross-referencing three
independently-fetched pages (FantasyFootballCalculator, 4for4, and an NBC Sports page
that was otherwise excluded — see below) that agreed with each other everywhere they
overlapped, e.g. all three independently placed Derrick Henry on BAL, Kenneth Walker
III on KC, George Pickens on DAL, Ashton Jeanty on LV. Rank/name/position were
consistent across repeated CBS fetches and unaffected. Even an imperfect `team` value
degrades gracefully rather than failing to resolve — `resolve()` already falls back
to name+position when name+team doesn't match (see "Player identity resolution"
below).

**Sources evaluated for this snapshot and excluded, not just skipped:**
- **RotoWire ADP (ppr)** — rank order was visible but the numeric ADP cells were
  paywalled (`—` / "premium feature") or the page errored (HTTP 522). No genuine
  number existed to record, so nothing was written rather than approximating one.
- **NBC Sports/Rotoworld top 200** — checked twice for a stated scoring format
  (the rankings article and its linked Draft Kit page); neither confirmed whether the
  overall list itself is standard, half, or PPR. Skipped entirely rather than guess —
  it was still useful as one of the three cross-validation sources for the CBS team
  fix above, which doesn't require knowing its scoring format.

## Calculation 1 — Rank divergence

`computeConsensusDivergences` (`cmd/projections/consensus.go`), run via
`-consensus-diff -season N -format ppr|half_ppr|standard`:

```
our_rank(p)        = 1-based rank of p's proj_fpts[_ppr|_half] within p's position group
source_rank(p, s)  = RANK() OVER (PARTITION BY source, position ORDER BY value ASC)
                      -- recovers a position-relative rank from source s's list,
                      -- whether s published an overall or positional ranking
consensus_rank(p)  = median( source_rank(p, s) for every source s that ranked p )
rank_delta(p)      = our_rank(p) − consensus_rank(p)
source_count(p)    = count of sources that ranked p
```

Positive `rank_delta` = we rank the player worse than the market; negative = better.
Written to `nfl_projection_divergences`, one row per `(gsis_id, season, format)`,
re-upserted (not appended) each run.

## Calculation 2 — Consensus auction value

`loadConsensusValues` (`internal/handlers/draft_consensus.go`), called from
`GetDraftValues` for every `/draft-prep` and Draft-tab request:

```
# Preferred: an imported real market price, if one exists for this season+format
imported_price(p) = median(value for rows where metric_type='auction')
                       × (teams × budget) / (12 × 200)     -- rescale from the
                                                             -- standard market pool
                                                             -- these prices assume

# Fallback: derive a price from the consensus rank, read against OUR OWN value curve
consensus_rank(p) = median( RANK() OVER (source, position ORDER BY value ASC) )
                      -- same formula as Calculation 1, computed independently
                      -- here (this handler doesn't read nfl_projection_divergences)
cons_auction(p)    = our_auction_curve[ position(p) ][ round(consensus_rank(p)) ]
                      -- i.e. "what does OUR board pay for the player at the rank
                      -- slot the market assigns him"

edge(p) = auction(p) − cons_auction(p)
```

No source currently publishes `metric_type='auction'` rows, so every consensus dollar
value in the live app today is the derived (fallback) form — `Derived: true` on every
`consensusValue`. The imported-price branch is live code, just unfed.

`source_count(p) == 1` sets the single-source flag (`*` in the UI) on both the
divergence table and the draft board — a lone source is treated as uncorroborated,
never as a confident signal.

## Player identity resolution

`loadPlayerIndex` / `resolve()` (`cmd/projections/consensus.go`), tried in order,
first match wins:

1. `sleeper_id` exact match
2. `espn_id` exact match
3. normalized `name + team` (`normalizePlayerName`: lowercase, strip punctuation and
   suffixes `Jr./Sr./II/III/IV/V`, collapse whitespace)
4. normalized `name + position` (catches players traded since the source's data was
   captured, so team no longer matches)

Unmatched rows are kept with `gsis_id = NULL`, not dropped, specifically so they can
be audited (`SELECT * FROM nfl_consensus_rankings WHERE gsis_id IS NULL`) and the
resolver improved later rather than silently losing data.

## Why hand-curated, not scraped

Several major ranking sites render their tables client-side or actively block
scraping (checked directly, not assumed): FantasyPros' true consensus-rank page,
Yahoo's consensus article, and PFF's rankings all failed a straightforward fetch —
FantasyPros and Yahoo are JS-rendered with no data in the raw page, PFF sits behind
a paywall. **CBS is a correction to earlier research**: an initial attempt in the
original consensus-ensemble.md writeup found CBS unfetchable too, but the
2026-08-14 pass fetched its `top200` rankings pages (standard and PPR) cleanly —
either CBS changed how that page renders, or the earlier attempt hit a different,
blocked page on the same site (e.g. a consensus/aggregator tool rather than the
plain rankings list). Either way, CBS is now a working source (see the catalog
above) and this note is kept to flag that "blocked" isn't necessarily permanent —
worth re-checking previously-blocked sources periodically, not just once.
There is no reliable automated refresh path for the sources that remain blocked —
snapshots go stale between manual `-import-consensus` runs, and the `captured_date`
on every row exists precisely so a stale snapshot is visible rather than silently
trusted.

## Candidate sources still open (not fetchable as of 2026-08-14)

Researched 2026-08-14 looking specifically for sources that would close the known
gaps above. Four leads converted into the actual `rankings_2026-08-14.json` snapshot
above (CBS standard/ppr, FantasyFootballCalculator, 4for4 superflex). Two more —
RotoWire ADP and NBC Sports top-200 — looked promising by reputation but didn't
survive an actual fetch attempt (paywalled ADP cells; no confirmable scoring format,
respectively — see the exclusion note above). What's left below is genuinely still
just leads, not data:

> **Method note.** Every claim below comes from an actual `WebFetch` during this
> research pass, not from training-data recall — several outlets that looked
> promising by name (Establish The Run, NFL.com, RotoBaller) turned out to be dead
> ends the moment someone tried to fetch them, which is exactly the failure mode this
> note exists to catch before a row lands in the database.

### Confirmed real and current, but not fetchable today

These are exactly the outlets that would close our remaining gaps (a true
FantasyPros-style ECR, a second expert-authored dynasty list) — genuinely worth
revisiting if a curator can copy the table by hand or fetch through something that
executes JS, but a plain automated fetch could not read them:

| Outlet | URL | Why it matters | What blocked it |
|---|---|---|---|
| **PFF** | `pff.com/news/…standard-top-200`, `…superflex-top-200`, `…dynasty-top-200`, `…ppr-rankings-for-drafts` | Would single-handedly cover standard + superflex + a second expert dynasty list from a top-tier outlet | PFF+ paywall; only 2–4 name-drops surfaced per article, not the table |
| **FantasyPros — true ECR** (not their ADP, which we already have) | `fantasypros.com/nfl/rankings/consensus-cheatsheets.php`, `consensus-superflex-cheatsheets.php`, `dynasty-overall.php` | This is the specific gap flagged above ("no true FantasyPros expert consensus rank") | Table is JS-rendered; fetch returned page chrome only, no rows |
| **Yahoo Fantasy consensus** | `sports.yahoo.com/fantasy/article/2026-fantasy-football-full-ppr-rankings-consensus-top-300-players` | A real second "true consensus" methodology — 6-analyst panel (Boone, Harmon, Norris, Pianowski, Smyth, Winks), published Aug 10 2026 | Table dynamically loaded; fetch returned nav/metadata only |
| **Draft Sharks** | `draftsharks.com/adp`, `/auction-values`, `/dynasty-rankings` | Multi-format outlet | Fully client-rendered tool, no static table in page source |
| **Dynasty League Football (DLF)** | `dynastyleaguefootball.com/rankings/dynasty-rankings/` | A second, expert-authored (not crowdsourced) dynasty source — our only dynasty source today (KeepTradeCut) is crowdsourced trade value, not an expert list | HTTP 403 — blocks bot fetches |
| **Dynasty Nerds** | `dynastynerds.com/dynasty-rankings/` | 4-ranker consensus dynasty, 300+ players | HTTP 403 |
| **Footballguys** | `footballguys.com/salary-cap-auction-values` | Would be our first real `metric_type='auction'` rows — imported prices take precedence over our derived consensus value (`auction-values.md`) | Free tier stops at 15 rows; 16+ gated behind paid "ELITE" tier — too shallow to be useful alone |
| **RotoWire — Auction Values** | `rotowire.com/football/auction-values.php` | Same as above: real `$200`-pool auction prices | ~9 clean rows before hitting "data unavailable" cells — partial dynamic/paywall gap, same shallowness problem |

### Dead ends, checked once and abandoned

- **RotoWire ADP (ppr)** — rank order was visible but every numeric ADP cell was
  either paywalled (`—` / "premium feature") or the page returned HTTP 522. Looked
  like the strongest tenth-source candidate for PPR corroboration depth; turned out
  to have nothing genuine to record.
- **NBC Sports/Rotoworld top 200** — checked twice for a stated scoring format
  (the rankings article and its linked Draft Kit page); neither confirmed whether the
  overall list is standard, half, or PPR. Still useful as a cross-validation source
  for the CBS team-field fix (see the data-quality note above), just not importable
  as a ranking in its own right.
- **MyFantasyLeague ADP API** — a real endpoint exists, but the extracted sample
  mixed linebackers into a skill-position ADP list, which isn't plausible. Not
  trustworthy enough to report as verified; would need a direct, non-summarized fetch.
- **Establish The Run** — real and reputable, but its rankings sit behind a flat
  $54.99 paywall with no free tier.
- **NFL.com** — no standalone full-rankings page surfaced; appears to no longer
  run one, or it isn't indexed.
- **RotoBaller** — page exists, claims PPR/non-PPR coverage, but the fetch returned
  only site chrome (dynamic content).
- Not chased at all, to avoid padding a focused report: The Athletic, numberFire
  (likely defunct post-FanDuel), Razzball, FTN Fantasy, DynastyProcess, NFFC/RTSports
  raw ADP feeds.

### Priority for the next research pass

Everything in "confirmed real but not fetchable today" is worth a manual copy-paste
pass by a human curator rather than further automated attempts — these are
specifically the outlets that would close the two gaps still open after this pass:
a true expert-consensus rank (not ADP) from a major outlet (FantasyPros' own ECR,
Yahoo's 6-analyst panel, or PFF's format-specific top-200s), and a second
expert-authored dynasty list to sit alongside KeepTradeCut's crowdsourced trade
values (DLF or Dynasty Nerds).

## When it applies in this codebase

- `backend/cmd/projections/testdata/consensus/*.json` — the current hand-curated
  snapshots; new sources are added as new rows here, then imported.
- `backend/cmd/projections/consensus.go` — `importConsensusRankings` (accepts any
  `source`/`format`/`metric_type` string — no source allowlist to update when adding
  one) and `computeConsensusDivergences`.
- `backend/internal/handlers/draft_consensus.go` — `loadConsensusValues`.
- `docs/stats/consensus-ensemble.md`, `docs/stats/auction-values.md` — the design
  rationale this catalog implements.

## References

- Source list and row counts: direct inspection of
  `rankings_2026-08-04.json` (515 rows, 9 sources, imported) and
  `situational_notes_2026-08-04.json` (27 rows, 17 distinct outlet strings) in this
  repo, 2026-08-14.
- `rankings_2026-08-14.json` (270 rows, 4 sources, captured but not yet imported) —
  built from live `WebFetch` pulls against CBS Sports, FantasyFootballCalculator, and
  4for4 on 2026-08-14, cross-validated against a third independent source (NBC
  Sports) for team-field corrections. See "Candidate sources" above for the full
  research trail, including what didn't pan out.
- Coverage-depth figures: `docs/algorithm-review.md` §7.1 (62-player 2026 PPR
  coverage check).
