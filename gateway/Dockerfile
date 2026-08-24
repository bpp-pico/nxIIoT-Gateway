FROM golang:1.25-alpine AS build

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/gateway ./cmd/gateway

FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates
WORKDIR /app
COPY --from=build /out/gateway ./gateway
COPY migrations ./migrations
COPY configs ./configs

EXPOSE 8080
ENTRYPOINT ["./gateway", "-config", "configs/config.yaml", "-migrations", "migrations"]
