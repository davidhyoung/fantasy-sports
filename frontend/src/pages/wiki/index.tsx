import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import Formula, { C } from './components/Formula'
import Note from './components/Note'
import Pipeline from './components/Pipeline'

const TOC = [
  { href: '#grades', label: '01 · Grades', stage: true },
  { href: '#projections', label: '02 · Projections', stage: true },
  { href: '#profiles', label: 'Season profiles', sub: true },
  { href: '#similarity', label: 'Comp matching', sub: true },
  { href: '#growth', label: 'Growth & rookies', sub: true },
  { href: '#uncertainty', label: 'Uncertainty', sub: true },
  { href: '#calibration', label: 'Calibration', sub: true },
  { href: '#rankings', label: '03 · League value', stage: true },
  { href: '#auction', label: '04 · Auction pricing', stage: true },
  { href: '#tiering', label: '05 · Tiering', stage: true },
  { href: '#consensus', label: 'Consensus check', cross: true },
  { href: '#techniques', label: 'Technique status' },
  { href: '#glossary', label: 'Glossary' },
]

function Toc() {
  return (
    <nav className="hidden lg:block">
      <div className="sticky top-[calc(var(--nav-height)+1.5rem)] flex flex-col gap-0.5 max-h-[calc(100vh-var(--nav-height)-3rem)] overflow-y-auto pr-2">
        {TOC.map((item) => (
          <a
            key={item.href}
            href={item.href}
            className={`font-display border-l-2 border-transparent pl-3 py-1 text-[13px] hover:border-border hover:text-foreground ${
              item.stage
                ? 'font-semibold text-foreground'
                : item.cross
                  ? 'font-semibold text-highlight-foreground'
                  : item.sub
                    ? 'pl-5 text-[12px] text-muted-foreground'
                    : 'text-muted-foreground'
            }`}
          >
            {item.label}
          </a>
        ))}
      </div>
    </nav>
  )
}

/**
 * Below `lg` the sidebar TOC (`Toc`, above) vanishes with nowhere to go, so
 * this is its tap-driven replacement: a button under the `<h1>` opening a flat
 * list of the same jump links. Tapping a link scrolls to the section (the
 * browser's native `#hash` anchor behavior) and auto-collapses the list, since
 * once you've navigated there's nothing left to pick from it.
 */
function MobileToc() {
  const [open, setOpen] = useState(false)
  return (
    <div className="lg:hidden">
      <button
        onClick={() => setOpen(true)}
        className="inline-flex h-11 items-center gap-1.5 rounded-md border border-border bg-card px-3.5 font-display text-xs font-semibold text-foreground"
      >
        <span aria-hidden>☰</span> Contents <span aria-hidden className="text-[9px]">▾</span>
      </button>
      <MobileSheet open={open} onClose={() => setOpen(false)} title="Contents">
        <ul>
          {TOC.map((item) => (
            <li key={item.href} className="border-b border-border last:border-0">
              <a
                href={item.href}
                onClick={() => setOpen(false)}
                className={`flex min-h-[var(--tap-target-min)] items-center py-2 font-display text-sm ${
                  item.stage
                    ? 'font-semibold text-foreground'
                    : item.cross
                      ? 'font-semibold text-highlight-foreground'
                      : item.sub
                        ? 'pl-3 text-muted-foreground'
                        : 'text-muted-foreground'
                }`}
              >
                {item.label}
              </a>
            </li>
          ))}
        </ul>
      </MobileSheet>
    </div>
  )
}

function StageHead({
  num,
  title,
  kicker,
  cross,
}: {
  num: string
  title: string
  kicker: string
  cross?: boolean
}) {
  return (
    <div className="mb-6">
      <div className="flex items-baseline gap-2">
        <span className={`font-mono text-sm ${cross ? 'text-highlight' : 'text-primary'}`}>{num}</span>
        <h2 className="font-display text-xl font-bold text-foreground">{title}</h2>
      </div>
      <p className="mt-2 max-w-xl text-sm text-muted-foreground">{kicker}</p>
    </div>
  )
}

function Sub({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <h3 id={id} className="font-display text-[15px] font-semibold text-foreground mt-8 mb-2 scroll-mt-24">
      {children}
    </h3>
  )
}

