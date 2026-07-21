# syntax=docker/dockerfile:1
#
# This Dockerfile is built by GoReleaser's dockers_v2 pipe (see
# .goreleaser.yaml), which builds the Go binary itself (once per
# platform, in the builds section) and hands this file only a
# pre-built binary to copy in — it does NOT run `go build` here.
# That avoids compiling the binary twice (once for the release
# archives, once for the Docker image) for what would otherwise be
# the same output.
#
# One consequence: `docker build .` on its own, with nothing else run
# first, will fail — there's no linux/<arch>/wa for COPY to find. To
# build and test the image locally, run the whole pipeline instead
# (which builds the binaries, populates this context correctly, and
# builds the image, without publishing anywhere):
#
#   goreleaser release --snapshot --skip=publish --clean
#
# ca-certificates: whatsmeow talks TLS to WhatsApp's servers.
# tzdata: message timestamps render in local time (see cmd output
# formatting) — without it the container has no timezone database.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/wa /usr/local/bin/wa

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
