import { useParams, Link as RouterLink, useSearchParams } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { useLeagueCore } from './hooks/useLeagueCore'
import { StandingsTab } from './StandingsTab'
import { ScoreboardTab } from './ScoreboardTab'
import { PlayersTab } from './PlayersTab'
import { KeepersTab } from './KeepersTab'
import { DraftTab } from './DraftTab'

const SPORT_LABEL: Record<string, string> = {
  nfl: '🏈 NFL',
  nba: '🏀 NBA',
}

export default function LeagueDetail() {
  const { id } = useParams<{ id: string }>()
  const leagueId = Number(id)

  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') ?? 'standings'
  const setActiveTab = (tab: string) =>
    setSearchParams((prev) => { prev.set('tab', tab); return prev }, { replace: true })

  const { league, teams, yahooKeyToId, error } = useLeagueCore(leagueId)

  const myTeam = teams.find((t) => t.user_id)

  if (error) return <p className="text-red-600 dark:text-red-400">{(error as Error).message}</p>
  if (!league) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  return (
    <div className="max-w-6xl" style={{ '--tabs-height': '49px' } as React.CSSProperties}>
      {/* League header */}
      <div className="mb-6">
        <RouterLink to="/leagues" className="text-sm text-primary hover:underline">
          ← Leagues
        </RouterLink>
        <div className="flex items-center gap-4 mt-2">
          {league.logo_url && (
            <img src={league.logo_url} alt={league.name} className="h-14 w-14 rounded object-contain shrink-0" />
          )}
          <div>
            <h1 className="text-2xl font-bold text-foreground">{league.name}</h1>
            <p className="text-sm text-muted-foreground mt-1">
              {SPORT_LABEL[league.sport] ?? league.sport} · {league.season}
            </p>
          </div>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList className="mb-6">
          <TabsTrigger value="standings">Standings</TabsTrigger>
          <TabsTrigger value="scoreboard">Scoreboard</TabsTrigger>
          <TabsTrigger value="players">Players</TabsTrigger>
          <TabsTrigger value="keepers">Keepers</TabsTrigger>
          {league.sport === 'nfl' && <TabsTrigger value="draft">Draft</TabsTrigger>}
        </TabsList>

        <TabsContent value="standings">
          <StandingsTab leagueId={leagueId} active={activeTab === 'standings'} yahooKeyToId={yahooKeyToId} />
        </TabsContent>

        <TabsContent value="scoreboard">
          <ScoreboardTab leagueId={leagueId} active={activeTab === 'scoreboard'} yahooKeyToId={yahooKeyToId} />
        </TabsContent>

        <TabsContent value="players">
          <PlayersTab leagueId={leagueId} active={activeTab === 'players'} sport={league.sport} />
        </TabsContent>

        <TabsContent value="keepers">
          <KeepersTab leagueId={leagueId} active={activeTab === 'keepers'} teams={teams} myTeam={myTeam} season={league.season} />
        </TabsContent>

        {league.sport === 'nfl' && (
          <TabsContent value="draft">
            <DraftTab leagueId={leagueId} active={activeTab === 'draft'} season={league.season} />
          </TabsContent>
        )}
      </Tabs>
    </div>
  )
}
