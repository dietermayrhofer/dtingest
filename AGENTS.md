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

## Current state

The analyzer detects: platform/OS, container runtime (Docker), Kubernetes (with distribution and context), OneAgent, OTel Collector, AWS, Azure, and running services.

Installers are partially implemented. The recommendation and analysis engine is complete.
