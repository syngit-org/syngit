
# Image URL to use all building/pushing image targets
IMG ?= local/syngit-controller:dev
DEV_CLUSTER ?= syngit-dev-cluster
KIND_KUBECONFIG_PATH ?= /tmp/syngit-dev-cluster-kubeconfig
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0
CRD_OPTIONS ?= "crd"

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

# WEBHOOK_PATH is the path to the cluster webhook directory.
WEBHOOK_PATH ?= config/webhook
# CRD_PATH is the path to the cluster crd directory.
CRD_PATH ?= config/crd
# DEV_LOCAL_PATH is the path to the local dev env directory.
DEV_LOCAL_PATH ?= config/local

# DYNAMIC_WEBHOOK_NAME is the name of the webhook that handle the interception logic of RemoteSyncers
DYNAMIC_WEBHOOK_NAME ?= syngit-dynamic-remotesyncer-webhook

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

.PHONY: pre-commit-check
pre-commit-check: cleanup-tests manifests generate test lint ## Run all the tests and linters.

##@ Dev environment

# LOCALHOST_BRIDGE is a IP:PORT combination. The static & dynamic webhooks will be served on this host.
LOCALHOST_BRIDGE ?= $(shell docker network inspect bridge --format='{{(index .IPAM.Config 0).Gateway}}') # 172.17.0.1 is the default docker0 bridge IP.for linux user
# SYNGIT_SERVICE_NAME is the name of the Service object to serve the webhook server
SYNGIT_SERVICE_NAME="syngit-webhook-service.syngit.svc"
# TEMP_CERT_DIR is the path to the certificate that will be used by the webhook server.
TEMP_CERT_DIR ?= "/tmp/k8s-webhook-server/serving-certs"
# DEV_WEBHOOK_CERT is the path to the certificate that will be used by the webhook server.
DEV_WEBHOOK_CERT ?= $(TEMP_CERT_DIR)/tls.crt

.PHONY: run-fast
run-fast: manifests generate fmt vet ## Run a controller from your host. Install CRDs & webhooks if does not exists. No resources are deleted when killed (meant to be run often).
	@if ! kubectl get crd remoteusers.syngit.io &> /dev/null; then \
		make install-crds && make setup-webhooks-for-run; \
	fi
	export MANAGER_NAMESPACE=syngit DYNAMIC_WEBHOOK_NAME=$(DYNAMIC_WEBHOOK_NAME) DEV_MODE="true" DEV_WEBHOOK_HOST=$(LOCALHOST_BRIDGE) DEV_WEBHOOK_PORT=9443 DEV_WEBHOOK_CERT=$(DEV_WEBHOOK_CERT) && go run cmd/main.go

.PHONY: run
run: manifests generate fmt vet install-crds setup-webhooks-for-run ## Install CRDs, webhooks & run a controller from your host. All resources are deleted when killed.
	export MANAGER_NAMESPACE=syngit DYNAMIC_WEBHOOK_NAME=$(DYNAMIC_WEBHOOK_NAME) DEV_MODE="true" DEV_WEBHOOK_HOST=$(LOCALHOST_BRIDGE) DEV_WEBHOOK_PORT=9443 DEV_WEBHOOK_CERT=$(DEV_WEBHOOK_CERT) && \
	{ \
		trap 'echo "Cleanup resources"; make cleanup-run; exit' SIGINT; \
		go run cmd/main.go; \
	}

.PHONY: cleanup-run
cleanup-run: uninstall-crds cleanup-webhooks-for-run ## Cleanup the resources created by run-fast.
	kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io syngit-dynamic-remotesyncer-webhook || true

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

##@ Test

.PHONY: test
test: kind-create-cluster
test: export KUBECONFIG=${KIND_KUBECONFIG_PATH}
test: test-controller test-build-deploy test-behavior test-chart-install test-chart-upgrade ## Run all the tests.

.PHONY: test-controller
test-controller: kind-create-cluster
test-controller: export KUBECONFIG=${KIND_KUBECONFIG_PATH}
test-controller: manifests generate fmt vet envtest setup-webhooks-for-run ## Run tests embeded in the controller package & webhook package.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out
	make cleanup-webhooks-for-run

.PHONY: test-build-deploy
test-build-deploy: kind-create-cluster
test-build-deploy: export KUBECONFIG=${KIND_KUBECONFIG_PATH}
test-build-deploy: ## Run tests to build the Docker image and deploy all the manifests.
	go test ./test/e2e/build -v -ginkgo.v

# DEPRECATED_API_VERSIONS is a list of API versions that should not be tested since they are supposed to be converted to the last one.
DEPREACTED_API_VERSIONS = $(shell go list ./... | grep -oP 'v\d+\w+\d+' | sort -uV | awk 'NR == 1 {latest = $$0} NR > 1 {print prev} {prev = $$0}' | grep -v '^$$' | paste -sd "|" -)
# COVERPKG is a list of packages to be covered by the tests (internal/, pkg/ & cmd/).
COVERPKG = $(shell go list ./... | grep -v 'test' | grep -v -E "$(DEPREACTED_API_VERSIONS)" | paste -sd "," -)

