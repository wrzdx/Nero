FROM node:22-bookworm-slim AS frontend
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY web ./web
COPY internal/web/views ./internal/web/views
COPY scripts/build-assets.mjs ./scripts/build-assets.mjs
RUN npm run css:build

FROM golang:1.26.4-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/static /app/web/static
COPY --from=frontend /app/internal/web/views/assets_generated.go /app/internal/web/views/assets_generated.go
RUN go tool templ generate
RUN CGO_ENABLED=0 go build -o /app/cmd/nero/exe /app/cmd/nero

FROM alpine:3.23.5
RUN apk add --no-cache tzdata
WORKDIR /app
COPY --from=builder /app/cmd/nero/exe /app
COPY --from=builder /app/web/static /app/web/static
CMD ["/app/exe"]
