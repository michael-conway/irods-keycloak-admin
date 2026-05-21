.PHONY: test
test:
	go test ./...

.PHONY: fmt
fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

.PHONY: build
build:
	go build ./cmd/...

.PHONY: install
install:
	go install ./cmd/irods-kc-admin
	go install ./cmd/irods-kc-admin-server
	go install ./cmd/irods-kc-doctor
	go install ./cmd/irods-kc-sync
