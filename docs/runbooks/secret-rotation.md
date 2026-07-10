# Runbook: secret rotation

All production secrets, where they live, and what rotating them breaks.

| Secret | Lives in | Rotate by | Blast radius |
|---|---|---|---|
| `DATABASE_URL` | Render env | Neon console → reset role password → update Render env (service restarts) → update `BACKUP_DATABASE_URL` GitHub secret | Brief restart; backups break if the GitHub copy is forgotten |
| `VIBEGRID_ADMIN_PASSWORD` | Render env | Generate long random value → update Render env | Admin must use the new password; existing sessions stay valid until expiry |
| `VIBEGRID_ADMIN_SESSION_SECRET` | Render env | Generate (`openssl rand -base64 32`) → update Render env | All live admin CSRF tokens invalidate — admins re-login |
| `VIBEGRID_ADMIN_TOKEN` (optional) | Render env | Same as above | Any scripts using bearer auth need the new token |
| `VIBEGRID_METRICS_TOKEN` | Render env | Generate → update Render env → update the scraper (Prometheus/Grafana config) | Metrics scrape 401s until scraper updated |
| `BACKUP_DATABASE_URL` | GitHub repo secret | Follows `DATABASE_URL` | Nightly backup fails loudly if stale |
| `BACKUP_PASSPHRASE` | GitHub repo secret | Generate → update secret; keep old value somewhere safe until 30-day-old artifacts expire | Old artifacts still need the old passphrase |
| `RENDER_DEPLOY_HOOK_URL` | GitHub repo secret | Render dashboard → Settings → regenerate deploy hook → update secret | Deploys fail loudly until updated |

## Rules

- Rotate immediately if a secret ever appears in a log, commit, screenshot, or shared terminal.
- Generate with `openssl rand -base64 32` — never reuse across secrets.
- After any rotation, run the deploy smoke check and (if DB-related) trigger the backup workflow manually to confirm it still passes.
- Nothing here is ever committed: `render.yaml` marks them `sync: false`, `.env.example` holds placeholders only.
