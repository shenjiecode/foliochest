.PHONY: build web test check

web:
	npm --prefix web ci --no-audit --no-fund
	npm --prefix web run build

build: web
	CGO_ENABLED=0 go build -trimpath -o bin/keyfolio ./cmd/keyfolio

test: web
	go test ./...

check: web
	go vet ./...
	go test -race ./...
