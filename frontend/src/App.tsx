import { Routes, Route, Navigate, useParams, useLocation, Link as RouterLink } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Sun, Moon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/components/ui/provider'
import Home from './pages/Home'
import LeagueDetail from './pages/league-detail'
import MyTeamRedirect from './pages/league-detail/MyTeamRedirect'
import TeamDetail from './pages/team-detail'
import MatchupDetail from './pages/matchup-detail'
import PlayerDetail from './pages/player-detail'
import Statistics from './pages/statistics'
import DraftPrep from './pages/draft-prep'
import Divergences from './pages/divergences'
import Wiki from './pages/wiki'
import { getMe } from './api/client'
import { keys } from './api/queryKeys'

function ProjectionRedirect() {
  const { gsisId } = useParams<{ gsisId: string }>()
  return <Navigate to={`/players/${gsisId}`} replace />
}

/** The only graphic in the system — four flat squares, no imagery anywhere else. */
function BrandMark() {
  return (
    <svg width="24" height="24" viewBox="0 0 26 26" aria-hidden>
      <rect x="2" y="2" width="9" height="9" className="fill-primary" />
      <rect x="15" y="2" width="9" height="9" className="fill-muted" />
      <rect x="2" y="15" width="9" height="9" className="fill-muted" />
      <rect x="15" y="15" width="9" height="9" className="fill-secondary" />
    </svg>
  )
}

/** `match` lists the path prefixes that should light up each nav segment. */
const NAV_ITEMS = [
  { to: '/', label: 'Leagues', match: ['/', '/leagues', '/teams'] },
  { to: '/draft-prep', label: 'Draft Prep', match: ['/draft-prep'] },
  { to: '/statistics', label: 'Statistics', match: ['/statistics', '/players', '/divergences'] },
  { to: '/wiki', label: 'Wiki', match: ['/wiki'] },
]

export default function App() {
  const location = useLocation()
  const { data: user, isLoading: authLoading } = useQuery({
    queryKey: keys.me,
    queryFn: getMe,
    retry: false,
  })
  const { theme, setTheme } = useTheme()
  const isDark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)

  return (
    <div className="min-h-screen bg-background">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:top-2 focus:left-2 focus:px-4 focus:py-2 focus:bg-primary focus:text-primary-foreground focus:rounded-md"
      >
        Skip to content
      </a>
      <nav className="sticky top-0 z-40 bg-card border-b border-border px-7 py-4 flex items-center justify-between">
        <RouterLink to="/" className="flex items-center gap-2.5">
          <BrandMark />
          <span className="font-display text-base font-bold text-foreground">Fantasy Sports</span>
        </RouterLink>

        <div className="flex items-center gap-2.5">
          {/* Leagues, the pre-draft board, and the player-data surface. */}
          <div className="flex items-center gap-0.5 rounded-pill bg-muted p-1">
            {NAV_ITEMS.map(({ to, label, match }) => {
              const active = match.some(p =>
                p === '/' ? location.pathname === '/' : location.pathname.startsWith(p)
              )
              return (
                <RouterLink
                  key={to}
                  to={to}
                  className={`rounded-pill px-3.5 py-1.5 font-display text-xs font-semibold ${
                    active
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:text-foreground'
                  }`}
                >
                  {label}
                </RouterLink>
              )
            })}
          </div>

          <button
            onClick={() => setTheme(isDark ? 'light' : 'dark')}
            className="flex h-[22px] w-[22px] items-center justify-center rounded-full border-[1.5px] border-input text-[11px] text-muted-foreground hover:text-foreground"
            aria-label="Toggle theme"
          >
            {isDark ? <Sun className="h-3 w-3" /> : <Moon className="h-3 w-3" />}
          </button>
          {authLoading ? null : user ? (
            <div className="flex items-center gap-3">
              <span className="font-display text-[13px] font-semibold text-foreground">
                {user.display_name}
              </span>
              <a href="/auth/logout" className="font-display text-[13px] text-muted-foreground hover:text-foreground">
                Logout
              </a>
            </div>
          ) : (
            <Button asChild size="sm">
              <a href="/auth/login">Login with Yahoo</a>
            </Button>
          )}
        </div>
      </nav>

      <main id="main-content" className="px-6 pt-8 pb-6">
        <Routes>
          <Route path="/" element={<Home user={user ?? null} />} />
          {/* The leagues list lives on the home screen now. */}
          <Route path="/leagues" element={<Navigate to="/" replace />} />
          <Route path="/leagues/:id" element={<LeagueDetail />} />
          {/* Stable deep link to your own team in a league → /teams/:teamId */}
          <Route path="/leagues/:id/my-team" element={<MyTeamRedirect />} />
          <Route path="/teams/:id" element={<TeamDetail />} />
          <Route path="/leagues/:leagueId/matchup/:week/:t1/:t2" element={<MatchupDetail />} />
          <Route path="/draft-prep" element={<DraftPrep />} />
          <Route path="/wiki" element={<Wiki />} />
          <Route path="/statistics" element={<Statistics />} />
          {/* Rankings and Projections merged into Statistics as two views. */}
          <Route path="/rankings" element={<Navigate to="/statistics?view=grades" replace />} />
          <Route path="/projections" element={<Navigate to="/statistics?view=projections" replace />} />
          {/* Kept as a deep page: Home surfaces the top signals, this is the full table. */}
          <Route path="/divergences" element={<Divergences />} />
          <Route path="/players/:gsisId" element={<PlayerDetail />} />
          {/* Legacy redirect — old /projections/:gsisId links now go to /players/:gsisId */}
          <Route path="/projections/:gsisId" element={<ProjectionRedirect />} />
        </Routes>
      </main>
    </div>
  )
}
