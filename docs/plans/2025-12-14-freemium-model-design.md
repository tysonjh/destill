# Freemium Model Design for Destill

**Date:** 2025-12-14
**Status:** Approved
**Philosophy:** Useful first, money generating second

---

## Executive Summary

Transform Destill into a freemium product where the free tier is genuinely useful (not artificially limited) and premium tier provides organizational value (team features, managed infrastructure, historical insights).

**Target Users:** Engineers at larger companies (like Redpanda) who struggle to sift through CI/CD logs to identify build failures quickly.

**Business Model:**
- **Free Tier:** Unlimited local analysis, self-hosted option, Claude MCP integration (open source MIT)
- **Premium SaaS:** Managed infrastructure + team features + historical analysis + custom tuning

---

## Architecture & Freemium Boundary

### Overall Architecture

```
┌─────────────────────────────────────────────────────────┐
│ FREE TIER (Open Source MIT)                             │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Local Mode (No Infrastructure)                         │
│  ┌──────────────────────────────────────────┐          │
│  │ destill analyze <build-url>              │          │
│  │  ├─ Buildkite API client                 │          │
│  │  ├─ GitHub Actions API client (NEW)      │          │
│  │  ├─ In-memory broker                     │          │
│  │  ├─ Ingest/Analyze agents (goroutines)   │          │
│  │  └─ TUI (streaming results)              │          │
│  └──────────────────────────────────────────┘          │
│                                                          │
│  Self-Hosted Mode (BYOI - Bring Your Own Infra)        │
│  ┌──────────────────────────────────────────┐          │
│  │ User runs: Redpanda + Postgres           │          │
│  │ User runs: destill-ingest, destill-analyze│         │
│  │ User runs: destill submit/view           │          │
│  │  └─ Persistent storage, team sharing     │          │
│  └──────────────────────────────────────────┘          │
│                                                          │
│  Claude MCP Integration (Local)                         │
│  ┌──────────────────────────────────────────┐          │
│  │ MCP Server: destill-mcp                  │          │
│  │  ├─ Tools: analyze_build, explain_error  │          │
│  │  ├─ Uses user's ANTHROPIC_API_KEY        │          │
│  │  └─ Integrates with Claude Desktop/Code  │          │
│  └──────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ PREMIUM TIER (SaaS - Managed)                           │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Managed Infrastructure (We Host)                       │
│  ┌──────────────────────────────────────────┐          │
│  │ Redpanda + Postgres (managed by Destill) │          │
│  │ API Gateway (authentication, rate limits) │          │
│  │ Web Dashboard + API                       │          │
│  └──────────────────────────────────────────┘          │
│                                                          │
│  Premium Features                                       │
│  ├─ 📊 Historical analysis (cross-build trends)        │
│  ├─ 🎯 Custom pattern tuning (org-specific scoring)    │
│  ├─ 👥 Team collaboration (shared findings, annotations)│
│  ├─ 🔔 Integrations (Slack, GitHub, webhooks)          │
│  ├─ 🤖 Hosted AI (better prompts, no user API key)     │
│  └─ 📈 Analytics (MTTR, failure categories, trends)    │
└─────────────────────────────────────────────────────────┘
```

### Key Design Decisions

1. **Free tier remains fully functional** - Not a limited trial, genuinely useful for individual engineers
2. **No artificial limits** - No rate limiting, no build caps, no feature gating on free tier
3. **Premium = organizational value** - Team features, managed infrastructure, historical insights
4. **MCP integration is free** - Engineers use their own API keys locally, premium offers better hosted AI with curated prompts

### Why This Model Works

- **Engineers adopt tools that work without friction** - Local + unlimited = instant adoption
- **Companies pay for team value, not individual value** - History, customization, collaboration justify $$
- **Open source builds community** - Contributors improve patterns, add CI platforms, report issues
- **Self-hosted addresses data sovereignty** - Companies can run it internally, but most will pay for managed SaaS convenience
- **MCP integration locally is differentiating** - Shows confidence in product, engineers will love you for it

**Premium pitch:** "Your team loves Destill locally? Get managed SaaS with team history, custom tuning, and collaboration features for $X/engineer/month."

---

## Phase 1: MVP Components

### 1. GitHub Actions Support (NEW)

**Component:** `src/githubactions/client.go` (new package)

**Purpose:** Fetch workflow run logs, job logs, and artifacts from GitHub API

