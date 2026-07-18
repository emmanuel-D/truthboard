# Build a shared Truthboard board image. The board derives everything
# from a git clone, so the runtime image is just the binary plus git.
#
#   docker build -t truthboard .
#   docker run -p 1337:1337 -e REPO_URL=https://github.com/you/project truthboard
#
# See docs/deploy.md for systemd, Docker, and Coolify walkthroughs.

FROM golang:1.26-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/truthboard ./cmd/truthboard

FROM alpine:3.22
RUN apk add --no-cache git ca-certificates \
    && adduser -D board \
    && mkdir /repo && chown board /repo \
    # A mounted clone is usually owned by the host user, not `board`.
    && git config --system safe.directory '*'
COPY --from=build /out/truthboard /usr/local/bin/truthboard
COPY docker/entrypoint.sh /usr/local/bin/board-entrypoint
USER board
EXPOSE 1337
ENTRYPOINT ["/usr/local/bin/board-entrypoint"]
