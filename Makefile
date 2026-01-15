GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor)

help:
	@echo "Various utilities for managing the terragrunt repository"

fmt:
	@echo "Running source files through gofmt..."
	gofmt -w $(GOFMT_FILES)

fmtcheck:
	pre-commit run goimports --all-files

install-pre-commit-hook:
	pre-commit install

# This build target just for convenience for those building directly from
# source. See also: .github/workflows/build.yml
build: terragrunt
terragrunt: $(shell find . \( -type d -name 'vendor' -prune \) \
                        -o \( -type f -name '*.go'   -print \) )
	set -xe ;\
	vtag_maybe_extra=$$(git describe --tags --abbrev=12 --dirty --broken) ;\
	CGO_ENABLED=0 go build -o $@ -ldflags "-s -w -X github.com/gruntwork-io/go-commons/version.Version=$${vtag_maybe_extra}" .

clean:
	rm -f terragrunt

IGNORE_TAGS := windows|linux|darwin|freebsd|openbsd|netbsd|dragonfly|solaris|plan9|js|wasip1|aix|android|illumos|ios|386|amd64|arm|arm64|mips|mips64|mips64le|mipsle|ppc64|ppc64le|riscv64|s390x|wasm

LINT_TAGS := $(shell grep -rh 'go:build' . | \
	sed 's/.*go:build\s*//' | \
	tr -cs '[:alnum:]_' '\n' | \
	grep -vE '^($(IGNORE_TAGS))$$' | \
	sed '/^$$/d' | \
	sort -u | \
	paste -sd, -)

run-lint:
	@echo "Linting with feature flags: [$(LINT_TAGS)]"
	GOFLAGS="-tags=$(LINT_TAGS)" golangci-lint run -v --timeout=10m ./...

run-strict-lint:
	golangci-lint run -v --timeout=10m -c .strict.golangci.yml --new-from-rev origin/main ./...

generate-mocks:
	go generate ./...

license-check:
	go mod vendor
	licensei cache --debug
	licensei check --debug
	licensei header --debug

.PHONY: help fmt fmtcheck install-pre-commit-hook clean run-lint run-strict-lint
