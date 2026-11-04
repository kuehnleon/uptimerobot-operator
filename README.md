# uptimerobot-operator

Kubernetes operator managing [UptimeRobot](https://uptimerobot.com) monitors, monitor groups, integrations, personal alert contacts, maintenance windows, public status pages and announcements, using the [UptimeRobot v3 API](https://uptimerobot.com/api/v3/).

- `Monitor`, `MonitorGroup`, `Integration`, `AlertContact`, `MaintenanceWindow`, `PublicStatusPage` and `PublicStatusPageAnnouncement` custom resources
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

Published container images support `linux/amd64` and `linux/arm64`.

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
    - integrationRef:
        name: team-slack # Integration in the same namespace
    - alertContactRef:
        name: on-call # Personal AlertContact in the same namespace
    - id: 1234567 # or any alert contact by ID
  groupRef:
    name: team-a # MonitorGroup in the same namespace
  auth:
    type: HTTP_BASIC
    username: monitor
    passwordSecretRef:
      name: example-auth
      key: password
```

See `kubectl explain monitor.spec` for all fields. Type specific settings (e.g. `dnsRecords`, `apiAssertions`) can be passed as-is via `spec.config`.

### AlertContact

Personal email and mobile push contacts can be managed directly. Push tokens are read from a Secret and are never written to status or logs. SMS and voice contacts require phone verification and must continue to be referenced by an existing UptimeRobot ID.

```yaml
apiVersion: uptimerobot.kuehnleon.github.io/v1alpha1
kind: AlertContact
metadata:
  name: on-call
spec:
  type: MobileAppAndroid # Email, MobileAppIOS or MobileAppAndroid
  pushTokenSecretRef:
    name: on-call-push
    key: token
  deviceName: on-call phone
```

`type`, `value`, `pushTokenSecretRef` and `deviceName` are immutable because the API does not support changing a contact's address or push token. Updating the referenced Secret does not rotate the remote token; recreate the resource to change it.

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

### MonitorGroup

```yaml
apiVersion: uptimerobot.kuehnleon.github.io/v1alpha1
kind: MonitorGroup
metadata:
  name: team-a
spec:
  friendlyName: Team A # defaults to the resource name
```

Monitors join a group via `spec.groupRef`.

### Integration

Notification channels: `Slack`, `MSTeams`, `Webhook`, `Discord`, `Mattermost`, `GoogleChat`, `Pagerduty`, `Pushover`, `PushBullet`, `Splunk`, `Zapier`.

```yaml
apiVersion: uptimerobot.kuehnleon.github.io/v1alpha1
kind: Integration
metadata:
  name: team-slack
spec:
  type: Slack
  enableNotificationsFor: Down # UpAndDown (default), Down, Up, None
  config:
    customValue: "@here"
  secretConfig:
    webhookURL:
      name: team-slack
      key: webhook-url
```

Available types depend on the UptimeRobot plan (e.g. `Webhook` is not available on Solo), and some types validate their URL on creation.

`config` and `secretConfig` hold the type specific fields of the [API](https://uptimerobot.com/api/v3/#post-/integrations), e.g. `webhookURL` (Slack, MSTeams, Discord, Mattermost), `urlToNotify` and `postValue` (Webhook) or `integrationKey` (Pagerduty).

### MaintenanceWindow

```yaml
apiVersion: uptimerobot.kuehnleon.github.io/v1alpha1
kind: MaintenanceWindow
metadata:
  name: nightly
spec:
  interval: daily # once (requires date), daily, weekly or monthly (require days)
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

### PublicStatusPageAnnouncement

Announcements are attached to a `PublicStatusPage` in the same namespace. `status` may be `Offline`, `Pending`, `Published` or `Archived`; `type` may be `Info`, `Maintenance` or `Issue`. Set `pinned: true` to pin the announcement on the page.

```yaml
apiVersion: uptimerobot.kuehnleon.github.io/v1alpha1
kind: PublicStatusPageAnnouncement
metadata:
  name: scheduled-maintenance
spec:
  publicStatusPageRef:
    name: status
  title: Scheduled maintenance
  content: We will perform scheduled maintenance.
  type: Maintenance
  status: Published
  startDate: "2026-11-05T02:00:00Z"
  endDate: "2026-11-05T03:00:00Z"
  pinned: true
```

### Adoption and deletion

To adopt an existing UptimeRobot object, add its ID as an annotation. The operator verifies the ID exists, records it in status, and then reconciles the declared spec; it will not create a duplicate when the ID is invalid or missing.

```yaml
metadata:
  annotations:
    uptimerobot.kuehnleon.github.io/adopt-id: "1234567"
```

Deleting a resource deletes its UptimeRobot object by default. Set `spec.deletionPolicy: Orphan` to remove the Kubernetes resource while leaving the remote object intact. `Delete` is the default for every resource kind.

## Development

```sh
make generate # deepcopy + CRDs
make test     # unit and envtest integration tests
UPTIMEROBOT_API_KEY=... make run # against the current kubeconfig context
make kind     # build and deploy to a local kind cluster (requires secret "uptimerobot" in namespace "uptimerobot")
kubectl --context kind-uptimerobot apply -f hack/samples.yaml
```

`--zap-log-level=debug` (Helm: `extraArgs`) logs every API request including the remaining rate limit.

## Releasing

Run the [Release workflow](../../actions/workflows/release.yaml). The version is derived from the [Conventional Commits](https://www.conventionalcommits.org) since the last release (`fix` → patch, `feat` and breaking changes → minor before 1.0.0), or can be chosen explicitly. It updates [CHANGELOG.md](CHANGELOG.md) via [git-cliff](https://git-cliff.org) and the chart version, then commits and tags the version. The tag triggers the Build workflow, which publishes image and chart and creates a GitHub release.

The Release workflow uses the [kuehnleon-forgekeeper GitHub App](https://github.com/settings/apps/kuehnleon-forgekeeper). Install it on this repository with *Contents: read and write*, add it as a bypass actor to the `main` branch ruleset, and configure the repository variable `RELEASE_APP_ID` and secret `RELEASE_APP_PRIVATE_KEY`. The workflow mints a short-lived token scoped to this repository.

## Contributing

Pull requests are squash merged, so PR titles must follow [Conventional Commits](https://www.conventionalcommits.org), e.g. `feat(monitor): support regions` or `fix!: rename field`. They make up the changelog.
