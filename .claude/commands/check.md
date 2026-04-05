Do a quick health check of the codebase:

1. **Backend compile check:** `cd /Users/davidyoung/Projects/fantasy-sports/backend && go build ./...`
2. **Frontend type check:** `cd /Users/davidyoung/Projects/fantasy-sports/frontend && yarn build 2>&1`
3. **Backend tests:** `cd /Users/davidyoung/Projects/fantasy-sports/backend && go test ./... 2>&1`
4. **Lint:** `cd /Users/davidyoung/Projects/fantasy-sports/frontend && yarn lint 2>&1`

Report a summary table:
| Check | Status | Notes |
|-------|--------|-------|
| Go build | ✓/✗ | ... |
| TS build | ✓/✗ | ... |
| Go tests | ✓/✗ | ... |
| ESLint | ✓/✗ | ... |

List any errors in full so they can be fixed.
