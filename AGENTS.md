# AGENTS.md

Kubernetes operator syncing `Monitor`, `MaintenanceWindow` and `PublicStatusPage` resources (group `uptimerobot.kuehnleon.github.io`) with the UptimeRobot v3 API. Built with controller-runtime and kubebuilder conventions, delivered as a Helm chart.

## Principles

- Less is more. Keep code, docs and comments minimal. Prefer extending existing generic code over adding new paths.
- This repository is public. Never commit API keys, kubeconfigs, account IDs or other sensitive data, not even temporarily.
- The UptimeRobot API is heavily rate limited (FREE: 10 req/min). Every change must keep API calls independent of the number of resources.

## Layout

- `api/v1alpha1/`: CRD types with kubebuilder markers. `zz_generated.deepcopy.go` is generated.
- `internal/uptimerobot/`: API client with rate limiter and per-path list cache (`Get` is served from the cache; mutations update it).
- `internal/controller/sync.go`: generic `syncer[T]` used by all CRDs (finalizer, create/update/delete, hash and drift detection, status).
- `internal/controller/{monitor,maintenancewindow,publicstatuspage}.go`: per-kind request body mapping.
- `internal/controller/source.go`: Ingress / Traefik IngressRoute annotations → `Monitor`.
- `charts/uptimerobot-operator/`: Helm chart. `crds/` is generated; RBAC in `templates/rbac.yaml` is maintained by hand.

## Commands

```sh
make generate  # after any change in api/ (deepcopy + CRDs)
make test      # go vet, unit and envtest integration tests
golangci-lint run ./...
make helm-lint
```

CI fails if generated files are out of date.

## Conventions

- New UptimeRobot resource: add the type in `api/v1alpha1` (embed `ResourceStatus`, register in `groupversion_info.go`), add a `setupX` with a `syncer`, register it in `setup.go`, extend RBAC in the chart, add an example to `README.md`.
- Never call the API per resource on periodic reconciles; read through `uptimerobot.Client.Get`. Return all errors via `syncer.fail`, which backs off and suppresses retries on unrelated events.
- Status follows the Kubernetes API conventions: a single `Ready` condition (`setReady`, reasons in `sync.go`) plus events for remote changes and errors. Keep reasons CamelCase and stable, they are part of the API.
- Secrets are read uncached via `secretGetter` (operator only has `get` on secrets). Never write secret values to status, events or logs.
- Spec fields use Kubernetes naming; mapping to API field names happens in the per-kind `desired` functions.
- Changes go to `main` via squash-merged pull requests with Conventional Commit titles (`feat`, `fix`, `docs`, `build(deps)`, `chore`, `!` for breaking changes); `CHANGELOG.md` and release notes are generated from them by git-cliff (`cliff.toml`). Never edit `CHANGELOG.md` manually.
- Tests use the in-memory `fakeAPI` (`monitor_test.go`); assert on the API calls made.

## API references

- OpenAPI spec: https://cdn.uptimerobot.com/api/openapi.yaml (docs: https://uptimerobot.com/api/v3/)
- The official Terraform provider (https://github.com/uptimerobot/terraform-provider-uptimerobot) is the reference for undocumented behavior, e.g. `keywordCaseType` is sent as a number (0 = case sensitive, 1 = case insensitive), although the create schema documents strings.
