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

## Source catalog (as of 2026-08-04 snapshot)

All rows currently live in
`backend/cmd/projections/testdata/consensus/rankings_2026-08-04.json`, imported via
`make project-nfl ARGS="-import-consensus f.json -season 2026"`. Ingestion is
manual/file-based — see "Why hand-curated" below.

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
scraping (checked directly, not assumed): FantasyPros' consensus rank page, Yahoo,
PFF, and CBS all failed a straightforward fetch during research for this feature.
There is no reliable automated refresh path today — snapshots go stale between manual
`-import-consensus` runs, and the `captured_date` on every row exists precisely so a
stale snapshot is visible rather than silently trusted.

## Candidate sources for future inclusion

Researched 2026-08-14 looking specifically for sources that would close the known
gaps above (a `standard`-format source, a `superflex` source, a second dynasty/expert
list, deeper positional coverage). Nothing below has a row in `nfl_consensus_rankings`
yet — these are candidates for the next `-import-consensus` file, evaluated by
actually attempting a fetch, not by reputation alone.

> **Method note.** Every "real data" claim below comes from an actual `WebFetch`
> during this research pass, not from training-data recall — several outlets that
> looked promising by name (Establish The Run, NFL.com, RotoBaller) turned out to be
> dead ends the moment someone tried to fetch them, which is exactly the failure mode
> this note is meant to catch before a row lands in the database.

### Genuinely accessible — real data fetched, gap it would close

| Outlet | URL | Publishes | Format | What it fetched | Currency |
|---|---|---|---|---|---|
| **CBS Sports** | `cbssports.com/fantasy/football/rankings/standard/top200/` and `/ppr/top200/` | Expert overall rank, top 200 | **standard** (closes our only-gap) + ppr | Top 30 fetched cleanly on both pages | Live, "updated 40s/24m ago" |
| **FantasyFootballCalculator** | `fantasyfootballcalculator.com/adp` | Crowd ADP from mock drafts | **standard** (12-team) | Top 30 fetched, based on 1,315 mock drafts Aug 7–14 2026 | Rolling weekly window |
| **4for4 Superflex ADP** | `4for4.com/superflex-adp` | Pick-format ADP | **superflex** (closes our only-gap) | Top 30 fetched; QBs correctly cluster rounds 1–3 as a real SF market would | "Last updated" Aug 13 2026 |
| **RotoWire** | `rotowire.com/football/adp.php` | ADP | ppr | Top 30 fetched cleanly | Dated Aug 14 2026 |
| **NBC Sports (Rotoworld)** | `nbcsports.com/fantasy/football/news/2026-fantasy-football-top-200-overall-rankings` | Expert overall rank, top 200 | unstated in the fetched text — **verify format before importing** | Top 30 fetched cleanly | Published Aug 5 2026 |
| **Footballguys** | `footballguys.com/salary-cap-auction-values` | Auction values | ppr, adjustable budget | Rows 1–15 free; 16+ gated behind a paid tier | Dated August 2026 |
| **RotoWire — Auction Values** | `rotowire.com/football/auction-values.php` | Auction values, $200 pool | unspecified | ~9 clean rows; several rows below returned "data unavailable" (partial dynamic/paywall gap) | Aug 14 2026 |

CBS Sports standard, spot-checked sample (fetched live, 2026-08-14):

```json
[
  {"rank": 1, "player": "J. Gibbs",    "pos": "RB", "team": "DET"},
  {"rank": 2, "player": "B. Robinson", "pos": "RB", "team": "ATL"},
  {"rank": 3, "player": "J. Chase",    "pos": "WR", "team": "CIN"},
  {"rank": 4, "player": "J. Taylor",   "pos": "RB", "team": "IND"},
  {"rank": 5, "player": "P. Nacua",    "pos": "WR", "team": "LAR"}
]
```

4for4 superflex ADP, spot-checked sample — exactly the QB-heavy shape a real
superflex market should produce, which our current sources cannot:

```json
[
  {"rank": 1, "player": "Jahmyr Gibbs",    "pos": "RB", "team": "DET", "adp": "1.01"},
  {"rank": 2, "player": "Josh Allen",      "pos": "QB", "team": "BUF", "adp": "1.02"},
  {"rank": 3, "player": "Bijan Robinson",  "pos": "RB", "team": "ATL", "adp": "1.03"},
  {"rank": 6, "player": "Lamar Jackson",   "pos": "QB", "team": "BAL", "adp": "1.06"},
  {"rank": 7, "player": "Drake Maye",      "pos": "QB", "team": "NE",  "adp": "1.07"}
]
```

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

### Dead ends, checked once and abandoned

- **MyFantasyLeague ADP API** — a real endpoint exists, but the extracted sample
  mixed linebackers into a skill-position ADP list, which isn't plausible. Not
  trustworthy enough to report as verified; would need a direct, non-summarized fetch.
- **Establish The Run** — real and reputable, but its rankings sit behind a flat
  $54.99 paywall with no free tier.
- **NFL.com** — no standalone full-rankings page surfaced; appears to no longer
  run one, or it isn't indexed.
- **RotoBaller** — page exists, claims PPR/non-PPR coverage, but the fetch returned
  only site chrome (dynamic content).
- Not chased at all, to avoid padding a focused report: The Athletic, RotoWire's
  full rankings page (vs. its ADP page, which is covered above), numberFire (likely
  defunct post-FanDuel), Razzball, FTN Fantasy, DynastyProcess, NFFC/RTSports raw
  ADP feeds.

### Priority if curating the next import file

1. **CBS Sports standard + FantasyFootballCalculator ADP** — the only two real,
   current, fetchable sources for the `standard` format, which we have zero coverage
   of today.
2. **4for4 superflex ADP** — the only real, current, fetchable source for
   `superflex`, same situation.
3. **RotoWire ADP (ppr)** — a tenth source for the already-covered PPR pool, useful
   purely for corroboration depth (more sources per player pushes more rows past the
   `source_count >= 2` bar for being taken seriously).
4. Everything in "confirmed real but not fetchable" is worth a manual copy-paste
   pass by a human curator rather than further automated attempts — these are the
   outlets that would close the ECR and second-dynasty-source gaps specifically.

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
  `rankings_2026-08-04.json` (515 rows, 9 sources) and
  `situational_notes_2026-08-04.json` (27 rows, 17 distinct outlet strings) in this
  repo, 2026-08-14.
- Coverage-depth figures: `docs/algorithm-review.md` §7.1 (62-player 2026 PPR
  coverage check).
