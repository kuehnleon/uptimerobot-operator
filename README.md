# uptimerobot-operator

Kubernetes operator managing [UptimeRobot](https://uptimerobot.com) monitors, maintenance windows and public status pages, using the [UptimeRobot v3 API](https://uptimerobot.com/api/v3/).

- `Monitor`, `MaintenanceWindow` and `PublicStatusPage` custom resources
- Monitors for annotated `Ingress` and Traefik `IngressRoute` resources
- Built for API rate limits: client-side rate limiting and one cached list request per resource type and sync interval, independent of the number of resources

## Installation

Create an account-specific (main) API key in the [UptimeRobot dashboard](https://dashboard.uptimerobot.com/integrations), then:

```sh
kubectl create namespace uptimerobot
kubectl -n uptimerobot create secret generic uptimerobot --from-literal=api-key=<API_KEY>
helm install uptimerobot-operator oci://ghcr.io/kuehnleon/charts/uptimerobot-operator \
  -n uptimerobot --set apiKey.existingSecret=uptimerobot
```

Helm does not upgrade CRDs. Apply them manually on upgrades: `kubectl apply -f charts/uptimerobot-operator/crds`.

### Rate limits

| Value          | Default | Description                                                                              |
| -------------- | ------- | ---------------------------------------------------------------------------------------- |
| `syncInterval` | `1m`    | Interval in which resources are synced, i.e. drift is detected and status is refreshed. |
| `apiRateLimit` | `10`    | Max API requests per minute. FREE: 10, PRO: 2 × monitor limit (max 5000).                |

Changes to resources are applied immediately (within the rate limit). On `429` the operator waits for `Retry-After`.

## Usage

All resources can be listed with `kubectl get uptimerobot` (`-o wide` adds error messages). Each resource has a `Ready` condition with reason `Progressing`, `Synced`, `Rejected` (by UptimeRobot, e.g. plan limits), `RateLimited` or `Failed`, and Kubernetes events for changes and errors.

### Monitor

```yaml
apiVersion: uptimerobot.kuehnleon.github.io/v1alpha1
kind: Monitor
metadata:
  name: example
  labels:
    app: example
spec:
  type: KEYWORD # HTTP, KEYWORD, PING, PORT, HEARTBEAT, DNS, API, UDP
  url: https://example.com
  interval: 300
  keyword:
    type: ALERT_NOT_EXISTS
    value: OK
  alertContacts:
    - id: 1234567
  auth:
    type: HTTP_BASIC
    username: monitor
    passwordSecretRef:
      name: example-auth
      key: password
```

See `kubectl explain monitor.spec` for all fields. Type specific settings (e.g. `dnsRecords`, `apiAssertions`) can be passed as-is via `spec.config`.

### Ingress / Traefik IngressRoute

Annotate an `Ingress` or `IngressRoute` (`traefik.io` or `traefik.containo.us`) to create a `Monitor` named `ingress-<name>` / `ingressroute-<name>`:

```yaml
metadata:
  annotations:
    uptimerobot.kuehnleon.github.io/enabled: "true"
    # Optional: any Monitor spec field, nested fields separated by dots
    uptimerobot.kuehnleon.github.io/interval: "60"
    uptimerobot.kuehnleon.github.io/alertContacts: '[{"id": 1234567}]'
```

The URL defaults to the first host (`https` if TLS is configured) and can be overridden with `uptimerobot.kuehnleon.github.io/url`. The labels of the source resource are copied to the `Monitor`.

### MaintenanceWindow

```yaml
apiVersion: uptimerobot.kuehnleon.github.io/v1alpha1
kind: MaintenanceWindow
metadata:
  name: nightly
spec:
  interval: daily # once, daily, weekly, monthly
  date: "2026-01-01"
  time: "03:00:00"
  duration: 30 # minutes
  monitorSelector: {} # all Monitors in the namespace
```

### PublicStatusPage

```yaml
apiVersion: uptimerobot.kuehnleon.github.io/v1alpha1
kind: PublicStatusPage
metadata:
  name: status
spec:
  monitorSelector:
    matchLabels:
      app: example
```

Deleting a resource deletes it in UptimeRobot.

## Development

```sh
make generate # deepcopy + CRDs
make test     # unit and envtest integration tests
UPTIMEROBOT_API_KEY=... make run # against the current kubeconfig context
make kind     # build and deploy to a local kind cluster (requires secret "uptimerobot" in namespace "uptimerobot")
kubectl --context kind-uptimerobot apply -f hack/samples.yaml
```

`--zap-log-level=debug` (Helm: `extraArgs`) logs every API request including the remaining rate limit.