export default function Wiki() {
  return (
    <div className="max-w-6xl mx-auto grid grid-cols-1 lg:grid-cols-[200px_1fr] gap-10">
      <Toc />

      <div className="min-w-0 lg:max-w-2xl space-y-16">
        {/* ── Overview ── */}
        <div>
          <p className="font-display text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            The stat engine, end to end
          </p>
          <h1 className="font-display text-[28px] font-bold text-foreground mt-1">
            Signal <span className="text-primary">&amp;</span> Noise
          </h1>
          <div className="mt-3">
            <MobileToc />
          </div>
          <p className="mt-3 text-[15px] leading-relaxed text-muted-foreground">
            Every number in this app — a grade, a projection, a rank, a dollar sign on
            draft day — is an attempt to separate a player's real signal from the
            noise of a small sample, a soft schedule, or a single hot week. This page
            walks the five stages that turn raw box scores into draft-day numbers, and
            the cross-cutting check that keeps the model honest against the outside
            world.
          </p>
          <Pipeline />
        </div>

        {/* ── 01 Grades ── */}
        <section id="grades" className="scroll-mt-24">
          <StageHead
            num="01"
            title="Player Grades"
            kicker="Real-life quality, 0-100, deliberately kept separate from fantasy scoring — a great blocking, ball-security RB should grade well even in a league that doesn't reward either."
          />
          <div className="space-y-3 text-sm leading-relaxed text-foreground">
            <p>
              A grade answers a different question than a projection: not "how many
              points will he score in <em>your</em> league," but "how good is this
              player at football." <code className="font-mono text-[13px]">grades.go</code>{' '}
              computes one grade per player per season from four sub-scores, each built
              from the same shrunk, schedule-adjusted z-scores that back projections.
            </p>
          </div>

          <Sub id="grade-weights">Four sub-scores, position-specific weights</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Production (raw output), Efficiency (rate stats — EPA/play, YPC, completion
            %), Usage (target/rush share — the role he's trusted with), and Durability
            (games played ÷ 17). The weights shift by position: a QB's grade leans on
            efficiency, an RB's splits evenly across all four because volume, burst and
            health are all genuinely part of "good."
          </p>
          <div className="overflow-x-auto rounded-lg border border-border mt-3">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Position</TableHead>
                  <TableHead className="text-right">Production</TableHead>
                  <TableHead className="text-right">Efficiency</TableHead>
                  <TableHead className="text-right">Usage</TableHead>
                  <TableHead className="text-right">Durability</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ['QB', 0.3, 0.35, 0.2, 0.15],
                  ['RB', 0.25, 0.25, 0.25, 0.25],
                  ['WR', 0.25, 0.3, 0.3, 0.15],
                  ['TE', 0.3, 0.3, 0.25, 0.15],
                  ['K', 0.4, 0.4, 0.05, 0.15],
                ].map((row) => (
                  <TableRow key={row[0] as string}>
                    <TableCell className="text-muted-foreground">{row[0]}</TableCell>
                    {(row.slice(1) as number[]).map((v, i) => (
                      <TableCell key={i} className="text-right font-mono tabular-nums">
                        {v.toFixed(2)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          <Sub id="grade-percentile">Percentile, not a raw z-score</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            A grade is an <em>empirical percentile</em> within the position group that
            season — where a player's weighted score falls among everyone else's, not a
            distance-from-mean number that means nothing to a reader. It uses the Hazen
            formula, which keeps 0 and 100 both technically unreachable:
          </p>
          <Formula>percentile = (players_below + 0.5) / n × 100</Formula>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Above 90, a soft ceiling compresses further gains — reaching 99 should feel
            like it costs more than reaching 60 did:
          </p>
          <Formula>
            {'grade = 90 + 10 × (1 - e^(-0.18 × (raw - 90)))   '}
            <C>{'// only above the raw-90 knee'}</C>
            {'\n\n'}
            <C>{'raw 95   → 93.2\nraw 98   → 95.5\nraw 99.5 → 96.3'}</C>
          </Formula>

          <Sub id="grade-durability">Durability</Sub>
          <Formula>durability = min(1.0, games_played / 17)</Formula>

          <Sub id="grade-feedback">Where the grade feeds back in</Sub>
          <ul className="list-disc pl-5 space-y-1.5 text-sm leading-relaxed text-muted-foreground">
            <li>
              <code className="font-mono text-[13px] text-foreground">overall_grade_z</code>{' '}
              is injected into each player's season profile and used as its own weighted
              dimension group (weight <span className="font-mono">1.25</span>) in comp
              matching — the engine prefers comps who were similarly <em>good</em> at
              football, not just stat-line twins.
            </li>
            <li>
              A bounded <span className="font-mono">±5%</span> grade-trend nudge adjusts
              projected stats for players whose real-life grade is trending faster than
              their box score.
            </li>
            <li>
              Rendered as the grade card on player detail, and as a sortable column on
              projections, draft, and player tables.
            </li>
          </ul>
        </section>

        {/* ── 02 Projections ── */}
        <section id="projections" className="scroll-mt-24">
          <StageHead
            num="02"
            title="Stat Projections"
            kicker="A comp-based (PECOTA-style) engine: find historically similar players, and let how they developed be the projection — not a fixed aging curve."
          />
          <p className="text-sm leading-relaxed text-foreground">
            Instead of a generic regression-to-the-mean curve, the engine (an offline
            batch CLI) finds each target player's closest historical comps and projects
            forward using how <em>those</em> players actually developed. If a player's
            comps all declined sharply at 29, that's baked in. Comps who{' '}
            <strong>washed out of the league</strong> entirely count as zero-growth
            outcomes — skipping them, as an earlier version did, was survivorship bias
            that quietly inflated projections for every risky archetype.
          </p>

          <Sub id="profiles">Season profiles</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            One row per player per season (minimum 4 games), built from weekly stats in
            two adjustment passes before anything is compared to anything else.
          </p>
          <p className="text-sm leading-relaxed text-muted-foreground mt-2">
            <strong className="text-foreground">Strength of schedule.</strong> Yards,
            TDs, fantasy points, air yards, YAC and first downs are scaled per game by
            how the opposing defense performed against that position league-wide,
            capped to keep outliers sane:
          </p>
          <Formula>
            {'adjusted = raw × (league_avg_allowed / opponent_allowed_to_position)   '}
            <C>{'// capped to [0.75, 1.25]'}</C>
          </Formula>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Volume (attempts, targets), turnovers, kicking and EPA stay raw — those
            aren't the kind of stat a defense "allows more of."
          </p>

          <p className="text-sm leading-relaxed text-muted-foreground mt-4">
            <strong className="text-foreground">Bayesian shrinkage on rate stats.</strong>{' '}
            Before z-scoring, rates like YPA, completion%, YPC, YPR and FG% are pulled
            toward the position-group mean, weighted by sample size — the same fix that
            keeps a 3-for-3 kicker from grading like he's automatic:
          </p>
          <Formula>
            {'shrunk_rate = (n × observed + k × position_mean) / (n + k)   '}
            <C>{'// k = median sample size in the pool'}</C>
          </Formula>
          <div className="overflow-x-auto rounded-lg border border-border mt-3">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Player</TableHead>
                  <TableHead className="text-right">Carries</TableHead>
                  <TableHead className="text-right">Raw YPC</TableHead>
                  <TableHead className="text-right">Shrunk YPC (k=80)</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow>
                  <TableCell>Veteran, full season</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">180</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">4.2</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">4.24</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>Rookie, hot start</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">25</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">6.8</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">4.92</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>Backup, cameo</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">8</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">2.1</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">4.28</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Usage shares (target share, rush-yard share) are deliberately{' '}
            <em>not</em> shrunk — a role is real even in a small sample; only rate stats
            get pulled toward the mean. Every rate is then z-scored within its position
            group <em>globally across all seasons since 1999</em>, so a z-score of +1.0
            means the same thing in 2005 and 2024.
          </p>

          <Note kind="fix" title="Extension — recency-weighted targets">
            <p>
              A player's <em>base</em> season and the season before it are blended for
              comp-matching purposes only (never for the historical comp pool, which
              stays single-season), weighted by games played and an exponential
              recency decay. At <span className="font-mono">decay=0</span> it's an
              exact no-op; it exists because an injury-shortened four-game season and a
              healthy seventeen-game one at the same per-game rate look{' '}
              <em>identical</em> to the matching step otherwise, even though one rests
              on 4× less evidence.
            </p>
            <p>
              Malik Nabers, 2025: 4 games at{' '}
              <span className="font-mono">target_share=0.271</span> blended with a
              healthy 2024 (15 games, <span className="font-mono">0.357</span>) at{' '}
              <span className="font-mono">decay=0.5</span> →{' '}
              <span className="font-mono">0.327</span> — closer to his real role than 4
              games alone suggests.
            </p>
          </Note>

          <Note kind="caveat" title="The failure mode this didn't cover">
            <p>
              Short-season shrinkage only blends against a <em>prior</em> season — a
              rookie whose own debut was injury-shortened has nothing to blend
              against. Cam Skattebo's 2025 ended Week 8 after a hot 3-TD stretch; his
              unregressed 8-game rate comped him to RB6 (279 pts) against a market read
              of RB18. A dedicated fix now regresses the counting stats (not raw
              opportunity) toward the position mean, weighted by{' '}
              <span className="font-mono">games_played/17</span>, whenever there's no
              prior season to lean on — post-fix he lands at RB17 (206 pts), within one
              rank of consensus.
            </p>
          </Note>

          <Sub id="similarity">Comp matching</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Each target's profile is compared against every historical profile at the
            same position (age ±2 years) using weighted Euclidean distance over{' '}
            <strong className="text-foreground">dimension groups</strong>, not
            individual stats — so adding more fields to a group doesn't accidentally
            dilute its weight:
          </p>
          <Formula>
            {'group_diff_g = avg_z(target, g) - avg_z(candidate, g)\n'}
            {'distance     = sqrt( Σ w_g × group_diff_g² / Σ w_g )\n'}
            {'similarity   = 1 / (1 + distance)          '}
            <C>{'// accepted only if ≥ 0.60'}</C>
          </Formula>
          <p className="text-sm leading-relaxed text-muted-foreground">
            There's no fixed comp count — a common archetype might produce 40+ comps, a
            genuinely unusual player 0.
          </p>
          <div className="overflow-x-auto rounded-lg border border-border mt-3">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Position</TableHead>
                  <TableHead>Heaviest weight</TableHead>
                  <TableHead>Also weighted</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ['QB', 'Passing (3.0)', 'Rushing 2.0 · Value 2.0 · Grade 1.25'],
                  ['RB', 'Rushing (3.0)', 'Receiving 2.0 · Value 2.0 · Grade 1.25'],
                  ['WR', 'Receiving (3.0)', 'Value 2.0 · Grade 1.25 · Rushing 0.75'],
                  ['TE', 'Receiving (3.0)', 'Value 2.0 · Grade 1.25'],
                  ['K', 'Kicking / Value (3.0)', 'Grade 1.25'],
                ].map((row) => (
                  <TableRow key={row[0]}>
                    <TableCell className="text-muted-foreground">{row[0]}</TableCell>
                    <TableCell className="font-mono text-[13px]">{row[1]}</TableCell>
                    <TableCell className="text-muted-foreground text-[13px]">{row[2]}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Weights live in a tunable config, refined by the autotuner. A "grade"
            dimension group (weight 1.25 across positions) prefers comps who graded
            similarly well overall; draft capital is added (weight 1.0) only for
            players with under 3 years of experience.
          </p>

          <Sub id="growth">Growth curve, then rookies</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Each accepted comp's own year-over-year growth rate, similarity²-weighted,
            is applied to the target's base value:
          </p>
          <Formula>
            {'weight_i        = similarity_i² / Σ similarity²\n'}
            {'projected_PPR/G = base_PPR/G × Σ weight_i × growth_i   '}
            <C>{'// growth clamped to [0.1, 3.0]; washouts contribute 0'}</C>
          </Formula>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Zero-comp targets regress halfway to the position-group mean rather than
            repeating their season verbatim. A small phase-based aging multiplier sits
            on top (developing ×1.02 … late-career ×0.93), deliberately minor since
            age-matched comps already carry most of the aging signal.
          </p>

          <Note title="Rookies are a different problem entirely">
            <p>
              A player with zero NFL games has no profile to grow forward. Incoming
              rookies instead comp against <em>historical rookie seasons</em>, weighted
              purely by a Gaussian kernel on draft slot (σ=40 picks — UDFAs are pinned
              to a synthetic late slot so they comp against other UDFAs). The
              projection <em>is</em> the comps' own rookie-season output,
              similarity²-weighted — there's no prior value to grow from. Confidence is
              capped lower by construction, since the target has zero seasons of his
              own backing the number.
            </p>
          </Note>

          <Sub id="uncertainty">Uncertainty, not just a point estimate</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            The comp set doubles as an empirical outcome distribution for free — the
            same weighted machinery that produces the mean also produces the spread:
          </p>
          <Formula>
            {'μ   = Σ weight_i × stat_i\n'}
            {'σ²  = Σ weight_i × (stat_i - μ)²\n'}
            {'P_q = value where cumulative comp weight first exceeds q     '}
            <C>{'// P10 / P50 / P90'}</C>
          </Formula>
          <div className="overflow-x-auto rounded-lg border border-border mt-3">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Player</TableHead>
                  <TableHead className="text-right">Comps</TableHead>
                  <TableHead className="text-right">Mean</TableHead>
                  <TableHead className="text-right">Stdev</TableHead>
                  <TableHead className="text-right">P10</TableHead>
                  <TableHead className="text-right">P50</TableHead>
                  <TableHead className="text-right">P90</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ['Consistent archetype', 22, 1050, 140, 880, 1050, 1230],
                  ['Unique / volatile', 6, 1050, 310, 640, 1050, 1470],
                ].map((row) => (
                  <TableRow key={row[0] as string}>
                    <TableCell>{row[0]}</TableCell>
                    {(row.slice(1) as number[]).map((v, i) => (
                      <TableCell key={i} className="text-right font-mono tabular-nums">
                        {v}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Same point estimate, very different floor — that's exactly the distinction
            a single confidence score can't express. Confidence captures "how much do I
            trust this process at all"; the P10–P90 band captures "how wide is the
            honest range." A boom/bust player can score high on both at once.
          </p>

          <Sub id="calibration">Level calibration</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Backtesting 2015–2024 found a proportional, not additive, ~12%
            over-projection at every level and every season — not a bug in any one
            position, a uniform bias. Because a <em>uniform</em> scale factor cancels
            out of value-over-replacement math, correcting it moved the displayed
            points without moving a single rank, auction dollar, or consensus value on
            the live board:
          </p>
          <Formula>
            {'VOR = k·points - k·replacement = k·(points - replacement)   '}
            <C>{'// k cancels out of VOR'}</C>
            {'\n\n'}
            {'calibrated = raw × 0.884   '}
            <C>{'// seeded uniform, applied to totals and rates alike'}</C>
            {'\n'}
            <C>{'// bias: +1.583 ppg → -0.000 · MAE: 3.337 → 2.988'}</C>
          </Formula>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Per-position factors (QB 0.953, RB 0.840, WR 0.873, TE 0.880) would
            calibrate each position exactly, but would reprice positions{' '}
            <em>against</em> each other on the draft board — left as a deliberate open
            decision, not an oversight.
          </p>

          <Sub id="confidence">Confidence score</Sub>
          <Formula>
            {'confidence = 0.25 × avg_similarity\n'}
            {'           + 0.20 × min(1, comp_count / 10)\n'}
            {'           + 0.25 × comp_agreement        '}
            <C>{'// 1 / (1 + stdev of comp growths)'}</C>
            {'\n           + 0.15 × sample_depth          '}
            <C>{'// fraction of comps with a usable outcome'}</C>
            {'\n           + 0.15 × min(1, seasons_of_history / 3)'}
          </Formula>

          <Sub id="backtesting">Backtesting &amp; tuning</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Every historical replay respects temporal integrity — target season N sees
            only profiles and priors from seasons before N. The tuning objective is{' '}
            <strong className="text-foreground">per-game Spearman rank correlation</strong>,
            not point-error, because draft decisions are ordinal: getting the order
            right matters more than nailing exact totals. A tuned config only ships if
            it beats the default on held-out validation seasons.
          </p>
          <div className="overflow-x-auto rounded-lg border border-border mt-3">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Split</TableHead>
                  <TableHead className="text-right">Per-game ρ</TableHead>
                  <TableHead className="text-right">Season-total RMSE</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow>
                  <TableCell>Train (2015–2021)</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">0.71</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">—</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>Validation (2022–2024)</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">0.75</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">68–81 pts</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </section>

        {/* ── 03 Rankings ── */}
        <section id="rankings" className="scroll-mt-24">
          <StageHead
            num="03"
            title="League Value & Rankings"
            kicker="The same stat line is worth a different amount in every league — value has to be computed against the specific roster and scoring rules in front of you."
          />
          <p className="text-sm leading-relaxed text-foreground">
            Two modes, selected by sport. NFL leagues score points, so value is
            scarcity against a replacement player; NBA leagues score head-to-head
            categories, so value is a weighted z-score.
          </p>

          <Sub id="categories-mode">Categories mode — NBA</Sub>
          <Formula>
            {'z_c      = (value - mean_c) / stdev_c              '}
            <C>{"// flipped for \"lower is better\" cats like TO"}</C>
            {'\nweight_c = (CV_c × scarcity_c), normalized to mean 1.0\n'}
            {'  CV       = stdev / |mean|                       '}
            <C>{'// categories with more spread matter more'}</C>
            {'\n  scarcity = 1 / (1 + max(0, avg_FA_z))            '}
            <C>{'// deep free-agent pool in a cat discounts it'}</C>
            {'\n\noverall_score  = Σ weight_c × z_c\n'}
            {'position_score = z-score of overall_score within position group'}
          </Formula>

          <Sub id="points-mode">Points mode — NFL (VORP)</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Total points come straight from local season stats, translated through a
            canonical stat-ID vocabulary — Yahoo supplies fantasy <em>context</em> here
            (rosters, scoring, slots), not the underlying stats.
          </p>
          <Formula>
            {'total_points = Σ stat_value × league_stat_modifier\n\n'}
            {'replacement[pos] = points of the last starter-demand player at that position\n'}
            {'VORP(p)          = points(p) - replacement[pos(p)]'}
          </Formula>

          <Note kind="fix" title="Fix — flex slots pool, they don't split evenly">
            <p>
              Starter demand from dedicated slots (QB:1) is easy: count × teams. Flex
              slots are harder — an ordinary W/R/T FLEX splitting evenly across
              RB/WR/TE is a fine approximation, because those three are genuinely
              comparable at the margin. A superflex (Q/W/R/T) slot split the same way
              credited QB with only 0.25 of a starter per SFLEX — QB dwarfs the other
              three in points formats, so that undervalued QB by roughly half of what a
              real superflex market prices in.
            </p>
            <p>
              The fix: dedicated slots still claim starters directly; each flex-type
              slot then pools its still-<em>unclaimed</em> eligible candidates by
              projected value and lets the best players claim the spots, position-blind,
              no tunable weight. Verified: a league with SFLEX:1 now prices QBs{' '}
              <strong>identically</strong> to an explicit QB:2 roster — both land on 24
              starters / 2.0 slots-per-team, versus 15 / 1.25 under the old even split.
            </p>
          </Note>
        </section>

        {/* ── 04 Auction ── */}
        <section id="auction" className="scroll-mt-24">
          <StageHead
            num="04"
            title="Auction Pricing"
            kicker="Points don't answer 'what fraction of $200 is he worth' — money has to follow scarcity, and scarcity is exactly what VORP measures."
          />
          <Formula>
            {'roster_spots = teams × Σ(all roster slots, starters + bench)\n'}
            {'surplus      = teams × budget - roster_spots        '}
            <C>{'// $1 reserved per spot, up front'}</C>
            {'\ncompressedVOR(p) = max(0, VORP(p))^0.75              '}
            <C>{'// see "why compress" below'}</C>
            {'\nauction(p)   = max(1, round(1 + compressedVOR(p) / ΣcompressedVOR × surplus))'}
          </Formula>

          <Note kind="fix" title="Fix — the $1 reservation">
            <p>
              Standard value-based-drafting practice reserves a dollar for{' '}
              <em>every</em> roster spot, bench included, before splitting what's left
              by VOR share. The original version skipped that reservation and split the
              full nominal budget across only the VOR-positive players — the board ran
              systematically hot, worst at the top. Restricted to players who'd
              actually fill a roster spot, the board's sum now lands within $1 of{' '}
              <span className="font-mono">teams × budget</span>, as VBD math says it
              should.
            </p>
          </Note>

          <Note kind="caveat" title="Why compress VOR to the 0.75 power">
            <p>
              A shallow single-QB roster (Yahoo's common default) leaves few
              flex-eligible starter slots, concentrating the whole linear-shared pool
              onto a handful of players — the consensus #1 overall blew past $100 in a
              12-team/$200 league, well above the real-world ceiling (~$70–80). Real
              bidders don't execute pure linear VOR math; budget-diversification
              instincts cap how much of $200 anyone puts on one player. Raising VOR to
              the 0.75 power compresses the spread while leaving rank order untouched —
              a single empirically-picked global constant, not tuned per league.
            </p>
          </Note>

          <Sub id="consensus-pricing">Reading the market's rank on our own curve</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            External sources publish ranks and ADP, not dollars. Rather than invent a
            second pricing model, the consensus dollar figure reads <em>our own</em>{' '}
            price curve at the rank the market assigns:
          </p>
          <Formula>
            {'consensus_rank(p)    = median over sources of RANK() within (source, position)\n'}
            {'consensus_auction(p) = our_auction_curve[ position ][ round(consensus_rank(p)) ]\n'}
            {'edge(p)               = auction(p) - consensus_auction(p)'}
          </Formula>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Both numbers share the same curve, so <span className="font-mono">edge</span>{' '}
            isolates exactly one thing — ranking disagreement — with no artifact of a
            second pricing model.
          </p>
          <div className="overflow-x-auto rounded-lg border border-border mt-3">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Player</TableHead>
                  <TableHead>Pos</TableHead>
                  <TableHead className="text-right">Our rank</TableHead>
                  <TableHead className="text-right">Cons. rank</TableHead>
                  <TableHead className="text-right">Our $</TableHead>
                  <TableHead className="text-right">Cons $</TableHead>
                  <TableHead className="text-right">Edge</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ['Kyle Pitts', 'TE', '13', '5.0', '$1', '$34', '−$33', false],
                  ['Justin Jefferson', 'WR', '16', '6.0', '$26', '$46', '−$20', false],
                  ['Malik Nabers', 'WR', '5', '13.5', '$55', '$29', '+$26', true],
                  ['Cam Skattebo', 'RB', '17', '18.0', '$23', '$21', '+$2', true],
                ].map((row) => (
                  <TableRow key={row[0] as string}>
                    <TableCell>{row[0]}</TableCell>
                    <TableCell className="text-muted-foreground">{row[1]}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{row[2]}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{row[3]}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{row[4]}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{row[5]}</TableCell>
                    <TableCell
                      className={`text-right font-mono tabular-nums ${
                        row[7] ? 'text-positive-foreground' : 'text-negative-foreground'
                      }`}
                    >
                      {row[6]}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Uncovered players (external sources publish roughly the top 100) show{' '}
            <span className="font-mono">—</span>, never{' '}
            <span className="font-mono">$0</span> — they sort last either way rather
            than pretending to be worthless.
          </p>
        </section>

        {/* ── 05 Tiering ── */}
        <section id="tiering" className="scroll-mt-24">
          <StageHead
            num="05"
            title="Tiering"
            kicker="'RB6 vs RB7' implies a precision the projections don't have. The question a drafter actually asks is: if I wait a round, do I still get someone like this?"
          />
          <p className="text-sm leading-relaxed text-foreground">
            A rank can't answer that; a tier can. Sort a position by value descending
            and walk it once, starting a new tier only when the running spread would
            blow a width budget:
          </p>
          <Formula>
            {'budget = (value[0] - value[draftable-1]) / targetTiers    '}
            <C>{'// targetTiers = 8, draftable = ceil(starters × teams × 1.5)'}</C>
            {'\n\nfor each player p, descending by value:\n'}
            {'    if tierTop - p.value > budget: start a new tier\n'}
            {'    assign current tier to p'}
          </Formula>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Two properties fall out of that one rule: everyone inside a tier is
            genuinely within one budget of everyone else in it, and cliffs break tiers
            automatically — a cliff <em>is</em> precisely a gap that blows the budget,
            with no separate gap-detection rule needed.
          </p>
          <div className="overflow-x-auto rounded-lg border border-border mt-3">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Pos</TableHead>
                  <TableHead className="text-right">Tier</TableHead>
                  <TableHead className="text-right">n</TableHead>
                  <TableHead className="text-right">Spread</TableHead>
                  <TableHead>Players</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ['RB', '1', '2', '2.8', 'Robinson, Gibbs'],
                  ['RB', '2', '3', '16.1', 'Achane, McCaffrey, Taylor'],
                  ['WR', '1', '1', '0.0', 'Nacua (alone — next drop is 34 pts)'],
                  ['WR', '3', '4', '13.9', 'Smith-Njigba … Rice'],
                ].map((row, i) => (
                  <TableRow key={i}>
                    <TableCell className="text-muted-foreground">{row[0]}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{row[1]}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{row[2]}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{row[3]}</TableCell>
                    <TableCell className="text-[13px]">{row[4]}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Read the WR column: Nacua and Chase are each alone in their own tier
            because the drop to the next receiver blows the budget — genuinely not
            interchangeable with anyone. Then four receivers sit inside 14 points of
            each other: miss the first, take the next, lose almost nothing.
          </p>
        </section>

        {/* ── Cross-cutting: Consensus ── */}
        <section id="consensus" className="scroll-mt-24">
          <StageHead
            num="✦"
            title="Consensus & Divergence"
            cross
            kicker="The model only ever sees last season's stats. It has no way to know about a June trade, a camp injury, or a lost snap-share battle — consensus is the check that catches what the model is structurally blind to."
          />
          <p className="text-sm leading-relaxed text-foreground">
            Two mechanisms, deliberately kept separate rather than blended into the
            projection itself, pending a season of retrospective validation.
          </p>

          <Sub id="rank-divergence">Rank divergence</Sub>
          <Formula>
            {'source_rank(p, s)    = RANK() OVER (PARTITION BY source, position ORDER BY value ASC)\n'}
            {'consensus_rank(p)    = median( source_rank(p, s) for every source that ranked p )\n'}
            {'rank_delta(p)        = our_rank(p) - consensus_rank(p)          '}
            <C>{'// within position group'}</C>
          </Formula>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Ranked <em>within position</em>, not overall — an overall ranking clusters
            nearly every startable QB in the top 30–40 regardless of quality, since QBs
            structurally outscore every other position; that alone produced rank deltas
            of 40–94 on the first run, a methodology artifact, not a real signal.
            Median, not mean, absorbs single-outlet outliers.
          </p>

          <Sub id="situational-tagging">Situational tagging</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Discrete events — injury, depth-chart battle, scheme change, holdout,
            trade — carry a direction, a magnitude, and a source-confidence level,
            surfaced <em>alongside</em> a projection rather than folded into it. Events
            that are really team-wide (a QB competition affecting every pass-catcher)
            carry an explicit team scope and print for every player on that roster with
            no inference about which way the impact cuts — a human makes that call, the
            model doesn't guess.
          </p>

          <Note kind="open" title="Worked example — why the model missed Justin Jefferson">
            <p>
              Our rank: 32 (WR). Consensus: 6 — a 26-spot gap. Jefferson's per-game PPR
              fell 19.0 → 11.6 even though his own role held (target share 0.307 vs
              0.301). What collapsed was Minnesota's entire passing offense — team pass
              yards/game down 27% behind a struggling rookie QB. The comp engine
              correctly matched him to "elite target-share receiver stuck with bad QB
              play" and those comps' growth is genuinely mixed. Consensus ranks him
              top-11 because the market already knows what the model structurally
              can't: Minnesota's 2026 starting job is expected to go to a proven
              veteran. Not a bug — a real blind spot, now visible instead of silent.
            </p>
          </Note>

          <Sub id="sources">Sources</Sub>
          <p className="text-sm leading-relaxed text-muted-foreground">
            Nine sources imported live today — expert ranks, live-draft ADP, and one
            dynasty trade-value feed — plus a second snapshot closing the standard-
            format and superflex gaps the first left open. Coverage is intentionally
            shallow — every source publishes roughly its own top 60; "not ranked" means
            the source didn't go that deep, not that they'd rank a player last.
          </p>
          <div className="overflow-x-auto rounded-lg border border-border mt-3">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Source</TableHead>
                  <TableHead>Publishes</TableHead>
                  <TableHead>Format</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ['ESPN · SI (×2) · Bleacher Report', 'Expert consensus rank', 'PPR'],
                  ['FantasyPros · Sleeper · ESPN ADP', 'Aggregated / live-draft ADP', 'PPR'],
                  ['Underdog', 'Live-draft ADP', 'Half-PPR'],
                  ['KeepTradeCut', 'Crowdsourced dynasty value', 'Dynasty'],
                  ['CBS Sports', 'Expert overall rank, top 200', 'Standard / PPR'],
                  ['FantasyFootballCalculator', 'Crowd mock-draft ADP', 'Standard'],
                  ['4for4', 'Pick-format ADP', 'Superflex'],
                ].map((row, i) => (
                  <TableRow key={i}>
                    <TableCell className="text-[13px]">{row[0]}</TableCell>
                    <TableCell className="text-muted-foreground text-[13px]">{row[1]}</TableCell>
                    <TableCell className="text-muted-foreground text-[13px]">{row[2]}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </section>

        {/* ── Technique status ── */}
        <section id="techniques" className="scroll-mt-24">
          <div className="mb-6">
            <h2 className="font-display text-xl font-bold text-foreground">Technique status board</h2>
            <p className="mt-2 max-w-xl text-sm text-muted-foreground">
              The full write-up for each technique — assumptions, worked examples, how
              to validate it — lives in <span className="font-mono">docs/stats/</span>.
              This is the map.
            </p>
          </div>
          <div className="overflow-x-auto rounded-lg border border-border">
            <Table>
              <TableHeader style={{ position: 'static' }}>
                <TableRow>
                  <TableHead>Technique</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Where</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(
                  [
                    ['Bayesian shrinkage (rates + growth)', 'pink', 'Live', 'bayesian-shrinkage.md'],
                    ['Uncertainty quantification (P10/P50/P90)', 'pink', 'Live', 'uncertainty-quantification.md'],
                    ['Strength of schedule (multiplicative)', 'pink', 'Live', 'strength-of-schedule.md'],
                    ['Recency-weighted target profiles', 'pink', 'Live, k=0 default', 'recency-weighted-profiles.md'],
                    ['Auction values + tiering', 'pink', 'Live', 'auction-values.md, tiering.md'],
                    ['Consensus divergence', 'amber', 'Diagnostic only', 'consensus-ensemble.md'],
                    ['Blending consensus into projections', 'purple', 'Pending validation', '§ consensus-ensemble.md'],
                    ['Shrinkage on live (non-season) rankings', 'purple', 'Not yet ported', 'analysis.go'],
                    ['SRS-style iterative SOS', 'purple', 'Future work', '§ strength-of-schedule.md'],
                    ['Consensus source catalog', 'blue', 'Reference', 'consensus-sources.md'],
                  ] as [string, 'pink' | 'amber' | 'purple' | 'blue', string, string][]
                ).map(([name, color, statusLabel, where]) => (
                  <TableRow key={name}>
                    <TableCell className="text-[13px]">{name}</TableCell>
                    <TableCell>
                      <Badge variant={color}>{statusLabel}</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-[12px] text-muted-foreground">{where}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </section>

        {/* ── Glossary ── */}
        <section id="glossary" className="scroll-mt-24 pb-8">
          <h2 className="font-display text-xl font-bold text-foreground mb-4">Glossary</h2>
          <dl className="space-y-3">
            {[
              ['Comp', 'A historically similar player, matched by weighted distance over dimension groups; the projection engine’s unit of evidence.'],
              ['VOR / VORP', 'Value Over Replacement (Player) — points above whoever would be freely available at that position once starter demand is filled.'],
              ['Shrinkage', 'Pulling a small-sample estimate toward a population mean, weighted by how much evidence backs it.'],
              ['SFLEX', 'Superflex — a roster slot any of QB/RB/WR/TE can fill; QB dominates it in points leagues because of the raw scoring gap.'],
              ['ADP', 'Average Draft Position — where a player actually gets picked across real drafts, one form of "the market’s" opinion.'],
              ['z-score', '(value − mean) / stdev — how many standard deviations from average, comparable across very different stats.'],
              ['Washed out', 'A comp who never had a qualifying season after the match point — counted as zero growth, not excluded, to avoid survivorship bias.'],
            ].map(([term, def]) => (
              <div key={term}>
                <dt className="font-mono text-sm font-semibold text-primary">{term}</dt>
                <dd className="text-sm text-muted-foreground mt-0.5">{def}</dd>
              </div>
            ))}
          </dl>

          <p className="mt-10 pt-6 border-t border-border text-xs text-muted-foreground">
            Sources: <span className="font-mono">docs/projection-algorithm.md</span>,{' '}
            <span className="font-mono">docs/ranking-algorithm.md</span>,{' '}
            <span className="font-mono">docs/algorithm-review.md</span>,{' '}
            <span className="font-mono">docs/stats/*.md</span>. This page summarizes;
            it doesn't replace those docs — assumptions, tradeoffs and validation plans
            are spelled out there in full.
          </p>
        </section>
      </div>
    </div>
  )
}
