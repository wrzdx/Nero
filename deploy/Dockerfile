# Build the Linux binary locally; do not run the Go compiler on a small VPS.
FROM alpine:3.23.5
WORKDIR /app
COPY --chmod=0555 bin/nero /app/nero
COPY web/static /app/web/static
RUN apk add --no-cache tzdata
USER 10001:10001
ENTRYPOINT ["/app/nero"]
