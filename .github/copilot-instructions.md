# Repository guidance

## Stack and commands

- Backend: Go 1.24+, `net/http`, `database/sql`, and SQLite through the sole third-party dependency `github.com/mattn/go-sqlite3` (cgo required).
- Frontend: self-written HTML, CSS, and plain JavaScript in `web/`; do not add frontend frameworks or other application libraries.
- Run the app with `go run .`. Set `KOMPAGNON_DB` or `PORT` to override defaults.
- Check changes with `gofmt -w *.go`, `go vet ./...`, `go test ./...`, `go build ./...`, `node --test web/*.test.js`, and `node --check web/app.js`.
- Run one Go test with `go test -run TestName -v`; run one JS test file with `node --test web/app-core.test.js`.

## Architecture

- `main.go` opens SQLite, migrates the schema, embeds `web/`, and starts the HTTP server.
- `db.go` owns the SQLite schema. `auth.go` handles account creation, PBKDF2 password derivation, database-backed sessions, and household invites. `app.go` contains authenticated cooking API handlers.
- `web/` contains the PWA shell and UI. The service worker caches static app assets only; keep `/api/` responses network-only.
- A user belongs to one household. Recipes, meal plans, and shopping lists are household-shared. Invitations are single-use and expire after seven days.

## Repository-specific invariants

- Register and login are the only unauthenticated API endpoints. Add new private API routes through `requireAuth`.
- Scope every household-owned database read, update, and delete to the authenticated account's `HouseholdID`; never trust a household ID supplied by the browser.
- Keep recipe ingredients in `recipe_ingredients` and write a recipe plus its ingredients in one transaction. Planned dates and shopping weeks use `YYYY-MM-DD`, with weeks starting Monday.
- Shopping generation combines numeric amounts only for matching normalized ingredient names and units. Meal-plan and recipe changes refresh affected weeks transactionally; preserve checked generated items that still match and all manually added items.
- Render user-controlled browser content through `escapeHTML` in `web/app-core.js`; do not interpolate raw user input into HTML.
- Keep auth and app functionality on Go's standard library. The SQLite driver is the only allowed third-party dependency; do not hand-roll cryptographic algorithms.