**API Endpoints:**
- `GET /repos/{owner}/{repo}/actions/runs/{run_id}` - Run metadata
- `GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs` - List jobs
- `GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs` - Download job logs (returns redirect URL)
- `GET /repos/{owner}/{repo}/actions/artifacts/{artifact_id}/{archive_format}` - Download artifacts

**Authentication:** GitHub token (PAT or GitHub App)

**Challenges:** GitHub logs are zip archives, not raw text like Buildkite

### 2. Unified Ingest Interface (REFACTOR)

**Current:** `src/ingest/agent.go` is Buildkite-specific

**New:** Abstract `BuildProvider` interface:
```go
type BuildProvider interface {
    ParseURL(url string) (*BuildRef, error)
    FetchBuild(ctx context.Context, ref BuildRef) (*Build, error)
    FetchJobLog(ctx context.Context, jobID string) (string, error)
    FetchArtifacts(ctx context.Context, jobID string) ([]Artifact, error)
}
```

**Implementations:** `BuildkiteProvider`, `GitHubActionsProvider`

**Ingest agent** selects provider based on URL pattern

### 3. MCP Server (NEW)

**Component:** `cmd/destill-mcp/main.go` (new binary)

**Purpose:** MCP server that Claude Desktop/Code can connect to

**Tools exposed:**
- `analyze_build(url: string)` - Run analysis, return findings as structured data
- `explain_error(finding_id: string, context: object)` - Get AI explanation for specific finding

**Implementation:** Uses MCP SDK for Go (if available) or implement stdio protocol

**Runs:** Local analysis in-process (same as `destill analyze`)

### 4. URL Detection & Routing (NEW)

**Component:** `src/ingest/urlparser.go` (new)

**Purpose:** Detect CI platform from URL and route to appropriate provider

**Patterns:**
- `buildkite.com/{org}/{pipeline}/builds/{num}` → Buildkite
- `github.com/{owner}/{repo}/actions/runs/{id}` → GitHub Actions

**Error handling:** Clear error if URL pattern not recognized

---

## Code Organization

### Monorepo with Dual Licensing (Recommended)

**Structure:**
```
destill/                          # Single repo
├── LICENSE-MIT                   # For open source components
├── LICENSE-COMMERCIAL            # For premium components
├── src/                          # Shared library (MIT)
│   ├── broker/
│   ├── ingest/
│   ├── analyze/
│   ├── buildkite/
│   ├── githubactions/           # NEW
│   └── ...
├── cmd/                          # Free tier binaries (MIT)
│   ├── destill/                 # Local CLI
│   ├── destill-ingest/          # Self-hosted ingest agent
│   ├── destill-analyze/         # Self-hosted analyze agent
│   └── destill-mcp/             # MCP server (NEW)
├── premium/                      # Premium SaaS (Proprietary)
│   ├── api/                     # REST API gateway
│   ├── webapp/                  # Web dashboard
│   ├── historical/              # Cross-build analysis
│   ├── tuning/                  # Custom pattern engine
│   └── integrations/            # Slack, GitHub, webhooks
└── .gitignore                   # Ignore premium/ (or separate branch)
```

**How it works:**
- **Open source release:** Publish `src/` + `cmd/` to GitHub (MIT license)
- **Premium development:** Keep `premium/` in private repo or private branch
- **Shared code:** Premium imports from `src/` as internal packages
- **No library versioning needed:** Premium uses same codebase

**Rationale:**
- Early stage - still iterating, need fast refactoring
- Shared core - Premium heavily uses the analysis engine
- Simple workflow - Don't want overhead of library versioning yet
- Easy migration - Can split into separate repos later if needed

---

## Data Flow & User Workflows

### Free Tier - Local Mode Workflow

```
Engineer has failing build
        ↓
$ destill analyze "https://github.com/org/repo/actions/runs/123"
        ↓
[URL Parser] → Detect GitHub Actions
        ↓
[GitHubActionsProvider] → Fetch run metadata, jobs, logs (zip), artifacts
        ↓
[Ingest] → Unzip logs, chunk into 500KB pieces, parse JUnit XMLs
        ↓
[In-Memory Broker] → Publish chunks to local goroutines
        ↓
[Analyze Agents] → Pattern matching + confidence scoring (parallel)
        ↓
[TUI] → Stream findings in real-time, sorted by confidence
        ↓
Engineer investigates top finding → copies error → pastes into Claude Code
        ↓
**Enhanced with MCP (optional):**
Claude Code → MCP tool call → destill-mcp analyze_build(url)
        ↓
Returns structured findings → Claude explains in context
```

