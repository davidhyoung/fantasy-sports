import type { Poll } from '@/api/client'

interface Props {
  poll: Poll
  onVote: (optionId: number) => void
  voting: boolean
}

/**
 * Poll options as horizontal fill bars once a result should show — either
 * the viewer has voted, or the poll is configured to always show votes.
 * Before that, plain clickable option rows with no bar (a bar with no data
 * behind it yet would misread as "0% for everyone").
 */
export function PollBar({ poll, onVote, voting }: Props) {
  const showResults = poll.my_option_id != null || poll.votes_visible === 'always'
  const closed = poll.closes_at != null && new Date(poll.closes_at).getTime() < Date.now()

  return (
    <div className="mt-3 flex flex-col gap-1.5">
      {poll.options.map((opt) => {
        const pct = poll.total_votes > 0 ? Math.round((opt.votes / poll.total_votes) * 100) : 0
        const mine = poll.my_option_id === opt.id
        return (
          <button
            key={opt.id}
            type="button"
            disabled={voting || closed}
            onClick={() => onVote(opt.id)}
            className="relative overflow-hidden rounded-md border border-border px-3 py-2 text-left disabled:cursor-default"
          >
            {showResults && (
              <span
                aria-hidden
                className="absolute inset-y-0 left-0 bg-positive-light"
                style={{ width: `${pct}%` }}
              />
            )}
            <span className="relative flex items-center justify-between gap-3">
              <span className={`font-display text-[13px] font-semibold ${mine ? 'text-primary' : 'text-foreground'}`}>
                {opt.label}
                {mine && ' ✓'.replace('✓', '·')}
              </span>
              {showResults && (
                <span className="font-mono text-xs tabular-nums text-muted-foreground">
                  {pct}% ({opt.votes})
                </span>
              )}
            </span>
          </button>
        )
      })}
      <div className="mt-0.5 flex items-center justify-between text-[11px] text-muted-foreground">
        <span>{poll.total_votes} vote{poll.total_votes === 1 ? '' : 's'}</span>
        {closed ? <span>Poll closed</span> : poll.closes_at && (
          <span>Closes {new Date(poll.closes_at).toLocaleDateString()}</span>
        )}
      </div>
    </div>
  )
}
