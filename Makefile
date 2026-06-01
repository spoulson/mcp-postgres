BINARY = mcp-postgres
GOLANGCI_LINT = ./bin/golangci-lint
GOLANGCI_LINT_VERSION = v2.11.4

.PHONY: build
build:
	go build -o $(BINARY) .

.PHONY: test
test:
	go test -race ./...

$(GOLANGCI_LINT):
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin $(GOLANGCI_LINT_VERSION)

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run
