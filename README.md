# Kompagnon

Kompagnon helps households keep everyday cooking in one place: recipes, a weekly meal plan, and a shared shopping list. The first release focuses on cooking; chores, appointments, and budgeting are future product areas.

## Run locally

Requirements: Go 1.24 or later, Node.js (for JavaScript tests), and a C compiler for the SQLite driver.

```sh
go run .
```

Open <http://localhost:8080>. The app creates `kompagnon.db` in the working directory and applies its schema on startup.

Configuration:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port |
| `KOMPAGNON_DB` | `kompagnon.db` | SQLite database path |
| `KOMPAGNON_COOKIE_SECURE` | `false` | Set to `true` when served over HTTPS |

The SQLite driver uses cgo. The app uses Go's standard library for HTTP, JSON, password derivation, and session handling; the SQLite driver is the only third-party dependency. The web client is handwritten HTML, CSS, and JavaScript with no frontend dependencies.

## Build and checks

```sh
gofmt -w *.go
go vet ./...
go test ./...
go build ./...
node --test web/*.test.js
node --check web/app.js
node --check web/service-worker.js
```

Run one Go test by name:

```sh
go test -run TestHouseholdInvitationSharesCookingData -v
```

Run the JavaScript unit tests by file:

```sh
node --test web/app-core.test.js
```

## How it fits together

- `main.go` opens SQLite, applies the schema, embeds `web/` into the executable, and starts the HTTP server.
- `auth.go` handles registration, sign-in, household invitations, and database-backed sessions. A new account creates a household; a single-use invitation lets another account join it.
- `app.go` serves the authenticated recipe, meal-plan, and shopping-list JSON endpoints. Every household-owned data query is scoped to the signed-in member's household.
- `db.go` defines the SQLite schema. Recipe ingredients are separate rows; meal plans use ISO calendar dates; shopping lists are grouped by Monday-starting week.
- `web/` is a mobile-first PWA. The service worker caches only the static shell; authenticated API responses remain network-only.

Shopping lists combine numeric amounts when ingredient names and units match, and scale quantities to planned servings. Free-form amounts (such as “to taste”) are preserved as notes. Meal-plan and recipe edits refresh generated items for affected weeks, preserving checked items that still match and keeping items added manually.

## Data and deployment notes

Each user belongs to one household in this MVP. Household invitations expire after seven days and are single-use. Passwords are derived with PBKDF2-HMAC-SHA256 from Go's standard library; session tokens are random, stored hashed in SQLite, and sent using HttpOnly, SameSite cookies. Serve production traffic over HTTPS and set `KOMPAGNON_COOKIE_SECURE=true`. Back up the SQLite database file to preserve household data.
