# Published-image layout. The binary is built on the host
# (CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-w" -o pogo ./cmd/pogo)
# and copied in. Other Dockerfiles can then:
#
#   COPY --from=pogo:local /usr/local/bin/pogo /usr/local/bin/pogo
#
#   docker build -f examples/docker/pogo.Dockerfile -t pogo:local .
#   docker run --rm pogo:local version

FROM scratch AS binary
COPY pogo /pogo

FROM alpine:3.20
COPY --from=binary /pogo /usr/local/bin/pogo
ENTRYPOINT ["pogo"]
CMD ["--help"]
