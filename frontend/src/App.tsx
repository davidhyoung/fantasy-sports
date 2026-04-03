import { Routes, Route, Navigate, useParams, useLocation, Link as RouterLink } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Sun, Moon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/components/ui/provider'
import Home from './pages/Home'
import Leagues from './pages/Leagues'
import LeagueDetail from './pages/LeagueDetail'
import TeamDetail from './pages/TeamDetail'
import MatchupDetail from './pages/MatchupDetail'
import PlayerDetail from './pages/player-detail'
import Projections from './pages/projections'
import Rankings from './pages/rankings'
import { getMe } from './api/client'
import { keys } from './api/queryKeys'

function ProjectionRedirect() {
  const { gsisId } = useParams<{ gsisId: string }>()
  return <Navigate to={`/players/${gsisId}`} replace />
}

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
      <nav className="sticky top-0 z-40 bg-background border-b border-border/40 px-6 pt-5 pb-3 flex items-center gap-6">
        <RouterLink
          to="/"
          className="font-semibold text-foreground hover:text-primary transition-colors"
        >
          Fantasy Sports
        </RouterLink>
        {[
          { to: '/leagues', label: 'Leagues' },
          { to: '/rankings', label: 'Rankings' },
          { to: '/projections', label: 'Projections' },
        ].map(({ to, label }) => (
          <RouterLink
            key={to}
            to={to}
            className={`transition-colors ${
              location.pathname.startsWith(to)
                ? 'text-foreground font-medium'
                : 'text-muted-foreground hover:text-primary'
            }`}
          >
            {label}
          </RouterLink>
        ))}
        <div className="ml-auto flex items-center gap-4">
          <button
            onClick={() => setTheme(isDark ? 'light' : 'dark')}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            aria-label="Toggle theme"
          >
            {isDark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
          {authLoading ? null : user ? (
            <div className="flex items-center gap-4">
              <span className="text-sm text-foreground">{user.display_name}</span>
              <a
                href="/auth/logout"
                className="text-sm text-muted-foreground hover:text-destructive transition-colors"
              >
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
          <Route path="/leagues" element={<Leagues />} />
          <Route path="/leagues/:id" element={<LeagueDetail />} />
          <Route path="/teams/:id" element={<TeamDetail />} />
          <Route path="/leagues/:leagueId/matchup/:week/:t1/:t2" element={<MatchupDetail />} />
          <Route path="/rankings" element={<Rankings />} />
          <Route path="/projections" element={<Projections />} />
          <Route path="/players/:gsisId" element={<PlayerDetail />} />
          {/* Legacy redirect — old /projections/:gsisId links now go to /players/:gsisId */}
          <Route path="/projections/:gsisId" element={<ProjectionRedirect />} />
        </Routes>
      </main>
    </div>
  )
}
