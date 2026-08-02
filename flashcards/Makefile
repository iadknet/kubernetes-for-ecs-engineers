IMAGE      ?= k8s-flashcards
TAG        ?= dev
CLUSTER    ?= k8s-training
PORT       ?= 8080
FUZZTIME   ?= 60s
CHAT_MODEL ?= sonnet

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## Run locally on http://localhost:$(PORT)
	PORT=$(PORT) go run ./cmd/flashcards

# Deliberately not folded into `run`: `run` matches what the cluster gets, and
# chat needs a logged-in claude CLI that not every clone of this repo has.
#
# It runs from the repo root rather than from here, because the chat subprocess
# inherits the server's working directory — and only at the root does the CLI
# pick up AGENTS.md and the k8s-for-ecs-engineers skill. Started from
# flashcards/ it answers without either, which looks like a worse model rather
# than a misconfiguration. DATA_DIR is pinned so review state stays in the same
# place `make run` puts it. Override the model with `make run-chat CHAT_MODEL=opus`.
.PHONY: run-chat
run-chat: build ## Run locally with the chat panel (needs a logged-in `claude` CLI)
	cd .. && PORT=$(PORT) DATA_DIR=$(CURDIR)/data \
	  CHAT_ENABLED=true CHAT_MODEL=$(CHAT_MODEL) $(CURDIR)/bin/flashcards

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

.PHONY: lint-decks
lint-decks: ## Check every card requires the glossary terms it uses (also run by `test`)
	go test -run 'TestGlossary' ./internal/deck/

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

.PHONY: secrets
secrets: ## Scan the whole repo, including git history, for committed secrets
	gitleaks detect --source .. --config ../.gitleaks.toml --no-banner --redact

.PHONY: dockerfile-lint
dockerfile-lint: ## Lint the Dockerfile (info-level nits reported, not fatal)
	hadolint --failure-threshold warning Dockerfile

.PHONY: fuzz
fuzz: ## Fuzz the deck parser for $(FUZZTIME) (seed corpus also runs in `make test`)
	go test -run=XXX -fuzz=FuzzLoad -fuzztime=$(FUZZTIME) ./internal/deck/

.PHONY: check
check: fmt-check lint vuln verify secrets dockerfile-lint test ## Every fast gate; no Docker needed
	@echo "all checks passed"

# --- Image scanning ----------------------------------------------------------
# Split from `check` because it needs Docker and an image build. `check` stays
# fast enough to run before every commit; this runs before a push or a deploy.

.PHONY: scan
scan: image ## Scan the built image for CVEs and secrets (needs Docker)
	trivy image --scanners vuln,secret --exit-code 1 \
	  --severity HIGH,CRITICAL --ignore-unfixed $(IMAGE):$(TAG)

.PHONY: check-all
check-all: check scan ## Everything, including the image scan
	@echo "all checks and scans passed"

.PHONY: base-digests
base-digests: ## Print current digests for the Dockerfile's base image tags
	@for ref in golang:1.26-alpine gcr.io/distroless/static-debian12:nonroot; do \
	  printf '%-48s %s\n' "$$ref" \
	    "$$(docker buildx imagetools inspect $$ref 2>/dev/null | awk '/^Digest:/{print $$2}')"; \
	done

.PHONY: hooks
hooks: ## Enable the repo's git hooks (one-time, per clone)
	git -C .. config core.hooksPath .githooks
	@echo "core.hooksPath -> .githooks (pre-commit: fast gates; pre-push: make check)"

.PHONY: tools
tools: ## Report whether the local toolchain is present
	@printf 'go            %s\n' "$$(go version 2>/dev/null || echo MISSING)"
	@printf 'golangci-lint %s\n' "$$(golangci-lint version 2>/dev/null || echo 'MISSING - brew install golangci-lint')"
	@printf 'govulncheck   %s\n' "$$(go tool govulncheck -version 2>/dev/null | grep Scanner || echo 'MISSING - go get -tool golang.org/x/vuln/cmd/govulncheck')"
	@printf 'gitleaks      %s\n' "$$(gitleaks version 2>/dev/null || echo 'MISSING - brew install gitleaks')"
	@printf 'hadolint      %s\n' "$$(hadolint --version 2>/dev/null || echo 'MISSING - brew install hadolint')"
	@printf 'trivy         %s\n' "$$(trivy --version 2>/dev/null | head -1 || echo 'MISSING - brew install trivy')"

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
