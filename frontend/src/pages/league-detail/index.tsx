import { useEffect } from 'react'
import { useParams, Link as RouterLink, useSearchParams } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { useLeagueCore } from './hooks/useLeagueCore'
import { MyTeamTab } from './MyTeamTab'
import { StandingsTab } from './StandingsTab'
import { ScoreboardTab } from './ScoreboardTab'
import { PlayersTab } from './PlayersTab'
import { NativePlayersTab } from './NativePlayersTab'
import { NativeRosterTab } from './NativeRosterTab'
import { NativeMyTeamTab } from './NativeMyTeamTab'
import { NativeStandingsTab } from './NativeStandingsTab'
import { NativeScoreboardTab } from './NativeScoreboardTab'
import { DraftSection } from './DraftSection'

const SPORT_LABEL: Record<string, string> = {
  nfl: '🏈 NFL',
  nba: '🏀 NBA',
}

export default function LeagueDetail() {
  const { id } = useParams<{ id: string }>()
  const leagueId = Number(id)

  const [searchParams, setSearchParams] = useSearchParams()
  const rawTab = searchParams.get('tab') ?? 'standings'
  // Keepers moved under Draft; old `?tab=keepers` links still land in the right place.
  const activeTab = rawTab === 'keepers' ? 'draft' : rawTab
  const setActiveTab = (tab: string) =>
    setSearchParams((prev) => { prev.set('tab', tab); return prev }, { replace: true })

  useEffect(() => {
    if (rawTab !== 'keepers') return
    setSearchParams((prev) => {
      prev.set('tab', 'draft')
      prev.set('sub', 'keepers')
      return prev
    }, { replace: true })
  }, [rawTab, setSearchParams])

  const { league, teams, yahooKeyToId, error } = useLeagueCore(leagueId)

  const myTeam = teams.find((t) => t.user_id)
  const isNative = league?.source === 'native'

  // The My Team tab only exists once we know you own a team here. Wait for teams
  // to load before falling back, so a `?tab=my-team` deep link isn't bounced.
  const visibleTab =
    activeTab === 'my-team' && teams.length > 0 && !myTeam
      ? 'standings'
      : activeTab

  if (error) return <p className="text-destructive">{(error as Error).message}</p>
  if (!league) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  // --tabs-height matches TabsList: pt-4 (16) + track [p-1 + trigger 28 = 36] + pb-3 (12).
  return (
    <div className="max-w-6xl" style={{ '--tabs-height': '64px' } as React.CSSProperties}>
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
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-bold text-foreground">{league.name}</h1>
              {isNative && <Badge variant="purple">{league.format}</Badge>}
            </div>
            <p className="text-sm text-muted-foreground mt-1">
              {SPORT_LABEL[league.sport] ?? league.sport} · {league.season}
              {isNative && ' · Native'}
            </p>
          </div>
        </div>
      </div>

      <Tabs value={visibleTab} onValueChange={setActiveTab}>
        <TabsList className="mb-6">
          {myTeam && <TabsTrigger value="my-team">My Team</TabsTrigger>}
          <TabsTrigger value="standings">Standings</TabsTrigger>
          <TabsTrigger value="scoreboard">Scoreboard</TabsTrigger>
          <TabsTrigger value="players">Players</TabsTrigger>
          {isNative && <TabsTrigger value="roster">Roster</TabsTrigger>}
          {/* Draft covers draft values (NFL) + keepers (or picks, for native). */}
          <TabsTrigger value="draft">Draft</TabsTrigger>
        </TabsList>

        {myTeam && (
          <TabsContent value="my-team">
            {isNative ? (
              <NativeMyTeamTab leagueId={leagueId} active={visibleTab === 'my-team'} myTeam={myTeam} />
            ) : (
              <MyTeamTab myTeam={myTeam} />
            )}
          </TabsContent>
        )}

        <TabsContent value="standings">
          {isNative ? (
            <NativeStandingsTab leagueId={leagueId} active={visibleTab === 'standings'} />
          ) : (
            <StandingsTab leagueId={leagueId} active={visibleTab === 'standings'} yahooKeyToId={yahooKeyToId} />
          )}
        </TabsContent>

        <TabsContent value="scoreboard">
          {isNative ? (
            <NativeScoreboardTab leagueId={leagueId} active={visibleTab === 'scoreboard'} />
          ) : (
            <ScoreboardTab leagueId={leagueId} active={visibleTab === 'scoreboard'} yahooKeyToId={yahooKeyToId} />
          )}
        </TabsContent>

        <TabsContent value="players">
          {isNative ? (
            <NativePlayersTab leagueId={leagueId} active={visibleTab === 'players'} teams={teams} />
          ) : (
            <PlayersTab leagueId={leagueId} active={visibleTab === 'players'} sport={league.sport} />
          )}
        </TabsContent>

        {isNative && (
          <TabsContent value="roster">
            <NativeRosterTab
              leagueId={leagueId}
              active={visibleTab === 'roster'}
              teams={teams}
              myTeam={myTeam}
              format={league.format}
            />
          </TabsContent>
        )}

        <TabsContent value="draft">
          <DraftSection
            leagueId={leagueId}
            active={visibleTab === 'draft'}
            sport={league.sport}
            season={league.season}
            teams={teams}
            myTeam={myTeam}
            isNative={isNative}
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}
