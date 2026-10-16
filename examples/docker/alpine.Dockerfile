# Alpine image with pogo as PID 1.
# Build from the repository root after producing both static binaries:
#
#   CGO_ENABLED=0 go build -ldflags="-s -w" -o pogo ./cmd/pogo
#   CGO_ENABLED=0 go build -ldflags="-s -w" -o examples/docker/pid1/pid1 ./examples/docker/pid1
#   docker build -f examples/docker/alpine.Dockerfile -t pogo-entrypoint-alpine .
#
# The entrypoint prompts on a terminal (docker run -it). It does not read a
# passphrase file. For a pipe, override the entrypoint with --key-stdin.
# See: pogo help docker
#
#   docker run -it --rm -v "$PWD/.env.enc:/app/.env.enc:ro" pogo-entrypoint-alpine

FROM alpine:3.20 AS bins
COPY pogo /pogo
COPY examples/docker/pid1/pid1 /pid1

FROM alpine:3.20
COPY --from=bins /pogo /usr/local/bin/pogo
COPY --from=bins /pid1 /app/pid1
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--"]
CMD ["/app/pid1"]
