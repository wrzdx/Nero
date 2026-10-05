FROM golang:1.26.4-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/cmd/messenger/exe /app/cmd/messenger

FROM alpine:3.23.5
RUN apk add --no-cache tzdata
WORKDIR /app
COPY --from=builder /app/cmd/messenger/exe /app
COPY --from=builder /app/web/static /app/web/static
CMD ["/app/exe"]