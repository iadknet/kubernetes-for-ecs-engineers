IMAGE   ?= k8s-flashcards
TAG     ?= dev
CLUSTER ?= k8s-training
PORT    ?= 8080

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## Run locally on http://localhost:$(PORT)
	PORT=$(PORT) go run ./cmd/flashcards

# --- Quality gates -----------------------------------------------------------
# Every gate is its own target so it can be run and debugged on its own. `check`
# only composes them. When a pipeline is eventually added it should call these
# same targets, so a check can never exist in CI but not on a laptop.

.PHONY: test
test: ## Run all tests under the race detector
	go test -race ./...

.PHONY: cover
cover: ## Run tests with a coverage summary
	go test -race -cover ./...

.PHONY: fmt
fmt: ## Rewrite files with gofmt + gofumpt
	golangci-lint fmt

.PHONY: fmt-check
fmt-check: ## Fail if any file needs formatting (no writes)
	@golangci-lint fmt --diff

.PHONY: lint
lint: ## Static analysis (see .golangci.yml; includes gosec)
	golangci-lint run ./...

.PHONY: vuln
vuln: ## Scan dependencies for known vulnerabilities that our code actually reaches
	go tool govulncheck ./...

.PHONY: verify
verify: ## Verify module checksums and that go.mod/go.sum are tidy
	go mod verify
	go mod tidy -diff

.PHONY: check
check: fmt-check lint vuln verify test ## Run every quality gate (what CI would run)
	@echo "all checks passed"

.PHONY: tools
tools: ## Report whether the local toolchain is present
	@printf 'go            %s\n' "$$(go version 2>/dev/null || echo MISSING)"
	@printf 'golangci-lint %s\n' "$$(golangci-lint version 2>/dev/null || echo 'MISSING - brew install golangci-lint')"
	@printf 'govulncheck   %s\n' "$$(go tool govulncheck -version 2>/dev/null | grep Scanner || echo 'MISSING - go get -tool golang.org/x/vuln/cmd/govulncheck')"

.PHONY: build
build: ## Build the binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/flashcards ./cmd/flashcards

.PHONY: image
image: ## Build the container image
	docker build -t $(IMAGE):$(TAG) .

.PHONY: docker-run
docker-run: image ## Run the container, persisting review state to ./data
	docker run --rm -p $(PORT):8080 -v "$(PWD)/data:/data" $(IMAGE):$(TAG)

.PHONY: clean
clean: ## Remove build output
	rm -rf bin

# --- Kubernetes targets: unused until M1. See the hardening backlog in README.

.PHONY: kind-load
kind-load: image ## Load the image into the KIND cluster (M1 onward)
	kind load docker-image $(IMAGE):$(TAG) --name $(CLUSTER)