.PHONY: test-behavior
test-behavior: kind-create-cluster
test-behavior: export KUBECONFIG=${KIND_KUBECONFIG_PATH}
test-behavior: cleanup-tests ## Install the test env (gitea). Run the behavior tests against a Kind k8s instance that is spun up. Cleanup when finished.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./test/e2e/syngit -timeout 25m -v -ginkgo.v -cover -coverpkg=$(COVERPKG) -coverprofile=coverage.txt

.PHONY: fast-behavior
fast-behavior: ## Install the test env if not already installed. Run the behavior tests against a Kind k8s instance that is spun up. Does not cleanup when finished (meant to be run often).
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./test/e2e/syngit -timeout 25m -v -ginkgo.v -cover -coverpkg=$(COVERPKG) -setup fast

.PHONY: test-selected
test-selected: kind-create-cluster
test-selected: export KUBECONFIG=${KIND_KUBECONFIG_PATH}
test-selected: ## Install the test env if not already installed. Run only one selected test against a Kind k8s instance that is spun up. Does not cleanup when finished (meant to be run often).
	@bash -c ' \
		TEST_NUMBER=$(TEST_NUMBER) ./hack/tests/run_one_test.sh $(TEST_NUMBER); \
		trap "./hack/tests/reset_test.sh" EXIT; \
		KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./test/e2e/syngit -v -ginkgo.v -cover -coverpkg=$(COVERPKG) -setup fast; \
	'

.PHONY: cleanup-tests
test-selected: kind-create-cluster
test-selected: export KUBECONFIG=${KIND_KUBECONFIG_PATH}
cleanup-tests: ## Uninstall all the charts needed for the tests.
	helm uninstall -n syngit syngit || true
	helm uninstall -n cert-manager cert-manager || true
	helm uninstall -n saturn gitea || true
	helm uninstall -n jupyter gitea || true
	./hack/webhooks/cleanup-injector.sh $(TEMP_CERT_DIR) || true
	./hack/tests/reset_test.sh

.PHONY: test-chart-install
test-chart-install: kind-create-cluster
test-chart-install: export KUBECONFIG=${KIND_KUBECONFIG_PATH}
test-chart-install: ## Run tests to install the chart.
	kubectl delete ns test || true
	go test ./test/e2e/helm/install -v -ginkgo.v

.PHONY: test-chart-upgrade
test-chart-upgrade: kind-create-cluster
test-chart-upgrade: export KUBECONFIG=${KIND_KUBECONFIG_PATH}
test-chart-upgrade: ## Run tests to upgrade the chart.
	kubectl delete ns test || true
	go test ./test/e2e/helm/upgrade -v -ginkgo.v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	@if [ -d "config/crd" ]; then \
		$(KUSTOMIZE) build config/crd > dist/install.yaml; \
	fi
	echo "---" >> dist/install.yaml  # Add a document separator before appending
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default >> dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = true
endif

.PHONY: install-crds
install-crds: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall-crds
uninstall-crds: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy syngit to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -
	make setup-webhooks-for-deploy

.PHONY: undeploy
undeploy: kustomize cleanup-webhooks-for-deploy ## Undeploy syngit from the K8s cluster specified in ~/.kube/config. Can be use after deploy or deploy-all.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy-all
deploy-all: kind-create-cluster docker-build kind-load-image cleanup-tests deploy # Create the dev cluster, build the image, load it in the cluster and deploy syngit.

##@ Webhook injection

.PHONY: setup-webhooks-for-run
setup-webhooks-for-run: manifests kustomize ## Setup webhooks using auto-generated certs & docker bridge host (make run).
	./hack/webhooks/cert-injector.sh $(WEBHOOK_PATH) $(CRD_PATH) $(DEV_LOCAL_PATH)/run $(TEMP_CERT_DIR) $(LOCALHOST_BRIDGE)
	./hack/webhooks/run/inject-for-run.sh $(LOCALHOST_BRIDGE)
	$(KUSTOMIZE) build $(DEV_LOCAL_PATH)/run | $(KUBECTL) apply -f -

.PHONY: cleanup-webhooks-for-run
cleanup-webhooks-for-run: manifests kustomize ## Cleanup webhooks using auto-generated certs & docker bridge host (make run).
	$(KUSTOMIZE) build $(DEV_LOCAL_PATH)/run | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -
	./hack/webhooks/cleanup-injector.sh $(TEMP_CERT_DIR) || true

.PHONY: setup-webhooks-for-deploy
setup-webhooks-for-deploy: manifests kustomize ## Setup webhooks using auto-generated certs (make deploy).
	./hack/webhooks/cert-injector.sh $(WEBHOOK_PATH) $(CRD_PATH) $(DEV_LOCAL_PATH)/deploy $(TEMP_CERT_DIR) $(SYNGIT_SERVICE_NAME)
	./hack/webhooks/deploy/inject-for-deploy.sh $(TEMP_CERT_DIR) $(WEBHOOK_PATH)
	$(KUSTOMIZE) build $(DEV_LOCAL_PATH)/deploy | $(KUBECTL) apply -f -

