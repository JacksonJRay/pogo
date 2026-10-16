.PHONY: build test vet race e2e cross clean security security-report

BINARY := pogo
PKG := ./cmd/pogo
LDFLAGS := -w
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

build:
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o $(BINARY) $(PKG)

vet:
	go vet ./...

test: vet
	go test -count=1 -timeout 60m ./...

race:
	go test -race -count=1 -timeout 60m ./...

security: build
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -show verbose ./cmd/pogo ./internal/...
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -mode binary ./$(BINARY)

security-report: build
	python3 scripts/security_report.py

e2e: build
	bash scripts/e2e.sh

cross:
	@mkdir -p dist
	@for pair in $(TARGETS); do \
		os=$${pair%/*}; arch=$${pair#*/}; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o dist/pogo-$$os-$$arch $(PKG); \
	done

clean:
	rm -f $(BINARY)
	rm -rf dist
