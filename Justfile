set shell := ["bash", "-uc"]
set windows-shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command"]

default:
    @just --list

fmt:
    gofmt -w cmd internal

[unix]
go-test:
    mkdir -p .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" go test ./...

[windows]
go-test:
    New-Item -ItemType Directory -Force .tmp/go-build-cache | Out-Null
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; go test ./...

ruby-test:
    ruby -Itest test/comprehensive_features_test.rb test/encjson_api_selection_test.rb

test: go-test ruby-test

[unix]
readme-smoke:
    just build
    scripts/readme-smoke ./dist/kube-build-app

[unix]
build:
    mkdir -p dist
    mkdir -p .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" go build -o dist/kube-build-app ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" go build -o dist/kube-edit-app ./cmd/kube-edit-app
    GOCACHE="$PWD/.tmp/go-build-cache" go build -o dist/kube-ops-app ./cmd/kube-ops-app

[windows]
build:
    New-Item -ItemType Directory -Force dist, .tmp/go-build-cache | Out-Null
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; go build -o dist/kube-build-app.exe ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; go build -o dist/kube-edit-app.exe ./cmd/kube-edit-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; go build -o dist/kube-ops-app.exe ./cmd/kube-ops-app

[unix]
build-versioned version:
    mkdir -p dist .tmp/go-build-cache
    commit="$(git rev-parse --short HEAD 2>/dev/null || printf unknown)"; \
    date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"; \
    ldflags="-X kube-env/internal/appinfo.Version={{version}} -X kube-env/internal/appinfo.Commit=$commit -X kube-env/internal/appinfo.Date=$date"; \
    GOCACHE="$PWD/.tmp/go-build-cache" go build -ldflags "$ldflags" -o dist/kube-build-app ./cmd/kube-build-app; \
    GOCACHE="$PWD/.tmp/go-build-cache" go build -ldflags "$ldflags" -o dist/kube-edit-app ./cmd/kube-edit-app; \
    GOCACHE="$PWD/.tmp/go-build-cache" go build -ldflags "$ldflags" -o dist/kube-ops-app ./cmd/kube-ops-app

[windows]
build-versioned version:
    New-Item -ItemType Directory -Force dist, .tmp/go-build-cache | Out-Null
    $commit = git rev-parse --short HEAD 2>$null; if ($LASTEXITCODE -ne 0) { $commit = "unknown" }; $date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"); $ldflags = "-X kube-env/internal/appinfo.Version={{version}} -X kube-env/internal/appinfo.Commit=$commit -X kube-env/internal/appinfo.Date=$date"; $env:GOCACHE = "$PWD\.tmp\go-build-cache"; go build -ldflags $ldflags -o dist/kube-build-app.exe ./cmd/kube-build-app; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }; go build -ldflags $ldflags -o dist/kube-edit-app.exe ./cmd/kube-edit-app

[unix]
build-cross:
    mkdir -p dist .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=arm64 go build -o dist/kube-build-app-darwin-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=amd64 go build -o dist/kube-build-app-darwin-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=amd64 go build -o dist/kube-build-app-linux-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=arm64 go build -o dist/kube-build-app-linux-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=windows GOARCH=amd64 go build -o dist/kube-build-app-windows-amd64.exe ./cmd/kube-build-app

[windows]
build-cross:
    New-Item -ItemType Directory -Force dist, .tmp/go-build-cache | Out-Null
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "darwin"; $env:GOARCH = "arm64"; go build -o dist/kube-build-app-darwin-arm64 ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "darwin"; $env:GOARCH = "amd64"; go build -o dist/kube-build-app-darwin-amd64 ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "linux"; $env:GOARCH = "amd64"; go build -o dist/kube-build-app-linux-amd64 ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "linux"; $env:GOARCH = "arm64"; go build -o dist/kube-build-app-linux-arm64 ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -o dist/kube-build-app-windows-amd64.exe ./cmd/kube-build-app

[unix]
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

