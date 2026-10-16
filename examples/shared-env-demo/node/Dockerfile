# Node app for the shared-env demo.
# Build from the repository root so the static pogo binary is in the context:
#
#   CGO_ENABLED=0 go build -ldflags="-s -w" -o pogo ./cmd/pogo
#   docker build -f examples/shared-env-demo/node/Dockerfile -t pogo-shared-node .
#
# The first stage only holds the binary. The runtime stage is Alpine plus node.
# pogo is the entrypoint, so after exec the Node process is PID 1.
# The passphrase is piped to --key-stdin. There is no passphrase file.

FROM alpine:3.20 AS pogo
COPY pogo /pogo

FROM alpine:3.20
RUN apk add --no-cache nodejs
COPY --from=pogo /pogo /usr/local/bin/pogo
COPY examples/shared-env-demo/node/package.json /app/package.json
COPY examples/shared-env-demo/node/server.js /app/server.js
WORKDIR /app
ENV HOST=0.0.0.0
ENTRYPOINT ["pogo","run","-f","/app/.env.enc","--key-stdin","--"]
CMD ["node","server.js"]
