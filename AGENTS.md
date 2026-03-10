# dtingest — Agent Context

## Goal

`dtingest` is a CLI tool that makes it effortless to get Dynatrace observability deployed on any system. The core idea: a user should be able to run a single command, and the tool figures out what the best Dynatrace ingestion method is for their environment, then installs it.

## What we want to achieve

- **Zero guesswork for the user.** The tool analyzes the system (OS, container runtime, Kubernetes, cloud provider, existing agents) and recommends the right approach — whether that's the Dynatrace Operator on Kubernetes, OneAgent on a bare-metal host, a Docker-based agent, or an OpenTelemetry Collector.

- **A guided, interactive experience.** `dtingest setup` runs the full flow: analyze → recommend → pick → install. The user doesn't need to know which ingestion method to choose; the tool drives the decision.

- **Reuse dtctl for auth.** Authentication is fully delegated to `dtctl`. The user configures their Dynatrace environment once with `dtctl auth login` or `dtctl config set-context`, and `dtingest` picks it up automatically. No duplicated auth logic.

- **Clear, minimal output.** The CLI is opinionated about not overwhelming the user with information. The system analysis shows what was detected, recommendations are concise and actionable, and the installer guides the remaining steps.

- **Extensible installers.** Each ingestion method (OneAgent, Kubernetes Operator, Docker, OTel Collector, AWS CloudFormation) lives in its own installer module. Adding support for a new method should be straightforward.

## Key design decisions

| Decision | Rationale |
|---|---|
| Auth via dtctl | Avoids reimplementing OAuth PKCE, token refresh, OS keyring, and multi-context config |
| Analyze before recommend | Recommendations are grounded in what's actually on the system, not user input |
| Crisp recommendation output | Details (prerequisites, steps) belong in the installer, not the recommendation list |
| `MethodNotSupported` hidden from recommendations | Platform limitations (e.g. macOS) are noted inline in the analysis, not as a recommendation noise |
| **Prefer `dtctl` shell-out over direct API calls** | See below |

## Prefer dtctl over direct Dynatrace API calls

Whenever `dtingest` needs to query or interact with the Dynatrace platform, **prefer shelling out to `dtctl` over making direct HTTP calls**.

### Why

Direct API calls require managing:
- Which URL variant to use (classic `*.dynatracelabs.com` vs platform `*.apps.dynatracelabs.com`)
- Which token type is valid for the endpoint (OAuth `Bearer` vs classic `Api-Token`)
- Which scopes the token has (e.g. `storage:logs:read` for Grail DQL is only available on platform tokens, not the OAuth tokens `dtctl auth login` issues by default)

`dtctl` already handles all of this. If the user has authenticated and their context is configured correctly, `dtctl` will hit the right URL with the right token automatically.

### Concrete example: Grail log search

Logs ingested via the OTel Collector land in **Grail** storage, not the Classic log store. They are only queryable via DQL on the `.apps.` subdomain — **not** via `/api/v2/logs/search`. Attempting to query them directly requires:

1. Converting the env URL to the apps variant (e.g. `.dynatracelabs.com` → `.apps.dynatracelabs.com`)
2. Posting to `/platform/storage/query/v1/query:execute` with a JSON body
3. A token with `storage:logs:read` scope — which the default OAuth flow does **not** grant

Instead, `dtingest` shells out to `dtctl query`:

```go
out, err := exec.Command("dtctl", "query", "--plain", dqlQuery).Output()
if err == nil && strings.Contains(string(out), searchTerm) {
    // found
}
```

`dtctl query` picks up the active context automatically. The user authenticates once with:

```
dtctl auth login --context myenv-apps --environment https://myenv.apps.dynatracelabs.com
```

and everything works without `dtingest` needing to know about tokens or URL variants.

### Rule of thumb

- **Read/query operations** (logs, metrics, entities): shell out to `dtctl query` or other `dtctl` subcommands.
- **Write/ingest operations** (sending logs, metrics, traces): direct HTTP to the ingest endpoint is fine — those use simple API tokens with narrow ingest-only scopes that are already available.

## Releases

Releases are built and published with **GoReleaser** (`.goreleaser.yaml`). GoReleaser cross-compiles for all supported platforms, creates archives, and uploads them to the GitHub release.

### How to cut a release

```sh
git tag v0.x.y
git push origin v0.x.y
GITHUB_TOKEN=$(gh auth token) goreleaser release --clean
```

### Asset naming convention (dtingest)

Archives follow GoReleaser's default template:

```
dtingest_{version}_{os}_{arch}.tar.gz   # Linux / macOS
dtingest_{version}_{os}_{arch}.zip      # Windows
```

Examples: `dtingest_0.1.3_darwin_arm64.tar.gz`, `dtingest_0.1.3_linux_amd64.tar.gz`.

The install script (`scripts/install.sh`) constructs this name at runtime and downloads the matching asset from the GitHub release.

### Pitfall: tag exists but release has no assets

`git push --tags` (or the GitHub UI "draft release" flow) can create a lightweight GitHub release with an empty assets list. In that state `dtingest install otel-collector` — and the install script itself — will fail with 404 because there are no binaries to download.

**Fix:** run `goreleaser release --clean` against the existing tag. GoReleaser detects the already-created release and uploads the missing archives.

## Current state

The analyzer detects: platform/OS, container runtime (Docker), Kubernetes (with distribution and context), OneAgent, OTel Collector, AWS, Azure, and running services.

Installers are partially implemented. The recommendation and analysis engine is complete.
