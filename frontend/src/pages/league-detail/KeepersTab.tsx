import { Loader2 } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import type { Team } from '../../api/client'
import { submitKeepers, unsubmitKeepers, getKeeperSummary } from '../../api/client'
import { keys } from '../../api/queryKeys'
import { useKeepers, MAX_KEEPERS } from './hooks/useKeepers'
import { KeeperRulesBar } from './components/KeeperRulesBar'
import { KeeperDraftTable } from './components/KeeperDraftTable'
import { CommissionerKeeperView } from './components/CommissionerKeeperView'

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
  myTeam: Team | undefined
  season: string
}

/** Keeper management tab: rules editor, team selector, draft table with wishlist and season stats. */
export function KeepersTab({ leagueId, active, teams, myTeam, season }: Props) {
  const qc = useQueryClient()

  const {
    setKeeperTeamFilter,
    effectiveTeamFilter,
    rulesForm, setRulesForm,
    rulesMutation, addMutation, removeMutation,
    draftResults, wishlist, wishlistMap, designatedSet, visiblePicks,
    rankByPlayer,
    auctionByName,
  } = useKeepers(leagueId, myTeam, active, season)

  const showWishlistColumn = !!myTeam && effectiveTeamFilter === myTeam.yahoo_key
  const wishlistCount = wishlist?.length ?? 0
  const isCommissioner = myTeam?.is_commissioner ?? false

  // For non-commissioner team owners: check if they've already submitted.
  const { data: summary } = useQuery({
    queryKey: keys.keeperSummary(leagueId),
    queryFn: () => getKeeperSummary(leagueId),
    enabled: active && !!myTeam,
  })
  const myEntry = myTeam ? summary?.find((t) => t.team_id === myTeam.id) : undefined
  const mySubmitted = myEntry?.submitted ?? false

  const submitMutation = useMutation({
    mutationFn: () => submitKeepers(myTeam!.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.keeperSummary(leagueId) }),
  })
  const unsubmitMutation = useMutation({
    mutationFn: () => unsubmitKeepers(myTeam!.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.keeperSummary(leagueId) }),
  })

  return (
    <>
      <KeeperRulesBar
        rulesForm={rulesForm}
        onChange={setRulesForm}
        onSave={() => rulesMutation.mutate(rulesForm)}
        isPending={rulesMutation.isPending}
      />

      {/* Submit / Unsubmit button for team owners */}
      {myTeam && (
        <div className="flex items-center gap-3 mb-4">
          {mySubmitted ? (
            <>
              <span className="text-sm text-green-600 dark:text-green-400 font-medium">✓ Keepers submitted</span>
              <Button
                size="sm"
                variant="outline"
                disabled={unsubmitMutation.isPending}
                onClick={() => unsubmitMutation.mutate()}
              >
                {unsubmitMutation.isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Unsubmit'}
              </Button>
            </>
          ) : (
            <Button
              size="sm"
              disabled={submitMutation.isPending || wishlistCount === 0}
              onClick={() => submitMutation.mutate()}
            >
              {submitMutation.isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Submit Keepers'}
            </Button>
          )}
        </div>
      )}

      {/* Team selector + keeper count for your own team */}
      <div className="flex items-center gap-3 mb-4">
        <select
          value={effectiveTeamFilter}
          onChange={(e) => setKeeperTeamFilter(e.target.value)}
          className="h-8 rounded-md border border-input bg-background text-foreground px-2 py-1 text-sm cursor-pointer focus:outline-none focus:ring-2 focus:ring-ring"
        >
          <option value="">All Teams</option>
          {teams.map((t) => (
            <option key={t.id} value={t.yahoo_key ?? ''}>
              {t.name}{t.user_id ? ' (you)' : ''}
            </option>
          ))}
        </select>

        {myTeam && effectiveTeamFilter === myTeam.yahoo_key && (
          <span className={`text-sm font-medium ${wishlistCount >= MAX_KEEPERS ? 'text-orange-400' : 'text-muted-foreground'}`}>
            {wishlistCount} / {MAX_KEEPERS} keepers selected
          </span>
        )}
      </div>

      {!draftResults ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : draftResults.length === 0 ? (
        <p className="text-muted-foreground">No draft results available. Results appear after the league draft.</p>
      ) : (
        <KeeperDraftTable
          visiblePicks={visiblePicks}
          showTeamColumn={!effectiveTeamFilter}
          showWishlistColumn={showWishlistColumn}
          myTeamYahooKey={myTeam?.yahoo_key}
          wishlistMap={wishlistMap}
          designatedSet={designatedSet}
          wishlistCount={wishlistCount}
          isMutating={addMutation.isPending || removeMutation.isPending}
          rankByPlayer={rankByPlayer}
          auctionByName={auctionByName}
          onAdd={(playerKey, payload) => addMutation.mutate({ playerKey, payload })}
          onRemove={(playerKey) => removeMutation.mutate(playerKey)}
        />
      )}

      {/* Commissioner-only summary panel */}
      {isCommissioner && (
        <div className="mt-8 pt-6 border-t border-border">
          <CommissionerKeeperView leagueId={leagueId} myTeam={myTeam} active={active} />
        </div>
      )}
    </>
  )
}
