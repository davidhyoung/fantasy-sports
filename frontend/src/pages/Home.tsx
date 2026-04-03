import { Link as RouterLink } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { type User } from '../api/client'

interface Props {
  user: User | null
}

export default function Home({ user }: Props) {
  return (
    <div className="max-w-lg">
      <h1 className="text-2xl font-bold text-foreground">Welcome to Fantasy Sports</h1>

      {user ? (
        <p className="mt-2 text-muted-foreground">
          Hey, {user.display_name}! Head to{' '}
          <RouterLink to="/leagues" className="text-primary hover:underline">
            Leagues
          </RouterLink>{' '}
          to view your leagues.
        </p>
      ) : (
        <div className="mt-4">
          <p className="text-muted-foreground">Manage your leagues, teams, and rosters.</p>
          <Button asChild className="mt-4">
            <a href="/auth/login">Login with Yahoo to get started</a>
          </Button>
        </div>
      )}
    </div>
  )
}
