# Some of the targets in this file were taken from the terraform project:
# https://github.com/hashicorp/terraform/blob/master/scripts/gofmtcheck.sh

GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor)

help:
	@echo "Various utilities for managing the terragrunt repository"

fmtcheck:
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

fmt:
	gofmt -w $(GOFMT_FILES)

install-pre-commit-hook:
	@if [ -f .git/hooks/pre-commit ]; then \
       echo ""; \
       echo "There is already a pre-commit hook installed. Remove it and run 'make"; \
       echo "install-pre-commit-hook again, or manually alter it to add the contents"; \
       echo "of 'scripts/pre-commit'."; \
       echo ""; \
       exit 1; \
   fi
	@ln -s ../../scripts/pre-commit .git/hooks/pre-commit
	@echo "pre-commit hook installed."

.PHONY: help fmtcheck fmt install-fmt-hook
