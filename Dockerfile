FROM golang:1.26.1-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN resolved_arch="${TARGETARCH:-$(go env GOARCH)}" && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$resolved_arch" GOTOOLCHAIN=local \
    go build -trimpath -ldflags="-s -w" -o /out/mushroomchain ./cmd/server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/mushroomchain /usr/local/bin/mushroomchain
RUN mkdir -p /data && chown app:app /data
USER app
ENV HTTP_ADDR=:8080 DATABASE_PATH=/data/mushroomchain.db LOG_LEVEL=info
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/mushroomchain"]
