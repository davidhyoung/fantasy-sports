const STAGES = [
  { num: '01', label: 'Grades', q: 'Is he actually good, independent of role?' },
  { num: '02', label: 'Projections', q: 'What will he produce next season?' },
  { num: '03', label: 'League value', q: "What's that worth in this league?" },
  { num: '04', label: 'Auction $', q: 'What do I actually pay?' },
  { num: '05', label: 'Tiers', q: "Where's the real cliff?" },
]

/**
 * The five-stage pipeline as a horizontal strip. Direction between stages is
 * a typographic arrow, not an icon, matching the rest of the design system.
 */
export default function Pipeline() {
  return (
    <div className="mt-6">
      <div className="flex gap-0 overflow-x-auto pb-2">
        {STAGES.map((s, i) => (
          <div key={s.num} className="flex flex-none items-center">
            <div className="w-[150px] flex-none rounded-lg border border-border bg-card px-3 py-2.5">
              <div className="font-mono text-[11px] text-primary">{s.num}</div>
              <div className="font-display text-[13px] font-semibold text-foreground mt-0.5">{s.label}</div>
              <div className="text-[11px] leading-snug text-muted-foreground mt-1">{s.q}</div>
            </div>
            {i < STAGES.length - 1 && (
              <div className="w-6 flex-none text-center text-muted-foreground/50 font-mono">→</div>
            )}
          </div>
        ))}
      </div>
      <div className="mt-2.5 max-w-2xl rounded-lg border border-dashed border-highlight-border bg-highlight-light px-3.5 py-2.5 text-[12px] leading-relaxed text-muted-foreground">
        <span className="font-display font-semibold text-highlight-foreground">Cross-checked throughout — </span>
        Consensus &amp; Divergence compares stage 02's ranks and stage 04's dollars against the
        outside market, and flags where the model is structurally blind (trades, injuries, camp buzz).
      </div>
    </div>
  )
}
