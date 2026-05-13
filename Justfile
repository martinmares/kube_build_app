set shell := ["bash", "-uc"]

default:
    @just --list

fmt:
    gofmt -w cmd internal

go-test:
    mkdir -p .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" go test ./...

ruby-test:
    ruby -Itest test/comprehensive_features_test.rb test/encjson_api_selection_test.rb

test: go-test ruby-test

build:
    mkdir -p dist
    mkdir -p .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" go build -o dist/kube-build-app ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" go build -o dist/kube-env-app ./cmd/kube-env-app

build-cross:
    mkdir -p dist .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=arm64 go build -o dist/kube-build-app-darwin-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=amd64 go build -o dist/kube-build-app-darwin-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=amd64 go build -o dist/kube-build-app-linux-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=arm64 go build -o dist/kube-build-app-linux-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=windows GOARCH=amd64 go build -o dist/kube-build-app-windows-amd64.exe ./cmd/kube-build-app

build-cross-all:
    mkdir -p dist .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=arm64 go build -o dist/kube-build-app-darwin-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=arm64 go build -o dist/kube-env-app-darwin-arm64 ./cmd/kube-env-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=amd64 go build -o dist/kube-build-app-darwin-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=amd64 go build -o dist/kube-env-app-darwin-amd64 ./cmd/kube-env-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=amd64 go build -o dist/kube-build-app-linux-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=amd64 go build -o dist/kube-env-app-linux-amd64 ./cmd/kube-env-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=arm64 go build -o dist/kube-build-app-linux-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=arm64 go build -o dist/kube-env-app-linux-arm64 ./cmd/kube-env-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=windows GOARCH=amd64 go build -o dist/kube-build-app-windows-amd64.exe ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=windows GOARCH=amd64 go build -o dist/kube-env-app-windows-amd64.exe ./cmd/kube-env-app

run-env-app root listen="127.0.0.1:8080":
    mkdir -p .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" go run ./cmd/kube-env-app --root "{{root}}" --listen "{{listen}}"

parity name root env release_id:
    scripts/parity-build --name "{{name}}" --root "{{root}}" --env "{{env}}" --release-id "{{release_id}}"

parity-go name root env release_id:
    just build
    scripts/parity-build --name "{{name}}" --root "{{root}}" --env "{{env}}" --release-id "{{release_id}}" --go-bin ./dist/kube-build-app

clean:
    rm -rf dist