### Free Tier - Self-Hosted Mode Workflow

```
Company runs infrastructure (Redpanda + Postgres)
        ↓
Terminal 1: $ destill-ingest (subscribes to requests topic)
Terminal 2: $ destill-analyze (subscribes to logs topic)
        ↓
Engineer: $ destill submit "https://buildkite.com/..."
        ↓
[Request published to Redpanda] → Ingest picks up → Analyze processes
        ↓
[Findings → Postgres] → Persistent storage
        ↓
Engineer: $ destill view <request-id>
        ↓
[TUI loads from Postgres] → Can share request-id with teammates
```

### Premium SaaS - Managed Workflow (Future)

```
Engineer authenticates: $ destill login
        ↓
$ destill analyze "https://github.com/org/repo/actions/runs/123"
        ↓
[CLI] → POST to api.destill.io/v1/analyze
        ↓
[API Gateway] → Auth check → Publish to managed Redpanda
        ↓
[Managed agents] → Process build (same as self-hosted)
        ↓
[Findings → Managed Postgres + Historical DB]
        ↓
[API] → Return findings + historical context:
  - "This error appeared 3 times in last 7 days"
  - "First seen: 2025-12-10 (your commit abc123)"
  - "Similar failures: PR #456, PR #789"
        ↓
Engineer: "Did I cause this?" → API: "No, existed since Dec 10"
        ↓
**Premium AI (hosted):**
[API] → Claude with enriched context (history, similar failures, team notes)
        ↓
Returns: Root cause analysis + suggested fixes + related PRs
```

### Key Data Flow Differences

| Feature | Local (Free) | Self-Hosted (Free) | SaaS (Premium) |
|---------|--------------|-------------------|----------------|
| **Storage** | None (ephemeral) | Your Postgres | Our Postgres + History DB |
| **Broker** | In-memory | Your Redpanda | Our managed Redpanda |
| **History** | Single build only | Your DB retention | Unlimited, cross-build analysis |
| **AI Context** | Current build only | Current build only | History + team knowledge + patterns |
| **Sharing** | Copy/paste findings | Share request IDs | Web links + annotations |

---

## Phase 1 Feature Prioritization

**Goal:** Get engineers at companies like Redpanda using Destill locally as their go-to build triage tool.

### Critical Path (Build in this order)

#### Priority 1: GitHub Actions Support (MUST HAVE)

**Why first:** Required for broader adoption beyond Buildkite users

**Scope:**
- GitHub Actions API client
- Log fetching (handle zip archives)
- Artifact downloading
- URL parsing/routing

**Success metric:** Can analyze any GitHub Actions build URL

**Estimated complexity:** Medium (API is well-documented, but zip handling adds complexity)

#### Priority 2: Polish Local Mode UX (MUST HAVE)

**Why:** First impression matters - if local mode feels broken, no adoption

**Scope:**
- Better error messages (API auth failures, invalid URLs, network errors)
- Progress indicators (downloading logs, processing chunks, analyzing)
- Handle edge cases (private repos, expired logs, huge builds)
- Installation simplicity (brew, apt, or single binary download)

**Success metric:** New user can analyze their first build in < 2 minutes

**Estimated complexity:** Low-Medium (polish, not new features)

#### Priority 3: MCP Server for Claude Integration (HIGH VALUE)

**Why:** Differentiating feature, shows the vision

**Scope:**
- Implement MCP stdio protocol
- `analyze_build` tool (runs local analysis, returns structured findings)
- `explain_error` tool (optional: pass finding to Claude for explanation)
- Claude Desktop/Code configuration guide

**Success metric:** Engineers can say "analyze this build" in Claude and get results

**Estimated complexity:** Medium (depends on MCP SDK maturity for Go)

#### Priority 4: Documentation & Onboarding (MUST HAVE)

**Why:** Open source succeeds with great docs

**Scope:**
- README with quick start (5 commands to first analysis)
- API token setup guides (Buildkite, GitHub)
- MCP integration tutorial
- Architecture docs (for contributors)
- Video demo (2 min: paste URL → see results)

**Success metric:** New user doesn't need to ask questions

**Estimated complexity:** Low (writing, not coding)

#### Priority 5: Improve Confidence Scoring (NICE TO HAVE - DEFER)

