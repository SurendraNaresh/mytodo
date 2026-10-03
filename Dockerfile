FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mytodo-server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg sqlite3 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/mytodo-server /usr/local/bin/mytodo-server
COPY web /app/web
ENV MYTODO_LISTEN_ADDR=0.0.0.0:8080 MYTODO_DATA_DIR=/data MYTODO_WEB_DIR=/app/web
EXPOSE 8080
CMD ["/usr/local/bin/mytodo-server"]