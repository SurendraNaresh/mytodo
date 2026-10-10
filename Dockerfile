FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mytodo-server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/mytodo-server /usr/local/bin/mytodo-server
RUN mkdir -p /app/data
COPY --from=build /src/mytodo.db /app/data/mytodo.db
ENV PORT=9876 DB_FILENAME=/data/mytodo.db
EXPOSE 9876
CMD ["/usr/local/bin/mytodo-server"]