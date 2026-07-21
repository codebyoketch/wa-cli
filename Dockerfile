# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache dependency downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# modernc.org/sqlite is pure Go — no cgo needed, so this builds cleanly
# on Alpine without a C toolchain. VERSION/COMMIT/DATE are passed in as
# build args by the release workflow (GoReleaser's docker builder sets
# these); local `docker build` without them just gets "dev"/"none"/
# "unknown", matching internal/version's own zero-value defaults.
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w \
      -X github.com/codebyoketch/wa-cli/internal/version.Version=${VERSION} \
      -X github.com/codebyoketch/wa-cli/internal/version.Commit=${COMMIT} \
      -X github.com/codebyoketch/wa-cli/internal/version.BuildDate=${DATE}" \
    -o /out/wa .

# --- runtime stage -------------------------------------------------------
FROM alpine:3.20
# ca-certificates: whatsmeow talks TLS to WhatsApp's servers.
# tzdata: message timestamps are rendered in local time (see
# ARCHITECTURE.md / cmd output formatting) — without it the container
# has no timezone database to resolve against.
RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /out/wa /usr/local/bin/wa

# wa-cli's own default config dir is $XDG_CONFIG_HOME/wa (see
# internal/config.Dir); pointing XDG_CONFIG_HOME at a single volume
# keeps both the config file and the SQLite session/device store (and
# therefore your WhatsApp login) in one place to mount and persist —
# without this volume, `wa login` would need to be redone every
# container restart.
ENV XDG_CONFIG_HOME=/data
VOLUME /data

ENTRYPOINT ["wa"]
CMD ["--help"]
