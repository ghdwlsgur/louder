CONTROLLER_GEN ?= $(shell go env GOPATH)/bin/controller-gen
CONTROLLER_TOOLS_VERSION ?= v0.20.0
IMAGE ?= louder-operator:local

.PHONY: build docker-build kind-e2e kind-e2e-secrets kind-e2e-storage fmt fmt-check vet test manifests controller-gen

build:
	@mkdir -p bin
	go build -o bin/louder-operator ./cmd/operator
	go build -o bin/louder-collector ./cmd/collector
	go build -o bin/louder-analyzer ./cmd/analyzer

docker-build:
	docker build --tag $(IMAGE) .

kind-e2e: docker-build
	bash scripts/kind/e2e.sh

kind-e2e-secrets: docker-build
	bash scripts/kind/e2e.sh secrets

kind-e2e-storage: docker-build
	bash scripts/kind/e2e.sh storage

fmt:
	gofmt -w $$(find api cmd internal -name '*.go' -type f)

fmt-check:
	@test -z "$$(gofmt -l $$(find api cmd internal -name '*.go' -type f))"

vet:
	go vet ./...

test:
	go test ./... -count=1

manifests: controller-gen
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

controller-gen:
	@if [ ! -x "$(CONTROLLER_GEN)" ]; then \
		GOBIN="$$(go env GOPATH)/bin" go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION); \
	fi
