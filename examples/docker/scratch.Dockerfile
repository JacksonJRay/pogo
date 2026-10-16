# Distroless-style scratch image. No shell, no package manager.
# The app and pogo are both static binaries, so scratch can run them.
# Same entrypoint contract as the Alpine image.
#
#   docker build -f examples/docker/scratch.Dockerfile -t pogo-entrypoint-scratch .

FROM scratch AS bins
COPY pogo /pogo
COPY examples/docker/pid1/pid1 /pid1

FROM scratch
COPY --from=bins /pogo /usr/local/bin/pogo
COPY --from=bins /pid1 /app/pid1
ENTRYPOINT ["/usr/local/bin/pogo","run","-f","/app/.env.enc","--"]
CMD ["/app/pid1"]
