# dtingest

**Dynatrace Ingest CLI** — analyzes your system and deploys the best Dynatrace observability method.

`dtingest` is a Go CLI that ports the Python `ingest-agent` to Go. It reuses **dtctl's entire authentication stack** (config loading, multi-context support, OAuth PKCE token refresh, OS keyring, and API token fallback) by importing `github.com/dynatrace-oss/dtctl` as a module dependency.

## Prerequisites

Configure your Dynatrace environment with [dtctl](https://github.com/dynatrace-oss/dtctl):

```bash
# Option 1 – OAuth login (recommended)
dtctl auth login

# Option 2 – API token
dtctl config set-context my-env \
  --environment https://abc12345.apps.dynatrace.com \
  --token dt0c01.XXXX...
```

## Installation

```bash
# From source
git clone https://github.com/dietermayrhofer/dt-clis.git
cd dt-clis/dtingest
make install
```

## Available commands

| Command | Description |
|---------|-------------|
| `dtingest analyze` | Detect platform, containers, K8s, existing agents, cloud, and services |
| `dtingest recommend` | Generate ranked ingestion recommendations |
| `dtingest setup` | Interactive analyze → recommend → install workflow |
| `dtingest install oneagent` | Install Dynatrace OneAgent on this host |
| `dtingest install kubernetes` | Deploy Dynatrace Operator on Kubernetes |
| `dtingest install docker` | Install OneAgent for Docker |
| `dtingest install otel-collector` | Install/configure OpenTelemetry Collector |
| `dtingest install aws` | Set up Dynatrace AWS CloudFormation integration |
| `dtingest status` | Show Dynatrace connection status and system state |

Use `--context <name>` on any command to override the active dtctl context.

## Example workflow

```bash
# 1. Authenticate via dtctl
dtctl auth login

# 2. Analyze the current system
dtingest analyze

# 3. Get ranked recommendations
dtingest recommend

# 4. Install the recommended method (e.g., Kubernetes)
dtingest install kubernetes

# 5. Check status
dtingest status
```

## JSON output

`analyze` and `recommend` support `--json` for structured output:

```bash
dtingest analyze --json | jq .platform
dtingest recommend --json | jq '.[0].method'
```

## Building

```bash
cd dtingest
make build        # builds ./dtingest binary
make test         # runs go test ./...
make install      # installs to $GOPATH/bin
make clean        # removes build artifacts
```

## Architecture

```
dtingest/
├── main.go
├── cmd/
│   ├── root.go       # Cobra root + --context flag
│   ├── auth.go       # dtctl auth bridge (loadDtctlConfig, newDtClient, getDtEnvironment)
│   ├── analyze.go
│   ├── recommend.go
│   ├── setup.go
│   ├── install.go
│   └── status.go
└── pkg/
    ├── analyzer/     # System detection (platform, Docker, K8s, agents, cloud, services)
    ├── recommender/  # Recommendation engine
    └── installer/    # Shared utilities + per-method stubs
```

Authentication is fully delegated to dtctl — `dtingest` never stores credentials itself.
