# uptimerobot-operator

Kubernetes operator for managing UptimeRobot monitors, groups, integrations, maintenance windows, and public status pages

## Prerequisites

- Helm 3.8 or newer (for OCI chart support).
- A Kubernetes cluster. The repository currently tests against Kubernetes 1.37; it does not yet declare a wider compatibility range.
- An account-specific UptimeRobot Main API key.
- Gateway API CRDs are required only when enabling the optional `gatewayAPI` source.

## Installation

Create a namespace and a Secret containing the API key:

```sh
kubectl create namespace uptimerobot
kubectl -n uptimerobot create secret generic uptimerobot --from-literal=api-key=<API_KEY>
```

Install a pinned chart version, referencing that Secret:

```sh
helm install uptimerobot-operator oci://ghcr.io/kuehnleon/charts/uptimerobot-operator \
  --namespace uptimerobot \
  --version <CHART_VERSION> \
  --set apiKey.existingSecret=uptimerobot
```

The chart also accepts `apiKey.value`, but that stores the credential in Helm release values. Prefer an existing Kubernetes Secret and do not commit API keys to source control.

## CRD upgrades and uninstalling

On a fresh install Helm installs the CRDs in the chart's `crds/` directory. Helm does not upgrade CRDs, so apply the CRDs from the exact chart version before upgrading the operator:

```sh
VERSION=<CHART_VERSION>
helm pull oci://ghcr.io/kuehnleon/charts/uptimerobot-operator --version "$VERSION" --untar
kubectl apply -f uptimerobot-operator/crds
helm upgrade uptimerobot-operator oci://ghcr.io/kuehnleon/charts/uptimerobot-operator \
  --namespace uptimerobot --version "$VERSION" --set apiKey.existingSecret=uptimerobot
```

Uninstalling the chart does not delete CRDs or custom resources. Deleting a custom resource deletes its corresponding UptimeRobot resource. If that is intended, delete the custom resources while the operator is still running and wait for their finalizers to complete before uninstalling. Delete CRDs separately and only when no custom resources need to be retained.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules applied to the operator pod. |
| apiKey | object | `{"existingSecret":"","secretKey":"api-key","value":""}` | UptimeRobot API key (account-specific "Main API key"). Use an existing Secret where possible. |
| apiKey.existingSecret | string | `""` | Name of an existing Secret containing the API key. Recommended over `value`. |
| apiKey.secretKey | string | `"api-key"` | Key within the existing Secret, or key to create when using `value`. |
| apiKey.value | string | `""` | Plain API key value. Creates a Secret; avoid putting credentials in Helm values or source control. |
| apiRateLimit | int | `10` | Maximum UptimeRobot API requests per minute. FREE: 10; PRO: 2 x monitor limit (max 5000). |
| extraArgs | list | `[]` | Additional command-line arguments passed to the operator. |
| gatewayAPI | bool | `false` | Create Monitors for annotated Gateway API HTTPRoute resources (requires Gateway API CRDs). |
| image | object | `{"pullPolicy":"IfNotPresent","repository":"ghcr.io/kuehnleon/uptimerobot-operator","tag":""}` | Image configuration for the operator. |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. |
| image.repository | string | `"ghcr.io/kuehnleon/uptimerobot-operator"` | Container image repository. |
| image.tag | string | `""` | Image tag. Defaults to the chart appVersion. |
| imagePullSecrets | list | `[]` | Secrets to use when pulling the operator image. |
| ingress | bool | `true` | Create Monitors for annotated Kubernetes Ingress resources. |
| leaderElection | bool | `true` | Enable leader election when running multiple replicas. |
| nodeSelector | object | `{}` | Node labels used to select nodes for the operator pod. |
| podAnnotations | object | `{}` | Annotations to add to the operator pod. |
| podSecurityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod-level security context. |
| podSecurityContext.runAsNonRoot | bool | `true` | Run the pod as a non-root user. |
| podSecurityContext.seccompProfile | object | `{"type":"RuntimeDefault"}` | Seccomp profile for the pod. |
| podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` | Seccomp profile type. |
| replicaCount | int | `1` | Number of operator replicas. Multiple replicas require leader election. |
| resources | object | `{"limits":{"memory":"128Mi"},"requests":{"cpu":"10m","memory":"64Mi"}}` | CPU and memory requests and limits for the operator container. |
| resources.limits | object | `{"memory":"128Mi"}` | Maximum resources allowed for the container. |
| resources.limits.memory | string | `"128Mi"` | Maximum memory allowed for the container. |
| resources.requests | object | `{"cpu":"10m","memory":"64Mi"}` | Minimum CPU and memory resources requested by the container. |
| resources.requests.cpu | string | `"10m"` | CPU requested by the container. |
| resources.requests.memory | string | `"64Mi"` | Memory requested by the container. |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container-level security context. |
| securityContext.allowPrivilegeEscalation | bool | `false` | Disallow privilege escalation. |
| securityContext.capabilities | object | `{"drop":["ALL"]}` | Linux capabilities configuration. |
| securityContext.capabilities.drop | list | `["ALL"]` | Linux capabilities to drop from the container. |
| securityContext.readOnlyRootFilesystem | bool | `true` | Mount the container root filesystem as read-only. |
| syncInterval | string | `"1m"` | Interval in which resources are synced with UptimeRobot. |
| tolerations | list | `[]` | Tolerations applied to the operator pod. |
| traefik | bool | `true` | Create Monitors for annotated Traefik IngressRoute resources. |
