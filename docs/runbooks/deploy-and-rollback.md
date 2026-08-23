# Runbook: deploy and rollback

## How a change reaches production

1. Merge to `main` through a branch-protected PR.
2. GitHub Actions runs CI: Go race tests against Postgres, security/vulnerability
   checks, lint, typecheck, unit tests, and build.
3. After green CI, `.github/workflows/deploy.yml` calls the Render Deploy Hook.
   `render.yaml` has `autoDeploy: false`, so a red main cannot deploy.
4. Render builds the Dockerfile, applies boot migrations on the single free
   instance, and swaps traffic after `/readyz` passes.

`RENDER_DEPLOY_HOOK_URL` must be configured. If it is absent, the workflow warns
and no deployment occurs; this is safer than silently bypassing CI.

## Verify a deploy

```sh
VIBEGRID_BASE_URL=https://vibegrid.onrender.com npm run smoke:deploy
```

Also spot-check `https://vibegrid.onrender.com/api/vibes/today` returns one
prompt and exactly 16 fragments in four columns, then run the mutating smoke against a
database-backed environment to prove card replay.

## Rollback

Pick one, in order of preference:

1. **Render rollback (fastest, no git churn):** Render dashboard → service → Deploys → previous successful deploy → "Rollback to this deploy". Use when the bad change is the latest deploy and you need production healthy *now*.
2. **Git revert (durable):** `git revert <bad-sha> && git push` — goes back through the CI gate. Use when you know the offending commit and can wait ~5 min for CI.

After either, re-run the smoke check above.

## Migration policy (what makes rollback safe)

Migrations are goose, forward-only, and must stay **additive** (new
tables/columns/indexes only—never drop or rename something the previous release
reads). Migration `00019` follows that rule: the original 12 fragments remain in
`vibe_daily_boards`, while expansions and crew-size snapshots live in new
tables. The prior binary can boot against the migrated schema. However, once a
card selects an expansion fragment, rolling back would render that fragment
without text in the old UI; use a roll-forward unless the database is confirmed
to contain no such cards. If a destructive migration is ever unavoidable, take
a manual backup first and split it across two releases: release N stops using
the object, release N+1 drops it.
