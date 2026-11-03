IMG ?= ghcr.io/kuehnleon/uptimerobot-operator:dev
CHART := charts/uptimerobot-operator

.PHONY: all generate test build run docker-build helm-lint helm-docs kind

all: generate test build

generate:
	go tool controller-gen object paths=./api/...
	go tool controller-gen crd paths=./api/... output:crd:artifacts:config=$(CHART)/crds

test:
	go vet ./...
	KUBEBUILDER_ASSETS="$$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.25 use 1.37 -p path)" go test ./...

build:
	go build -o bin/manager ./cmd

run:
	go run ./cmd

docker-build:
	docker buildx build --platform linux/arm64 -t $(IMG) --load .

helm-lint:
	helm lint $(CHART) --set apiKey.value=lint-only
	helm template test $(CHART) --namespace uptimerobot --set apiKey.existingSecret=uptimerobot --include-crds >/dev/null

helm-docs:
	go run github.com/norwoodj/helm-docs/cmd/helm-docs@v1.14.2 --chart-search-root charts --documentation-strict-mode --skip-version-footer

KIND_CLUSTER ?= uptimerobot
KIND_CONTEXT := kind-$(KIND_CLUSTER)

# Deploys the operator to a local kind cluster. Requires the secret "uptimerobot" (key "api-key") in namespace "uptimerobot".
kind: docker-build
	kind get clusters | grep -qx $(KIND_CLUSTER) || DOCKER_DEFAULT_PLATFORM=linux/arm64 kind create cluster --name $(KIND_CLUSTER)
	kind load docker-image $(IMG) --name $(KIND_CLUSTER)
	kubectl --context $(KIND_CONTEXT) apply --server-side --force-conflicts -f $(CHART)/crds
	helm upgrade --install uptimerobot-operator $(CHART) --kube-context $(KIND_CONTEXT) -n uptimerobot --create-namespace \
		--set image.repository=$(word 1,$(subst :, ,$(IMG))) --set image.tag=$(word 2,$(subst :, ,$(IMG))) \
		--set apiKey.existingSecret=uptimerobot
	kubectl --context $(KIND_CONTEXT) -n uptimerobot rollout restart deployment uptimerobot-operator
	kubectl --context $(KIND_CONTEXT) -n uptimerobot rollout status deployment uptimerobot-operator
