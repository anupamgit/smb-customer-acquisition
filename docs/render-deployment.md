# Render deployment plan

## Readiness snapshot

| Item | Ready? | Notes |
|------|--------|--------|
| HTTP API + static UI | Yes | `src/api/main.go` serves `static/` |
| Health check | Yes | `GET /api/health` |
| Scenario demo mode | Yes | Default; no Square on public demo |
| Env-based secrets | Yes | Never in repo; set in Render dashboard |
| Dockerfile | **Needs update** | Must ship `data/scenarios` + `data/qloo` inside image |
| `LICENSE` | **Missing** | Add MIT before Devpost |
| Postgres | Optional | In-memory store if `DATABASE_URL` unset |
| LLM cost control | Yes | Per-scenario cache (`ADVISOR_LLM_CACHE`, 5 min default) |

## Render-specific behavior

- **DDoS:** Cloudflare edge on all public web services (automatic).
- **App rate limits:** Not built into Render — optional future: limit `POST /api/runs` by `X-Forwarded-For`.
- **Free tier:** Service sleeps after inactivity; cold start ~1 min — note on Devpost.
- **Port:** Render sets `PORT`; app already uses `envOr("PORT", "8080")`.
- **Do not** bake secrets into the image; use Render **Environment** → Secret files or env vars.

## Recommended service type

**Web Service** (Docker), one instance for hackathon demo.

## Build & Dockerfile plan

Use repo-root **`Dockerfile`** (build context = repository root):

```bash
docker build -t advisor .
```

It builds the React app, Go binary, and copies **`data/`** into the image. Legacy `src/api/Dockerfile` remains for local builds when `static/` is prebuilt.

Example runtime layout:

```text
/app/advisor
/app/static/
/app/data/scenarios/...
/app/data/qloo/partner_discovery.json
```

## Render dashboard — environment variables

| Variable | Required | Value |
|----------|----------|--------|
| `QLOO_API_KEY` | Yes | Hackathon key |
| `QLOO_BASE_URL` | Yes | `https://hackathon.api.qloo.com` |
| `LLM_PROVIDER` | Yes | e.g. `openrouter` |
| `LLM_BASE_URL` | Yes | e.g. `https://openrouter.ai/api/v1` |
| `LLM_MODEL` | Yes | Your chosen model |
| `LLM_API_KEY` | Yes | Secret |
| `USE_LIVE_CONNECTOR` | No | **Unset** for demo |
| `SCENARIO` | No | Default `coffee-nyc` (UI picks scenario) |
| `ADVISOR_LLM_CACHE` | No | Default `1` |
| `ADVISOR_LLM_CACHE_TTL_SECONDS` | No | Default `300` |
| `QLOO_LOCATION_RADIUS_METERS` | No | Default `10000` |
| `DATABASE_URL` | No | Omit for in-memory runs |

Do **not** set `PORT` in the Dockerfile `ENV` for Render (Render injects `PORT`).

## Deploy steps

1. Fix Dockerfile + verify local: `docker build` → run → one scenario analysis.
2. Connect repo to Render; set root directory / Dockerfile path per chosen layout.
3. Add env vars; deploy.
4. Set health check path: `/api/health`.
5. Smoke test live URL (all four scenarios if time permits).

## Post-deploy

- Paste URL into Devpost **Try it out** and `docs/hackathon-submission.md`.
- Confirm repo is **public** and **LICENSE** shows in GitHub About.
