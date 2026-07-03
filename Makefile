# Image URL to use all building/pushing image targets
TAG ?= dev
IMG ?= quay.io/cloudscalech/capcs-staging:$(TAG)
# YEAR defines the year value used for substituting the YEAR placeholder in the boilerplate header.
YEAR ?= $(shell date +%Y)
LDFLAGS ?= -X main.version=$(TAG)

# E2E image configuration
E2E_TAG ?= e2e-$(shell git rev-parse --short HEAD)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt",year=$(YEAR) paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

.PHONY: govulncheck
govulncheck: govulncheck-tool ## Run govulncheck to scan for known, reachable vulnerabilities (incl. stdlib/toolchain).
	"$(GOVULNCHECK)" ./...

##@ Dependencies

## Location to install dependencies to
LOCALBIN := $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

# Host OS/ARCH used to namespace tool binaries in $(LOCALBIN)
HOST_PLATFORM := $(shell go env GOOS)-$(shell go env GOARCH)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GINKGO ?= $(LOCALBIN)/ginkgo
GOVULNCHECK ?= $(LOCALBIN)/govulncheck

##@ E2E Testing

E2E_CONF_FILE_SOURCE ?= $(shell pwd)/test/e2e/config/cloudscale.yaml
E2E_CONF_FILE ?= $(shell pwd)/test/e2e/config/cloudscale.generated.yaml
E2E_ARTIFACTS_FOLDER ?= $(shell pwd)/_artifacts
E2E_TEMPLATES := test/e2e/data/infrastructure-cloudscale
GINKGO_TIMEOUT ?= 3h
GINKGO_NODES ?= 1
SKIP_RESOURCE_CLEANUP ?= false
USE_EXISTING_CLUSTER ?= false
KUBETEST_CONFIGURATION ?= ./data/kubetest/conformance.yaml
GINKGO_LABEL_FILTER ?=

# Cilium CNI configuration
CILIUM_VERSION ?= 1.19.2

# CCM configuration
CCM_VERSION ?= 1.3.0

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo locally if necessary.
$(GINKGO): $(LOCALBIN)
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo,$(shell go list -m -f '{{.Version}}' github.com/onsi/ginkgo/v2))

.PHONY: generate-e2e-cni
generate-e2e-cni: ## Regenerate Cilium CNI manifest from Helm chart
	@CILIUM_VERSION=$(CILIUM_VERSION) hack/generate-e2e-cni.sh

.PHONY: generate-e2e-ccm
generate-e2e-ccm: ## Regenerate cloudscale CCM manifest
	@CCM_VERSION=$(CCM_VERSION) hack/generate-e2e-ccm.sh

E2E_CLUSTER_TEMPLATES := cluster-template \
	cluster-template-fip \
	cluster-template-ha \
	cluster-template-md-remediation \
	cluster-template-pre-existing-network \
	cluster-template-public-lb-private-nodes \
	cluster-template-topology \
	cluster-template-upgrades \
	clusterclass-quick-start

.PHONY: generate-e2e-templates
generate-e2e-templates: $(KUSTOMIZE) generate-e2e-cni generate-e2e-ccm ## Generate e2e cluster templates using kustomize overlays
	@mkdir -p $(E2E_TEMPLATES)/main
	@$(foreach tmpl,$(E2E_CLUSTER_TEMPLATES),\
		echo "Generating $(tmpl).yaml..." && \
		"$(KUSTOMIZE)" build --load-restrictor LoadRestrictionsNone $(E2E_TEMPLATES)/$(tmpl) > $(E2E_TEMPLATES)/main/$(tmpl).yaml &&) true
	@echo "Templates generated successfully."

.PHONY: generate-e2e-config
generate-e2e-config: ## Generate e2e config from template by resolving environment variables
	TAG=$(TAG) IMG=$(IMG) KUBETEST_CONFIGURATION=$(KUBETEST_CONFIGURATION) envsubst < $(E2E_CONF_FILE_SOURCE) > $(E2E_CONF_FILE)

.PHONY: test-e2e
test-e2e: TAG = $(E2E_TAG)
test-e2e: KUBETEST_CONFIGURATION = ./data/kubetest/conformance-fast.yaml
test-e2e: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build docker-push ## Run all e2e tests (uses conformance-fast; for full conformance run test-e2e-conformance separately)
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--timeout=$(GINKGO_TIMEOUT) \
		$(if $(GINKGO_LABEL_FILTER),--label-filter="$(GINKGO_LABEL_FILTER)") \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_suite.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-lifecycle
