BINARY := ofmx-parser

.PHONY: build run test fmt vet

build:
	go build -o bin/$(BINARY) ./cmd/ofmx-parser

run:
	@if [ -z "$(INPUT)" ] || [ -z "$(OUTPUT)" ]; then \
		echo "Usage: make run INPUT=path/to/input.ofmx OUTPUT=path/to/output.xml"; \
		exit 1; \
	fi
	go run ./cmd/ofmx-parser --input $(INPUT) --output $(OUTPUT)

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal ./test

vet:
	go vet ./...
