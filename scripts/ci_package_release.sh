#!/usr/bin/env bash
set -euo pipefail

version="${RELEASE_VERSION:-$(tr -d '[:space:]' < VERSION)}"
commit="${CI_COMMIT_SHORT_SHA:-$(git rev-parse --short HEAD 2>/dev/null || printf unknown)}"
date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package="kube-build-app"
release_dir="dist/release"

if [[ -z "${version}" ]]; then
  echo "VERSION is empty" >&2
  exit 1
fi

rm -rf "${release_dir}"
mkdir -p "${release_dir}" .tmp/go-build-cache

ldflags="-X kube-env/internal/appinfo.Version=${version} -X kube-env/internal/appinfo.Commit=${commit} -X kube-env/internal/appinfo.Date=${date}"

targets=(
  "darwin/arm64"
  "darwin/amd64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  ext=""
  if [[ "${os}" == "windows" ]]; then
    ext=".exe"
  fi

  archive_base="${package}-${version}-${os}-${arch}"
  work_dir="${release_dir}/${archive_base}"
  mkdir -p "${work_dir}"

  echo "Building ${archive_base}"
  GOCACHE="${PWD}/.tmp/go-build-cache" \
  CGO_ENABLED=0 \
  GOOS="${os}" \
  GOARCH="${arch}" \
  go build -ldflags "${ldflags}" -o "${work_dir}/kube-build-app${ext}" ./cmd/kube-build-app

  GOCACHE="${PWD}/.tmp/go-build-cache" \
  CGO_ENABLED=0 \
  GOOS="${os}" \
  GOARCH="${arch}" \
  go build -ldflags "${ldflags}" -o "${work_dir}/kube-edit-app${ext}" ./cmd/kube-edit-app

  cp README.md README.cs.md LICENSE "${work_dir}/"
  tar -C "${release_dir}" -czf "${release_dir}/${archive_base}.tar.gz" "${archive_base}"
  rm -rf "${work_dir}"
done

(
  cd "${release_dir}"
  shasum -a 256 *.tar.gz > SHA256SUMS
)
