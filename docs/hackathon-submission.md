# Qloo Agentic Hackathon — submission packet

Source: [Qloo Agentic Hackathon on Devpost](https://qloo.devpost.com/) (deadline Oct 30, 2026 @ 11:45pm EDT).

Product name, tagline, and Qloo story: **`docs/customer-acquisition-advisor.md`** (canonical).

## Required deliverables (checklist)

| # | Requirement | Status / link |
|---|-------------|---------------|
| 1 | **Live demo URL** (working end-to-end) | |
| 2 | **Public repo** (GitHub/GitLab/Bitbucket) | |
| 3 | **Text description** (what it does, Qloo-powered) | See **Devpost copy** below |
| 4 | **Externally hosted** (not local-only) | Render web service — see `docs/render-deployment.md` |
| 5 | **Open-source license** visible in repo About | `LICENSE` (MIT) at repo root |

Demo video: **not required**.

---

## Devpost copy (draft — fill blanks before submit)

### Project name

Customer Acquisition Advisor

### Tagline (≤140 characters)

CAA finds the business next door whose customers already love what you sell, so you grow through them instead of through ads.

### Thumbnail

*(empty — add screenshot or branded image)*

### Story — what it does

**Customer Acquisition Advisor (CAA)** helps local shop owners find **nearby businesses in other categories** whose customers share the same tastes as their best buyers. The owner picks a demo shop (or, in production, connects sales data). CAA reads valuable-customer purchases and area, maps menu lines to **Qloo taste tags**, calls **Qloo Insights** for ranked cross-category places, then uses an LLM only to write partnership ideas for places Qloo already returned. Results: real place names, why each fits, what to propose, next step, and contact details when Qloo includes them on the entity.

### Who it is for

Independent local shops (demo: coffee, salon, Mexican restaurant) that want **neighbor partnerships**, not more paid reach to strangers.

### How it is built

- **Go** API + **React** UI (single container on Render)
- **Qloo** Tags + Insights (affinity, explainability, geo radius)
- **OpenRouter** (or compatible LLM) for opportunity copy only
- Synthetic **scenario** datasets for the public demo (`USE_LIVE_CONNECTOR` off)

### Why it needs Qloo (not LLM-only)

Partner discovery is **Qloo Insights** on resolved taste tags and location; the LLM must use Qloo-returned `candidateId`s only. Without Qloo there is no cross-category ranked place list or affinity evidence—only generic “shops nearby.”

### Built with

Qloo, Go, React, TypeScript, OpenRouter (LLM)

### Try it out

*(empty — paste Render URL after deploy)*

### Screenshots

*(empty — product UI: business picker, wait flow, partner cards)*

### Additional info

- Public GitHub: *(empty)*
- Built for the **Qloo Agentic Hackathon**
- **Testing notes:** Render free tier may sleep after idle time; first load after sleep can take ~1 minute. Pick a demo business and start an analysis (~10–30 s typical). Multiple visitors can run at once. **Qloo runs every time.** **LLM** copy is skipped for a given demo scenario when a recent in-memory cache entry is still valid (server-configurable TTL via `ADVISOR_LLM_CACHE_TTL_SECONDS`; see README).

---

## Judging criteria (self-check)

| Criterion | Notes |
|-----------|--------|
| **Technological Implementation** | Live Qloo Tags + Insights; explainability stored; hackathon host |
| **Design** | Owner flow: pick business → progress → ranked partners → optional feedback |
| **Potential Impact** | Local cross-promotion instead of ad spend |
| **Quality of the Idea** | Taste graph for **partner** discovery, not same-category competitors |

---

## Repo hygiene before public push

- [ ] No `.env`, `outputs/`, or secrets in git
- [ ] `README.md` uses **Customer Acquisition Advisor** and points to `docs/customer-acquisition-advisor.md`
- [ ] `LICENSE` at root
- [ ] Non-demo paths archived locally; see `docs/submission-manual-cleanup.md`
