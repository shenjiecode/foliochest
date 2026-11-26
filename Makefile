.PHONY: build web test check package

web:
	npm --prefix web ci --no-audit --no-fund
	npm --prefix web run build

build: web
	CGO_ENABLED=0 go build -trimpath -o bin/foliochest ./cmd/foliochest

package: web
	go run ./tools/package

test: web
	npm --prefix web test
	go test ./...

check: web
	npm --prefix web run check
	npm --prefix web test
	go vet ./...
	go test -race ./...
