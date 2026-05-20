.PHONY: test
test:
	go test ./...

.PHONY: fmt
fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

.PHONY: build
build:
	go build ./cmd/...

