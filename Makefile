
# Image URL to use all building/pushing image targets
CONTROLLER_IMG ?= kube-egress-gateway-controller
DAEMON_IMG ?= kube-egress-gateway-daemon
CNIMANAGER_IMG ?= kube-egress-gateway-cnimanager
CNI_IMG ?= kube-egress-gateway-cni
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.25.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
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
manifests: controller-gen daemon-rbac-manifests manager-rbac-manifests cnimanager-rbac-manifests ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases output:webhook:artifacts:config=config/manager/webhook
daemon-rbac-manifests: controller-gen ## Generate RBAC manifests for daemon.
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./controllers/manager/..." output:rbac:artifacts:config=config/manager/rbac
manager-rbac-manifests: controller-gen ## Generate RBAC manifests for manager.
	$(CONTROLLER_GEN) rbac:roleName=daemon-manager-role paths="./controllers/daemon/..." output:rbac:artifacts:config=config/daemon/rbac
cnimanager-rbac-manifests: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=cni-manager-role paths="./controllers/cnimanager/..." output:rbac:artifacts:config=config/cnimanager/rbac

.PHONY: generate
generate: generate-apiutils generate-protogo

generate-apiutils: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

generate-protogo: install-dependencies ## Generate code containing golang protobuf implementation.
	$(LOCALBIN)/buf generate
	$(LOCALBIN)/buf lint

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: golangci-lint ## Run go vet against code.
	$(LOCALBIN)/golangci-lint run --timeout 10m ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build

IMAGE_REGISTRY ?= local
IMAGE_TAG ?= $(shell git rev-parse --short=7 HEAD)

.PHONY: build
build: generate fmt vet ## Build manager binary.
	CGO_ENABLED=0 go build -o bin/manager ./cmd/kube-egress-gateway-controller/main.go
	CGO_ENABLED=0 go build -o bin/daemon ./cmd/kube-egress-gateway-daemon/main.go
	CGO_ENABLED=0 go build -o bin/cni ./cmd/kube-egress-cni/main.go
	CGO_ENABLED=0 go build -o bin/cnimanager ./cmd/kube-egress-gateway-cnimanager/main.go

AZURE_CONFIG_FILE ?= ./tests/deploy/azure.json
.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/kube-egress-gateway-controller/main.go --zap-log-level 5 --cloud-config $(AZURE_CONFIG_FILE)

.PHONY: docker-build
docker-build: test docker-builder-setup ## Build docker image with the manager.
	TAG=$(IMAGE_TAG) IMAGE_REGISTRY=$(IMAGE_REGISTRY) docker buildx bake -f docker-bake.hcl -f docker-localtag-bake.hcl --progress auto --push

.PHONY: docker-builder-setup
docker-builder-setup:
	docker run --privileged --rm tonistiigi/binfmt --install all

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize docker-build ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	cd config/default && \
	$(KUSTOMIZE) edit set image controller=$(IMAGE_REGISTRY)/$(CONTROLLER_IMG):$(IMAGE_TAG) && \
	$(KUSTOMIZE) edit set image daemon=$(IMAGE_REGISTRY)/${DAEMON_IMG}:$(IMAGE_TAG) && \
	$(KUSTOMIZE) edit set image cnimanager=$(IMAGE_REGISTRY)/${CNIMANAGER_IMG}:$(IMAGE_TAG) && \
	$(KUSTOMIZE) edit set image cni=$(IMAGE_REGISTRY)/${CNI_IMG}:$(IMAGE_TAG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

.PHONY: install-dependencies
install-dependencies: kustomize controller-gen envtest cobra-cli kubebuilder golangci-lint protoc-gen-go protoc-gen-go-grpc protoc buf ## Install all build dependencies.
	export PATH=$$PATH:$(LOCALBIN)

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)


KUSTOMIZE ?= $(LOCALBIN)/kustomize
KUSTOMIZE_VERSION ?= v4.5.7
KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || { curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

ENVTEST ?= $(LOCALBIN)/setup-envtest
.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

COBRA_CLI ?=$(LOCALBIN)/cobra-CLI
.PHONY: cobra-cli 
cobra-cli: $(COBRA_CLI) ## Download cobra cli locally if necessary.
${COBRA_CLI}: $(LOCALBIN)
	test -s $(LOCALBIN)/cobra-cli || GOBIN=$(LOCALBIN) go install github.com/spf13/cobra-cli@latest

KUBEBUILDER ?= $(LOCALBIN)/kubebuilder
.PHONY: kubebuilder 
kubebuilder: $(KUBEBUILDER) ## Download kubebuilder locally if necessary.
$(KUBEBUILDER): $(LOCALBIN) 
	test -s $(LOCALBIN)/kubebuilder || { curl -L -o $(LOCALBIN)/kubebuilder https://go.kubebuilder.io/dl/latest/`go env GOOS`/`go env GOARCH` && chmod a+x $(LOCALBIN)/kubebuilder ;}

GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
.PHONY: golangci-lint 
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LOCALBIN) latest

PROTOC_GEN_GO ?= $(LOCALBIN)/protoc-gen-go
.PHONY: protoc-gen-go
protoc-gen-go: $(PROTOC_GEN_GO) ## Download protoc-gen-go locally if necessary.
$(PROTOC_GEN_GO): $(LOCALBIN)
	test -s $(LOCALBIN)/protoc-gen-go || GOBIN=$(LOCALBIN) go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

PROTOC_GEN_GO_GRPC ?= $(LOCALBIN)/protoc-gen-go-grpc
.PHONY: protoc-gen-go-grpc
protoc-gen-go-grpc: $(PROTOC_GEN_GO_GRPC)  ## Download protoc-gen-go-grpc locally if necessary.
$(PROTOC_GEN_GO_GRPC): $(LOCALBIN) $(PROTOC)
	test -s $(LOCALBIN)/protoc-gen-go-grpc || GOBIN=$(LOCALBIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

PROTOC ?= $(LOCALBIN)/protoc
.PHONY: protoc
protoc: $(PROTOC)  ## Download protoc locally if necessary.
$(PROTOC): $(LOCALBIN) 
	test -s $(LOCALBIN)/protoc || { curl -L -o $(LOCALBIN)/protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v3.20.2/protoc-3.20.2-linux-x86_64.zip && unzip $(LOCALBIN)/protoc.zip -d $(LOCALBIN) && mv $(LOCALBIN)/bin/protoc $(LOCALBIN) && rm -rf $(LOCALBIN)/protoc.zip $(LOCALBIN)/readme.txt $(LOCALBIN)/bin $(LOCALBIN)/include; }

BUF ?= $(LOCALBIN)/buf
.PHONY: buf
buf: $(BUF)  ## Download buf locally if necessary.
$(BUF): $(LOCALBIN)
	test -s $(LOCALBIN)/buf || GOBIN=$(LOCALBIN) go install github.com/bufbuild/buf/cmd/buf@latest

