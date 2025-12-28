# build
FROM golang:1.22.2-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/flakeguard ./cmd/flakeguard

# runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/flakeguard /app/flakeguard
COPY web /app/web
COPY migrations /app/migrations
ENV FG_HTTP_ADDR=:8080
EXPOSE 8080
CMD ["/app/flakeguard"]
