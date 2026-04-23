# syntax=docker/dockerfile:1

# ------------------------------------------------------------------------------
# Utility stage: build apply-env (static musl)
# ------------------------------------------------------------------------------
FROM rust:1.93-alpine3.22 AS apply-env-builder

ARG APPLY_ENV_REF=main

RUN apk add --no-cache musl-dev pkgconfig build-base cmake perl make git
RUN rustup target add x86_64-unknown-linux-musl

WORKDIR /src
RUN git clone --depth 1 --branch "${APPLY_ENV_REF}" https://github.com/martinmares/apply-env-rs.git apply-env-rs

WORKDIR /src/apply-env-rs
RUN cargo build --release --target x86_64-unknown-linux-musl --bins && \
    mkdir -p /out && \
    if [ -x target/x86_64-unknown-linux-musl/release/apply-env ]; then \
      cp target/x86_64-unknown-linux-musl/release/apply-env /out/apply-env; \
    else \
      echo "apply-env binary not found" >&2; \
      ls -la target/x86_64-unknown-linux-musl/release >&2; \
      exit 1; \
    fi && \
    chmod +x /out/apply-env

# ------------------------------------------------------------------------------
# Utility stage: build encjson-rs (static musl, with simple-policy-engine)
# ------------------------------------------------------------------------------
FROM rust:1.93-alpine3.22 AS encjson-builder

ARG ENCJSON_REF=main
ARG SIMPLE_POLICY_ENGINE_REF=main

RUN apk add --no-cache musl-dev pkgconfig build-base cmake perl make git
RUN rustup target add x86_64-unknown-linux-musl

WORKDIR /src
RUN git clone --depth 1 --branch "${SIMPLE_POLICY_ENGINE_REF}" https://github.com/martinmares/simple-policy-engine.git simple-policy-engine && \
    git clone --depth 1 --branch "${ENCJSON_REF}" https://github.com/martinmares/encjson-rs.git encjson-rs

WORKDIR /src/encjson-rs
RUN cargo build --release --target x86_64-unknown-linux-musl --bins && \
    mkdir -p /out && \
    if [ -x target/x86_64-unknown-linux-musl/release/encjson-rs ]; then \
      cp target/x86_64-unknown-linux-musl/release/encjson-rs /out/encjson-rs; \
    elif [ -x target/x86_64-unknown-linux-musl/release/encjson ]; then \
      cp target/x86_64-unknown-linux-musl/release/encjson /out/encjson-rs; \
    else \
      echo "encjson binary not found" >&2; \
      ls -la target/x86_64-unknown-linux-musl/release >&2; \
      exit 1; \
    fi && \
    chmod +x /out/encjson-rs

# ------------------------------------------------------------------------------
# Utility stage: build legacy encjson (crystal)
# ------------------------------------------------------------------------------
FROM crystallang/crystal:1.13.3-alpine AS encjson-legacy-builder

ARG ENCJSON_LEGACY_REF=main

RUN apk add --no-cache git build-base musl-dev openssl-dev yaml-dev zlib-dev pcre2-dev

WORKDIR /src
RUN git clone --depth 1 --branch "${ENCJSON_LEGACY_REF}" https://github.com/martinmares/encjson.git encjson-legacy

WORKDIR /src/encjson-legacy
RUN shards update && \
    shards build --production --static && \
    mkdir -p /out && \
    cp bin/encjson /out/encjson-legacy && \
    strip -s /out/encjson-legacy

# ------------------------------------------------------------------------------
# Build stage: install gems on openSUSE (with build toolchain)
# ------------------------------------------------------------------------------
FROM opensuse/tumbleweed:latest AS builder

RUN zypper -n ref && \
    zypper -n in --no-recommends \
      ca-certificates curl ruby ruby-devel gcc gcc-c++ make patch tar gzip xz && \
    zypper -n clean -a

RUN RUBY_API="$(ruby -e 'print RbConfig::CONFIG["ruby_version"]')" && \
    mkdir -p "/bundle/ruby/${RUBY_API}" && \
    gem install --no-document bundler -v 2.4.12 && \
    gem install --no-document --install-dir "/bundle/ruby/${RUBY_API}" ostruct

WORKDIR /src

COPY Gemfile Gemfile.lock ./
RUN RUBY_API="$(ruby -e 'print RbConfig::CONFIG["ruby_version"]')" && \
    export GEM_PATH="/bundle/ruby/${RUBY_API}" && \
    bundle config set path /bundle && \
    bundle config set without "development:test" && \
    bundle install --jobs 1 --retry 3

COPY . .

# ------------------------------------------------------------------------------
# Runtime stage: openSUSE + ruby + app code + vendored gems
# ------------------------------------------------------------------------------
FROM opensuse/tumbleweed:latest

RUN zypper -n ref && \
    zypper -n in --no-recommends ca-certificates curl shadow ruby && \
    zypper -n clean -a

# app:app (uid/gid 1001), home=/app
RUN groupadd -g 1001 app && \
    useradd -u 1001 -g 1001 -d /app -m -s /sbin/nologin app

WORKDIR /app/kube_build_app

COPY --from=builder /bundle /bundle
COPY --from=builder /src /app/kube_build_app

RUN mkdir -p /app/bin
COPY --from=apply-env-builder /out/apply-env /app/bin/apply-env
COPY --from=encjson-builder /out/encjson-rs /app/bin/encjson-rs
COPY --from=encjson-legacy-builder /out/encjson-legacy /app/bin/encjson-legacy

ENV HOME=/app \
    GEM_HOME=/bundle \
    GEM_PATH=/bundle \
    BUNDLE_PATH=/bundle \
    BUNDLE_GEMFILE=/app/kube_build_app/Gemfile \
    ENCJSON_PATH=/app/bin/encjson-rs \
    ENCJSON_LEGACY_PATH=/app/bin/encjson-legacy \
    PATH=/app/bin:/app/kube_build_app/bin:/bundle/bin:$PATH

# Ownership + OpenShift-friendly write permissions.
# OpenShift may run with random UID; granting group 0 parity keeps /app writable.
RUN chown -R 1001:1001 /app /bundle && \
    chgrp -R 0 /app /bundle && \
    chmod -R g=u /app /bundle

USER 1001

ENTRYPOINT ["bundle", "exec", "ruby", "bin/kube_build_app"]
CMD ["--help"]
