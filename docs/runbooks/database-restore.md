# Runbook: database restore

## Where backups should live

- **Nightly encrypted dumps:** once `BACKUP_DATABASE_URL` and
  `BACKUP_PASSPHRASE` are configured, GitHub → Actions → **backup** workflow →
  run → artifact `vibegrid-db-<run id>` (30-day retention). Encrypted because
  the repo is public; decrypting requires the `BACKUP_PASSPHRASE` secret value.
  As of the August 1, 2026 readiness review, the inspected scheduled run skipped
  backup creation and produced no artifact. Treat this path as unconfigured
  until a successful artifact and restore drill are recorded.
- **Neon point-in-time restore:** Neon console → project → Restore. Verify that
  PITR is enabled for the current plan and record the actual recovery window;
  configuration is not proven by this runbook.

An on-demand backup can be taken any time: Actions → backup → "Run workflow".

## Restore procedure

Never restore over the live database directly — restore to a fresh target, verify, then switch.

1. Download the artifact and decrypt:

   ```sh
   openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 \
     -in vibegrid.pgcustom.enc -out vibegrid.pgcustom \
     -pass env:BACKUP_PASSPHRASE
   ```

2. Create a fresh database (new Neon branch or new database in the project) and restore into it:

   ```sh
   pg_restore --clean --if-exists --no-owner --no-privileges \
     -d "$NEW_DATABASE_URL" vibegrid.pgcustom
   ```

3. Verify: row counts on `puzzles`, `attempts`, `attempt_guesses`; then point a local run at it (`DATABASE_URL=$NEW_DATABASE_URL npm run dev:backend`) and load the daily + an attempt.
4. Switch production: update `DATABASE_URL` in the Render dashboard → service redeploys → run the smoke check from the deploy runbook.

## Restore drill

Do steps 1–3 once per quarter (skip step 4). If the drill has never been done, the backups should be treated as unverified.

## Notes

- `pg_restore` major version must be ≥ the dump's server version (use the `postgres:17` Docker image if local tools are older).
- Rotating `BACKUP_PASSPHRASE` does not re-encrypt old artifacts — keep the old passphrase until those expire (30 days).
