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

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: cover
cover: ## Run tests with a coverage summary
	go test -cover ./...

.PHONY: check
check: ## Format check, vet, and test
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...

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
