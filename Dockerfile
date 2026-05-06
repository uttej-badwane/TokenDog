# TokenDog Docker image. Two-stage so the final image has only the static
# binary and the runtime tools td wraps (git, gh, jq, curl). Useful for
# CI pipelines that want to run td against transcripts captured elsewhere,
# or for sandboxed evaluation without a host install.
#
# Build: docker build -t tokendog .
# Run:   docker run --rm tokendog --version
# Mount your Claude transcripts: docker run --rm -v ~/.claude:/root/.claude tokendog replay

FROM golang:1.22-alpine AS build
WORKDIR /src
# Cache deps separately from sources so source-only changes don't bust the
# layer.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO disabled for a fully static binary that runs on alpine without glibc.
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X tokendog/cmd.Version=docker" -o /out/td .

# Runtime: minimal, but include the tools td actually wraps so `td git ...`
# works inside the container. If you don't need them, build FROM scratch
# and copy only the binary.
FROM alpine:3.20
RUN apk add --no-cache git curl jq ca-certificates
COPY --from=build /out/td /usr/local/bin/td
RUN ln -s /usr/local/bin/td /usr/local/bin/tokendog
ENTRYPOINT ["td"]
CMD ["--help"]
