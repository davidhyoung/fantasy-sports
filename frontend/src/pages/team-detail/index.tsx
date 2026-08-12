import { useParams, Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { useTeamDetail } from './hooks/useTeamDetail'
import { TeamPanel } from './components/TeamPanel'

export default function TeamDetail() {
  const { id } = useParams<{ id: string }>()
  const teamId = Number(id)

  const { team, error } = useTeamDetail(teamId)

  if (error) return <p className="text-destructive">{(error as Error).message}</p>
  if (!team) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  return (
    <div className="max-w-5xl">
      {/* Team header */}
      <div className="mb-6">
        <RouterLink to={`/leagues/${team.league_id}`} className="text-sm text-primary hover:underline">
          ← League
        </RouterLink>
        <div className="flex items-center gap-4 mt-2">
          {team.logo_url && (
            <img src={team.logo_url} alt={team.name} className="h-14 w-14 rounded object-contain shrink-0" />
          )}
          <div>
            <h1 className="text-2xl font-bold text-foreground">{team.name}</h1>
            {team.user_id && (
              <Badge variant="pink" className="mt-1">Your team</Badge>
            )}
          </div>
        </div>
      </div>

      <TeamPanel teamId={teamId} />
    </div>
  )
}