[windows]
release version:
    if (Test-Path dist/release) { Remove-Item -Recurse -Force dist/release }; New-Item -ItemType Directory -Force dist/release, .tmp/go-build-cache | Out-Null
    $commit = git rev-parse --short HEAD 2>$null; if ($LASTEXITCODE -ne 0) { $commit = "unknown" }; $date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"); $ldflags = "-X kube-env/internal/appinfo.Version={{version}} -X kube-env/internal/appinfo.Commit=$commit -X kube-env/internal/appinfo.Date=$date"; $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $targets = @(@("darwin", "arm64", ""), @("darwin", "amd64", ""), @("linux", "amd64", ""), @("linux", "arm64", ""), @("windows", "amd64", ".exe")); foreach ($target in $targets) { $env:GOOS = $target[0]; $env:GOARCH = $target[1]; $out = "dist/release/kube-build-app-$($target[0])-$($target[1])$($target[2])"; go build -ldflags $ldflags -o $out ./cmd/kube-build-app; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE } }; Get-FileHash dist/release/kube-build-app-* -Algorithm SHA256 | ForEach-Object { "$($_.Hash.ToLower())  $(Split-Path $_.Path -Leaf)" } | Set-Content dist/release/SHA256SUMS

[unix]
build-cross-all:
    mkdir -p dist .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=arm64 go build -o dist/kube-build-app-darwin-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=arm64 go build -o dist/kube-edit-app-darwin-arm64 ./cmd/kube-edit-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=amd64 go build -o dist/kube-build-app-darwin-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=darwin GOARCH=amd64 go build -o dist/kube-edit-app-darwin-amd64 ./cmd/kube-edit-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=amd64 go build -o dist/kube-build-app-linux-amd64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=amd64 go build -o dist/kube-edit-app-linux-amd64 ./cmd/kube-edit-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=arm64 go build -o dist/kube-build-app-linux-arm64 ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=linux GOARCH=arm64 go build -o dist/kube-edit-app-linux-arm64 ./cmd/kube-edit-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=windows GOARCH=amd64 go build -o dist/kube-build-app-windows-amd64.exe ./cmd/kube-build-app
    GOCACHE="$PWD/.tmp/go-build-cache" GOOS=windows GOARCH=amd64 go build -o dist/kube-edit-app-windows-amd64.exe ./cmd/kube-edit-app

[windows]
build-cross-all:
    New-Item -ItemType Directory -Force dist, .tmp/go-build-cache | Out-Null
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "darwin"; $env:GOARCH = "arm64"; go build -o dist/kube-build-app-darwin-arm64 ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "darwin"; $env:GOARCH = "arm64"; go build -o dist/kube-edit-app-darwin-arm64 ./cmd/kube-edit-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "darwin"; $env:GOARCH = "amd64"; go build -o dist/kube-build-app-darwin-amd64 ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "darwin"; $env:GOARCH = "amd64"; go build -o dist/kube-edit-app-darwin-amd64 ./cmd/kube-edit-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "linux"; $env:GOARCH = "amd64"; go build -o dist/kube-build-app-linux-amd64 ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "linux"; $env:GOARCH = "amd64"; go build -o dist/kube-edit-app-linux-amd64 ./cmd/kube-edit-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "linux"; $env:GOARCH = "arm64"; go build -o dist/kube-build-app-linux-arm64 ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "linux"; $env:GOARCH = "arm64"; go build -o dist/kube-edit-app-linux-arm64 ./cmd/kube-edit-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -o dist/kube-build-app-windows-amd64.exe ./cmd/kube-build-app
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; $env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -o dist/kube-edit-app-windows-amd64.exe ./cmd/kube-edit-app

[unix]
run-edit-app root listen="127.0.0.1:8080":
    mkdir -p .tmp/go-build-cache
    GOCACHE="$PWD/.tmp/go-build-cache" go run ./cmd/kube-edit-app serve --root "{{root}}" --listen "{{listen}}"

[windows]
run-edit-app root listen="127.0.0.1:8080":
    New-Item -ItemType Directory -Force .tmp/go-build-cache | Out-Null
    $env:GOCACHE = "$PWD\.tmp\go-build-cache"; go run ./cmd/kube-edit-app serve --root "{{root}}" --listen "{{listen}}"

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

[unix]
clean:
    rm -rf dist

[windows]
clean:
    if (Test-Path dist) { Remove-Item -Recurse -Force dist }
