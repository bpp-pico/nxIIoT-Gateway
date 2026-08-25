FROM golang:1.25-alpine AS build

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/internal-server .

FROM alpine:3.20
COPY --from=build /out/internal-server /internal-server
EXPOSE 9100
ENTRYPOINT ["/internal-server"]
