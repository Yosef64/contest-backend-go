# Victory API (v1)

Go + Gin + Postgres backend for the Victory admin dashboard and the Victory
Flutter app. Replaces the DynamoDB backend used by the retired Telegram mini app.

## Run locally

```sh
cp .env.example .env          # set DATABASE_URL and BOOTSTRAP_ADMIN_*
go run ./cmd/api              # migrates, creates the owner admin, serves :8080
go run ./cmd/seed             # optional: starter courses, chapters, questions (empty DB only)
```

Migrations are embedded SQL (`internal/db/migrations`) applied on start under
a Postgres advisory lock; `go run ./cmd/migrate` applies them without serving.

## Layout

| Path | Purpose |
|---|---|
| `cmd/api` | HTTP server, graceful shutdown, owner bootstrap |
| `cmd/seed` | Seeds an empty database from `seed.json` |
| `cmd/import-dynamo` | One-off, read-only DynamoDB → Postgres import (below) |
| `internal/api` | Routes and handlers (`/api/v1/admin/*`, `/api/v1/*` for the app) |
| `internal/auth` | JWT, bcrypt, Google ID token and Telegram login verification |
| `internal/httpx` | Error envelope `{error:{code,message,fields}}`, request binding |
| `internal/store` | SQL (pgx); maps unique/FK violations to `ErrDuplicate`/`ErrInUse` |
| `internal/content` | HTML sanitizer for lesson bodies |
| `internal/media` | Image/video upload (Cloudinary or local) |

## Auth

- **Admins:** email + password → httpOnly `victory_admin` cookie (12 h). New
  admins must be approved by the owner; approval is re-checked on every request.
- **Students:** a Victory account (`POST /api/v1/auth/register` with name,
  email or Ethiopian phone, and password; `POST /api/v1/auth/login`), or
  `POST /api/v1/auth/google` (ID token) / `/auth/telegram` (Login Widget
  payload) → Bearer token (30 days). Logins are normalised (lower-case email,
  `+251…` phone); passwords need 8+ characters with a letter and a number and
  are bcrypt-hashed. Failed sign-ins are limited per login (8) and per address
  (30) for 15 minutes. `PUT /api/v1/me/password` changes the password. In
  development only, `POST /api/v1/auth/dev` issues a token without a provider.
- **Registration:** the first `PUT /api/v1/me` must include `department`
  (`entrance`, `remedial`, `freshman`) and `stream` (`natural`, `social`).

## Rules enforced server-side

- Contest questions are sent without answers; scoring happens on submit
  (one submission per student, 60 s grace after the end).
- Scores and the editorial are hidden until the contest ends.
- Chapter saves carry `base_version`; a stale save returns 409. Editing takes
  a 2-minute lock (423 for other admins). Every save is kept as a version.
- Lesson HTML is sanitized (no scripts or event handlers; YouTube iframes,
  `<video>`, inline SVG and lesson classes allowed).

## Public pages

- `GET /api/v1/site`: About text and official channels for the app (edited in
  the admin under Settings → Site & channels; `PUT /api/v1/admin/settings/site`).
- `GET /r/{student}/{chapter}`: landing page behind referral invite links. It
  opens the app (`victory://referral?…`), where the visit is recorded once the
  visitor is signed in; two distinct students unlock a referral-locked chapter.
  `GET /courses` reports each locked chapter's reason, progress and share link.

## Importing the old DynamoDB data

```sh
go run ./cmd/import-dynamo                 # dry run: counts + skipped rows
go run ./cmd/import-dynamo -apply          # write in one transaction
```

Uses the standard AWS credential chain (`AWS_PROFILE`, env vars). It only
scans DynamoDB and inserts with `ON CONFLICT DO NOTHING`, so it is safe to
re-run. Grades `1–4` → `9–12`, answers → 0-based, students keep their Telegram
ID, submissions are rebuilt from `score` + `missed_questions`, and admin
plaintext passwords are bcrypt-hashed (ask admins to change them afterwards).
Articles, comments, feedback, payments, notifications, achievements and page
views are retired and not imported.

## Checks

```sh
go vet ./... && go test ./...
python scripts/e2e_check.py   # against a running dev server; cleans up after itself
```