**Why later:** Current recall is good, precision improvements require user feedback

**Approach:** Ship MVP → collect feedback → iterate based on real false positives

**Don't over-engineer:** Resist urge to add ML or complex scoring before validating the model

### Phase 1 MVP Definition

**Included:**
- ✅ Buildkite support (already works)
- ✅ GitHub Actions support (new)
- ✅ Local mode polished (UX improvements)
- ✅ MCP server (Claude integration)
- ✅ Great documentation
- ✅ Single binary distribution

**Deferred:**
- ❌ Premium features (defer to Phase 2)
- ❌ Confidence scoring improvements (based on feedback)

**Timeline Philosophy:** Ship useful → get feedback → iterate. Don't build premium until free tier proves valuable.

---

## Success Metrics

### Phase 1 Success Metrics

#### Adoption (Free Tier)

- 📊 GitHub stars (visibility)
- 📊 Weekly active users (telemetry opt-in: count of `destill analyze` runs)
- 📊 Supported CI platforms used (Buildkite vs. GitHub Actions split)
- 📊 MCP server adoption (engineers using Claude integration)

#### Product-Market Fit Signals

- ✅ Engineers report: "This saved me 20+ minutes finding the failure"
- ✅ Teams adopt it as standard practice ("Check Destill before asking in Slack")
- ✅ Feature requests indicate premium need: "Can we save history?" "Can my team see this?"
- ✅ Companies ask: "Can we self-host?" or "Do you have a paid version?"

#### Quality Metrics

- 📊 False positive rate (findings marked as irrelevant by users - need feedback mechanism)
- 📊 Top finding accuracy (is #1 confidence finding actually the root cause?)
- 📊 Time to first finding (how fast does TUI show results)

---

## Phase 2 Preview (Premium SaaS)

**When to start Phase 2:** After 100+ weekly active users and clear feedback that free tier is useful.

### Premium Features (Priority Order)

#### P0: Historical Analysis

- Cross-build comparison: "Did I cause this or was it already broken?"
- Failure trends: "This test has been flaky for 2 weeks"
- MTTR tracking: "Our average time to resolve build failures"

#### P1: Hosted AI with Better Context

- Root cause analysis with historical context
- Suggested fixes based on similar failures in your org
- No API key required (we handle Claude API costs)

#### P2: Team Collaboration

- Shared findings with annotations ("Known issue, ignore" or "Investigating with @alice")
- Knowledge base: "This error means X, fix by doing Y"
- Team dashboard: Who's investigating what

#### P3: Custom Pattern Tuning

- Org-specific confidence scoring (learn from your feedback)
- Custom severity rules ("Treat NPM audit warnings as errors")
- Pattern library sharing across team

#### P4: Integrations

- GitHub PR comments: "Build failed, here's the likely culprit"
- Slack notifications: "@channel Build failed, top 3 findings attached"
- Webhooks for CI/CD orchestration

### Premium Architecture (High-Level)

```
Engineer: $ destill analyze <url> --premium
        ↓
[API Gateway] → Auth (JWT) → Rate limit check → Usage tracking
        ↓
[Analysis Service] → Run same ingest/analyze (reuse open source code)
        ↓
[Historical Service] → Query past builds → Find first occurrence → Blame analysis
        ↓
[AI Service] → Enrich prompt with history → Claude API → Return insights
        ↓
[Response] → Findings + "First seen: 2025-12-10 (commit abc123, PR #456)"
              + AI explanation + suggested fix
```

### Premium Pricing Model (Suggestion)

- **Free:** Unlimited local analysis, self-hosted option
- **Team ($50/engineer/month):** Managed SaaS, 90-day history, basic AI
- **Enterprise ($custom):** Unlimited history, advanced AI, custom tuning, dedicated support, BYOC option

---

## Summary

This freemium model prioritizes **usefulness over monetization in Phase 1**, building trust and adoption through a genuinely helpful free tier. Premium features focus on **organizational value** (team collaboration, historical insights, managed infrastructure) rather than artificial limitations.

**Key Success Factors:**
1. Free tier must solve the core problem (find build failures quickly)
2. No artificial limits that frustrate users
3. Premium provides clear team/org value
4. Open source builds community and trust
5. Ship → feedback → iterate (don't over-engineer)

**Next Steps:** Implement Phase 1 MVP, measure adoption, gather feedback, validate premium assumptions.
