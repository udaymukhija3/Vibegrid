# Runbook: deploy and rollback

## How a change reaches production

1. Push to `main` (via PR or directly).
2. GitHub Actions runs **CI** (`.github/workflows/ci.yml`): Go race tests against Postgres, vuln scans, security contract, lint, typecheck, unit tests, build.
3. On CI success, **deploy** (`.github/workflows/deploy.yml`) POSTs the Render deploy hook.
4. Render builds the `Dockerfile` and swaps traffic only after `/readyz` passes (`render.yaml` `healthCheckPath`). Migrations apply on boot (`VIBEGRID_MIGRATE_ON_BOOT=true`).

`autoDeploy` is **off** in `render.yaml` — a red main never deploys. This requires the `RENDER_DEPLOY_HOOK_URL` repo secret (Render dashboard → service → Settings → Deploy Hook).

## Verify a deploy

```sh
VIBEGRID_BASE_URL=https://vibegrid.onrender.com npm run smoke:deploy
```

Also spot-check `https://vibegrid.onrender.com/api/puzzles/today` returns 200.

## Rollback

Pick one, in order of preference:

1. **Render rollback (fastest, no git churn):** Render dashboard → service → Deploys → previous successful deploy → "Rollback to this deploy". Use when the bad change is the latest deploy and you need production healthy *now*.
2. **Git revert (durable):** `git revert <bad-sha> && git push` — goes back through the CI gate. Use when you know the offending commit and can wait ~5 min for CI.

After either, re-run the smoke check above.

## Migration policy (what makes rollback safe)

Migrations are goose, forward-only, and must stay **additive** (new tables/columns/indexes only — never drop or rename something the previous release reads). That way old code always runs against new schema, and a code rollback never needs a schema rollback. If a destructive migration is ever unavoidable, take a manual backup first (run the **backup** workflow via workflow_dispatch) and split it across two releases: release N stops using the object, release N+1 drops it.