.PHONY: cleanup-webhooks-for-deploy
cleanup-webhooks-for-deploy: manifests kustomize ## Cleanup webhooks using auto-generated certs (make deploy).
	$(KUSTOMIZE) build $(DEV_LOCAL_PATH)/deploy | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -
	./hack/webhooks/cleanup-injector.sh $(TEMP_CERT_DIR) || true
	mv $(DEV_LOCAL_PATH)/deploy/webhook/secret.yaml.bak $(DEV_LOCAL_PATH)/deploy/webhook/secret.yaml || true

.PHONY: force-cleanup
force-cleanup: ## Force cleanup of the resources (for dev purpose)
	$(KUBECTL) delete crd remotesyncers.syngit.io --ignore-not-found=$(ignore-not-found)
	$(KUBECTL) delete crd remoteusers.syngit.io --ignore-not-found=$(ignore-not-found)
	$(KUBECTL) delete crd remoteuserbindings.syngit.io --ignore-not-found=$(ignore-not-found)
	$(KUBECTL) delete validatingwebhookconfigurations.admissionregistration.k8s.io syngit-dynamic-remotesyncer-webhook --ignore-not-found=$(ignore-not-found)
	./hack/webhooks/cleanup-injector.sh $(TEMP_CERT_DIR) || true

##@ KinD & Helm

# BEFORE_LATEST_CHART is the chart version before the latest one listed in the charts/ folder.
BEFORE_LATEST_CHART ?= $(shell ls -v charts | tail -3 | head -1)
# LATEST_CHART is the latest chart version listed in the charts/ folder.
LATEST_CHART ?= $(shell find charts -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort -V | tail -n 1)
# CERT_MANAGER_CHART_VERSION is the cert-manager chart version defined in the latest chart listed in the charts/ folder.
CERT_MANAGER_CHART_VERSION ?= $(shell helm dependency update charts/$(LATEST_CHART) &> /dev/null && sleep 1 && grep -A 2 'name: cert-manager' "charts/${LATEST_CHART}/Chart.lock" | grep 'version:' | sed -E 's/.*version:\s*(v[0-9]+\.[0-9]+\.[0-9]+).*/\1/')

.PHONY: chart-install
chart-install: ## Install the latest chart version listed in the charts/ folder with 3 replicas.
	kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MANAGER_CHART_VERSION)/cert-manager.crds.yaml" || 
	helm install syngit charts/$(LATEST_CHART) -n syngit --create-namespace \
		--set controller.image.prefix=local \
		--set controller.image.name=syngit-controller \
		--set controller.image.tag=dev \
		--set certmanager.dependency.enabled="true"

.PHONY: chart-install-providers
chart-install-providers: ## Install the latest chart version listed in the charts/ folder with 3 replicas.
	kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MANAGER_CHART_VERSION)/cert-manager.crds.yaml"
	helm install syngit charts/$(LATEST_CHART) -n syngit --create-namespace \
		--set controller.image.prefix=local \
		--set controller.image.name=syngit-controller \
		--set controller.image.tag=dev \
		--set providers.github.enabled="true" \
		--set providers.gitlab.enabled="true" \
		--set certmanager.dependency.enabled="true"

.PHONY: chart-upgrade
chart-upgrade: ## Upgrade to the latest chart version listed in the charts/ folder.
	helm dependency update charts/$(LATEST_CHART)
	helm upgrade syngit charts/$(LATEST_CHART) -n syngit \
		--set controller.image.prefix=local \
		--set controller.image.name=syngit-controller \
		--set controller.image.tag=dev

.PHONY: chart-uninstall
chart-uninstall: ## Uninstall the chart.
	helm uninstall syngit -n syngit

.PHONY: kind-create-cluster
kind-create-cluster: ## Create the dev KinD cluster.
	kind create cluster --name ${DEV_CLUSTER} || true
	kind export kubeconfig --kubeconfig ${KIND_KUBECONFIG_PATH} --name syngit-dev-cluster

.PHONY: kind-delete-cluster
kind-delete-cluster: ## Delete the dev KinD cluster.
	kind delete cluster --name ${DEV_CLUSTER}

.PHONY: kind-load-image
kind-load-image: ## Load the image in KinD.
	kind load docker-image ${IMG} --name ${DEV_CLUSTER}

##@ e2e Custom deployments

.PHONY: setup-gitea
setup-gitea: ## Setup the 2 gitea platforms in the cluster
	./test/utils/gitea/launch-gitea-setup.sh

.PHONY: reset-gitea
reset-gitea: ## Setup the 2 gitea platforms in the cluster
	./test/utils/gitea/reset-gitea-repos.sh

.PHONY: cleanup-gitea
cleanup-gitea: ## Cleanup the 2 gitea platforms from the cluster.
	rm -rf /tmp/gitea-certs
	helm uninstall gitea -n jupyter
	kubectl delete ns jupyter
	helm uninstall gitea -n saturn
	kubectl delete ns saturn

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)

## Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.19.0
ENVTEST_VERSION ?= latest
GOLANGCI_LINT_VERSION ?= v2.5.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))


.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef
