import { Link as RouterLink } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { listLeagues, sync } from '../api/client'
import { keys } from '../api/queryKeys'

const SPORT_LABEL: Record<string, string> = {
  nfl: '🏈 NFL',
  nba: '🏀 NBA',
}

export default function Leagues() {
  const qc = useQueryClient()
  const { data: leagues = [], error } = useQuery({
    queryKey: keys.leagues,
    queryFn: listLeagues,
  })

  const syncMutation = useMutation({
    mutationFn: sync,
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.leagues }),
  })

  return (
    <div className="max-w-3xl">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-xl font-bold text-foreground">Leagues</h1>
        <Button
          size="sm"
          onClick={() => syncMutation.mutate()}
          disabled={syncMutation.isPending}
        >
          {syncMutation.isPending ? 'Syncing…' : 'Sync from Yahoo'}
        </Button>
      </div>

      {(error || syncMutation.error) && (
        <p className="text-red-600 dark:text-red-400 text-sm mb-4">
          {(error as Error)?.message ?? (syncMutation.error as Error)?.message}
        </p>
      )}

      {leagues.length === 0 ? (
        <p className="text-muted-foreground">
          No leagues yet — click <strong>Sync from Yahoo</strong> to import yours.
        </p>
      ) : (
        <div className="flex flex-col gap-2">
          {leagues.map((l) => (
            <RouterLink key={l.id} to={`/leagues/${l.id}`}>
              <div className="bg-card rounded-lg border border-border/30 p-4 hover:bg-muted/30 transition-colors cursor-pointer flex items-center gap-4">
                {l.logo_url ? (
                  <img src={l.logo_url} alt={l.name} className="h-12 w-12 rounded object-contain shrink-0" />
                ) : (
                  <div className="h-12 w-12 rounded bg-muted flex items-center justify-center shrink-0 text-xl">
                    {SPORT_LABEL[l.sport]?.split(' ')[0] ?? '🏆'}
                  </div>
                )}
                <div>
                  <p className="font-semibold text-foreground">{l.name}</p>
                  <p className="text-sm text-muted-foreground mt-1">
                    {SPORT_LABEL[l.sport] ?? l.sport} · {l.season}
                  </p>
                </div>
              </div>
            </RouterLink>
          ))}
        </div>
      )}
    </div>
  )
}