test-e2e-lifecycle: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build ## Run lifecycle e2e tests only (single control-plane)
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="lifecycle && !ha" \
		--timeout=60m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_lifecycle.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-ha
test-e2e-ha: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build ## Run HA e2e tests only (3 control-plane nodes)
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="ha" \
		--timeout=90m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_ha.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-upgrade
test-e2e-upgrade: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build ## Run cluster upgrade e2e tests
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="upgrade" \
		--timeout=90m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_upgrade.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-self-hosted
test-e2e-self-hosted: TAG = $(E2E_TAG)
test-e2e-self-hosted: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build docker-push ## Run self-hosted e2e tests
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="self-hosted" \
		--timeout=90m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_self_hosted.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-md-remediation
test-e2e-md-remediation: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build ## Run MD remediation e2e tests
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="md-remediation" \
		--timeout=90m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_md_remediation.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-pre-existing-networking
test-e2e-pre-existing-networking: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build ## Run pre-existing networking e2e tests
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="pre-existing-networking" \
		--timeout=90m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_pre-existing_networking.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-topology
test-e2e-topology: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build ## Run cluster-class topology e2e tests
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="topology" \
		--timeout=90m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_topology.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-conformance
test-e2e-conformance: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build ## Run K8s conformance e2e tests
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="conformance" \
		--timeout=150m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_conformance.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

.PHONY: test-e2e-conformance-fast
test-e2e-conformance-fast: KUBETEST_CONFIGURATION = ./data/kubetest/conformance-fast.yaml
test-e2e-conformance-fast: $(GINKGO) generate-e2e-templates generate-e2e-config docker-build ## Run K8s conformance e2e tests (fast, skip Serial)
	$(GINKGO) -v --trace --tags=e2e \
		--nodes=$(GINKGO_NODES) \
		--label-filter="conformance" \
		--timeout=90m \
		--output-dir="$(E2E_ARTIFACTS_FOLDER)" --junit-report="junit.e2e_conformance_fast.xml" \
		./test/e2e -- \
		-e2e.config=$(E2E_CONF_FILE) \
		-e2e.artifacts-folder=$(E2E_ARTIFACTS_FOLDER) \
		-e2e.skip-resource-cleanup=$(SKIP_RESOURCE_CLEANUP) \
		-e2e.use-existing-cluster=$(USE_EXISTING_CLUSTER)

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -ldflags '$(LDFLAGS)' -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run -ldflags '$(LDFLAGS)' ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build --platform linux/amd64 --build-arg VERSION=$(TAG) -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: clean-e2e-images
clean-e2e-images: ## Delete e2e-* tags older than 7 days from capcs-staging (requires regctl + quay.io auth)
	@./hack/clean-e2e-images.sh

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/amd64
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name cluster-api-provider-cloudscale-builder
	$(CONTAINER_TOOL) buildx use cluster-api-provider-cloudscale-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --build-arg VERSION=$(TAG) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm cluster-api-provider-cloudscale-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default > dist/infrastructure-components.yaml

.PHONY: release-manifests
release-manifests: build-installer ## Build all release artifacts into dist/ (infrastructure-components.yaml, metadata.yaml, cluster templates).
	cp metadata.yaml dist/metadata.yaml
	cp templates/cluster-template*.yaml dist/
	cp templates/cluster-class*.yaml dist/

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

## Tool Versions
KUSTOMIZE_VERSION ?= v5.8.1
CONTROLLER_TOOLS_VERSION ?= v0.21.0

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

GOLANGCI_LINT_VERSION ?= v2.12.2
GOVULNCHECK_VERSION ?= v1.5.0
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))
	@test -f .custom-gcl.yml && { \
		echo "Building custom golangci-lint with plugins..." && \
		$(GOLANGCI_LINT) custom --destination $(LOCALBIN) --name golangci-lint-custom && \
		mv -f $(LOCALBIN)/golangci-lint-custom $(GOLANGCI_LINT); \
	} || true

.PHONY: govulncheck-tool
govulncheck-tool: $(GOVULNCHECK) ## Download govulncheck locally if necessary.
$(GOVULNCHECK): $(LOCALBIN)
	$(call go-install-tool,$(GOVULNCHECK),golang.org/x/vuln/cmd/govulncheck,$(GOVULNCHECK_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)-$(HOST_PLATFORM)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)-$(HOST_PLATFORM)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package} ($(HOST_PLATFORM))" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)-$(HOST_PLATFORM)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)-$(HOST_PLATFORM)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
