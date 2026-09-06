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
import { NativeFreeAgencyTab } from './NativeFreeAgencyTab'
import { NativeStandingsTab } from './NativeStandingsTab'
import { NativeScoreboardTab } from './NativeScoreboardTab'
import { DraftSection } from './DraftSection'
import { MessagesTab } from './MessagesTab'
import { ActivityRail } from './components/ActivityRail'
import { useLeagueFeed } from './hooks/useMessageBoard'

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
  const { data: feed } = useLeagueFeed(leagueId, isNative)

  // Native leagues have no separate My Team tab — Roster covers both "my
  // team" and "every team" in one page (a team switcher, defaulting to your
  // claimed team, replaces the need for a dedicated personal view). A
  // `?tab=my-team` deep link (the stable /leagues/:id/my-team redirect)
  // lands on Roster instead. For Yahoo leagues, My Team only exists once we
  // know you own a team here — wait for teams to load before falling back,
  // so the deep link isn't bounced before ownership is known.
  const visibleTab =
    isNative && activeTab === 'my-team'
      ? 'roster'
      : activeTab === 'my-team' && teams.length > 0 && !myTeam
      ? 'standings'
      : activeTab

  if (error) return <p className="text-destructive">{(error as Error).message}</p>
  if (!league) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  // --tabs-height matches TabsList: pt-4 (16) + track [p-1 + trigger 28 = 36] + pb-3 (12).
  return (
    <div className="max-w-6xl" style={{ '--tabs-height': '64px' } as React.CSSProperties}>
      {/* League header */}
      <div className="mb-6">
        <RouterLink to="/leagues" className="text-sm text-primary hover:underline print:hidden">
          ← Leagues
        </RouterLink>
        <div className="flex items-center gap-4 mt-2">
          {league.logo_url && (
            <img src={league.logo_url} alt={league.name} className="h-14 w-14 rounded object-contain shrink-0" />
          )}
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-bold text-foreground">{league.name}</h1>
              {isNative && <Badge variant="format">{league.format}</Badge>}
            </div>
            <p className="text-sm text-muted-foreground mt-1">
              {SPORT_LABEL[league.sport] ?? league.sport} · {league.season}
              {isNative && ' · Native'}
              {teams.length > 0 && ` · ${teams.length} team${teams.length === 1 ? '' : 's'}`}
            </p>
          </div>
        </div>
      </div>

      <Tabs value={visibleTab} onValueChange={setActiveTab}>
        <TabsList className="mb-6 print:hidden">
          {/* Native leagues fold "my team" into Roster (a team switcher
              defaulting to your claimed team) instead of a separate tab —
              the two were the same content, once Roster grew a matchup card
              and My Team grew commissioner tools. */}
          {myTeam && !isNative && <TabsTrigger value="my-team">My Team</TabsTrigger>}
          <TabsTrigger value="standings">Standings</TabsTrigger>
          {isNative && <TabsTrigger value="roster">Roster</TabsTrigger>}
          <TabsTrigger value="scoreboard">Scoreboard</TabsTrigger>
          <TabsTrigger value="players">Players</TabsTrigger>
          {isNative && <TabsTrigger value="free-agency">Free Agency</TabsTrigger>}
          {/* Draft covers draft values (NFL) + keepers (or picks, for native). */}
          <TabsTrigger value="draft">Draft</TabsTrigger>
          {isNative && (
            <TabsTrigger value="messages">
              Messages{feed && feed.unread_count > 0 ? ` (${feed.unread_count})` : ''}
            </TabsTrigger>
          )}
        </TabsList>

        {myTeam && !isNative && (
          <TabsContent value="my-team">
            <MyTeamTab myTeam={myTeam} />
          </TabsContent>
        )}

        <TabsContent value="standings">
          {isNative ? (
            <div className="lg:grid lg:grid-cols-[1fr_320px] lg:items-start lg:gap-6">
              <NativeStandingsTab leagueId={leagueId} active={visibleTab === 'standings'} myTeamId={myTeam?.id} />
              <div className="mt-6 lg:mt-0">
                <ActivityRail leagueId={leagueId} active={visibleTab === 'standings'} />
              </div>
            </div>
          ) : (
            <StandingsTab leagueId={leagueId} active={visibleTab === 'standings'} yahooKeyToId={yahooKeyToId} />
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
          <TabsContent value="free-agency">
            <NativeFreeAgencyTab leagueId={leagueId} active={visibleTab === 'free-agency'} teams={teams} myTeam={myTeam} />
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

        {isNative && (
          <TabsContent value="messages">
            <MessagesTab
              leagueId={leagueId}
              active={visibleTab === 'messages'}
              teams={teams}
              canModerate={myTeam?.is_commissioner ?? false}
            />
          </TabsContent>
        )}
      </Tabs>
    </div>
  )
}
