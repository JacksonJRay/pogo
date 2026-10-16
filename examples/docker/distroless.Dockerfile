# Distroless static image. No shell. Runs as the nonroot user.
# The entrypoint prompts (docker run -it). It does not read a passphrase file.
#
#   docker build -f examples/docker/distroless.Dockerfile -t pogo-entrypoint-distroless .

FROM scratch AS bins
COPY pogo /pogo
COPY examples/docker/pid1/pid1 /pid1

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=bins /pogo /usr/local/bin/pogo
COPY --from=bins /pid1 /app/pid1
USER nonroot
ENTRYPOINT ["/usr/local/bin/pogo","run","-f","/app/.env.enc","--"]
CMD ["/app/pid1"]
