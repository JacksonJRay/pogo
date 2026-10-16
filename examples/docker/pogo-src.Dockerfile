# Build pogo inside Docker, then copy the static binary onto scratch.
# Requires a golang base image. The usual path on a machine that already
# has Go is examples/docker/pogo.Dockerfile, which packages the host binary.
#
#   docker build -f examples/docker/pogo-src.Dockerfile -t pogo:src .

FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-w" -o /out/pogo ./cmd/pogo

FROM scratch
COPY --from=build /out/pogo /usr/local/bin/pogo
ENTRYPOINT ["/usr/local/bin/pogo"]
CMD ["version"]
