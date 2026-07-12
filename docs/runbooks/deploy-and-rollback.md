# Runbook: deploy and rollback

## How a change reaches production

1. Push to `main` (via PR or directly).
2. Render auto-deploys the push (`autoDeploy: true` in `render.yaml`): builds the `Dockerfile` and swaps traffic only after `/readyz` passes. Migrations apply on boot (`VIBEGRID_MIGRATE_ON_BOOT=true`).
3. In parallel, GitHub Actions runs **CI** (`.github/workflows/ci.yml`): Go race tests against Postgres, vuln scans, security contract, lint, typecheck, unit tests, build. A red CI does not block the deploy in this mode — check Actions after pushing.

**Optional upgrade — CI-gated deploys:** create a Deploy Hook (Render dashboard → service → Settings), save it as the `RENDER_DEPLOY_HOOK_URL` repo secret, and set `autoDeploy: false` in `render.yaml`. From then on `.github/workflows/deploy.yml` deploys only after CI passes, and a red main never reaches production.

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
