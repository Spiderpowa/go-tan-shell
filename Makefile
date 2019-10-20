PROJECT_ROOT=github.com/Spiderpowa/go-tan-shell

define BUILD_RULE
$1: 
	go build -o ./build/bin/$1 $(PROJECT_ROOT)/cmd/$1
endef

COMPONENTS = \
	tsh-client

.PHONY: all vet check-security test bench lint format check-format pre-commit archive

default: all

all: $(COMPONENTS)

$(foreach component, $(COMPONENTS), $(eval $(call BUILD_RULE,$(component))))

format:
	@go fmt `go list ./... | grep -v 'vendor'`

lint:
	@golint -set_exit_status `go list ./... | grep -v 'vendor'`

vet:
	@go vet `go list ./... | grep -v 'vendor'`

check-security:
	@rm -f gosec.log
	@gosec -quiet -out gosec.log ./... || true
	@if [ -a gosec.log ]; then \
		cat gosec.log; \
		echo 'Error: security issue found'; \
		exit 1; \
	fi

test:
	@for pkg in `go list ./... | grep -v 'vendor'`; do \
		if ! go test -v -race $$pkg; then \
			echo 'Some test failed, abort'; \
			exit 1; \
		fi; \
	done

bench:
	@for pkg in `go list ./... | grep -v 'vendor'`; do \
		if ! go test -bench=. -run=^$$ $$pkg; then \
			echo 'Some test failed, abort'; \
			exit 1; \
		fi; \
	done

check-format:
	@if gofmt -l `go list -f '{{.Dir}}' ./...` | grep -v 'vendor' | grep -q go; then \
		echo 'Error: source code not formatted'; \
		exit 1; \
	fi

pre-commit: lint vet check-format test
