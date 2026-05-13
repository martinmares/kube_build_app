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

build-versioned version:
    mkdir -p dist .tmp/go-build-cache
    commit="$(git rev-parse --short HEAD 2>/dev/null || printf unknown)"; \
    date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"; \
    ldflags="-X kube-env/internal/appinfo.Version={{version}} -X kube-env/internal/appinfo.Commit=$commit -X kube-env/internal/appinfo.Date=$date"; \
    GOCACHE="$PWD/.tmp/go-build-cache" go build -ldflags "$ldflags" -o dist/kube-build-app ./cmd/kube-build-app; \
    GOCACHE="$PWD/.tmp/go-build-cache" go build -ldflags "$ldflags" -o dist/kube-env-app ./cmd/kube-env-app

build-cross:
    mkdir -p dist .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=arm64 go build -o dist/kube-build-app-darwin-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=amd64 go build -o dist/kube-build-app-darwin-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=amd64 go build -o dist/kube-build-app-linux-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=arm64 go build -o dist/kube-build-app-linux-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=windows GOARCH=amd64 go build -o dist/kube-build-app-windows-amd64.exe ./cmd/kube-build-app

release version:
    rm -rf dist/release
    mkdir -p dist/release .tmp/go-build-cache
    commit="$(git rev-parse --short HEAD 2>/dev/null || printf unknown)"; \
    date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"; \
    ldflags="-X kube-env/internal/appinfo.Version={{version}} -X kube-env/internal/appinfo.Commit=$commit -X kube-env/internal/appinfo.Date=$date"; \
    for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do \
      os="${target%/*}"; arch="${target#*/}"; ext=""; [ "$os" = windows ] && ext=".exe"; \
      out="dist/release/kube-build-app-$os-$arch$ext"; \
      GOCACHE="$PWD/.tmp/go-build-cache" GOOS="$os" GOARCH="$arch" go build -ldflags "$ldflags" -o "$out" ./cmd/kube-build-app; \
    done; \
    (cd dist/release && shasum -a 256 kube-build-app-* > SHA256SUMS)

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

parity-cetin-test release_id="2025.08.18.1":
    just build
    scripts/parity-build --name "cetin-test" --root "/Users/mares/Development/Src/Ruby/tsm/cetin/tsm-environments" --env "test" --release-id "{{release_id}}" --go-bin ./dist/kube-build-app

parity-o2sk-tst release_id="2025.08.18.1":
    just build
    scripts/parity-build --name "o2sk-tst" --root "/Users/mares/Development/O2sk/NAC/tsm-environments" --env "tst" --release-id "{{release_id}}" --go-bin ./dist/kube-build-app

parity-o2-nac-dev release_id="2025.08.18.1":
    just build
    scripts/parity-build --name "o2-nac-dev" --root "/Users/mares/Development/O2/NAC/dtl-nac/environments" --env "dev" --release-id "{{release_id}}" --go-bin ./dist/kube-build-app

parity-smoke release_id="2025.08.18.1":
    just parity-cetin-test "{{release_id}}"
    just parity-o2sk-tst "{{release_id}}"
    just parity-o2-nac-dev "{{release_id}}"

clean:
    rm -rf dist
