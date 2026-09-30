# Browser and shared database deployment

The browser build is a Fyne WebAssembly client. It does not open `wasm/mytodo.db`
or any other SQLite file in the web root: static files are read-only browser
resources, not a writable database location. Browser WASM sends authenticated
domain requests to the Go API, which owns the SQLite database. Browser users
and native clients configured for the server share that data.

## Build and serve

The installed Fyne web packaging command currently fails because
`go-text/typesetting/fontscan/scan.go` imports `os/user`, which is unavailable
for Go's `js/wasm` target. Use the wrapper, which creates a temporary patched
dependency copy and temporary module file without changing `go.mod` or the Go
module cache:

```powershell
.\scripts\fyne-web.ps1 package -os web -app-id com.fyneio.mytodo -name mysoc
```

Copy the generated files from `wasm/` into the server's `web/` directory. The
server serves those static assets and `/api/v1` from the same origin, avoiding
browser CORS configuration.

For a single-process deployment, start the server with:

```powershell
$env:MYTODO_LISTEN_ADDR = "127.0.0.1:8080"
$env:MYTODO_DATA_DIR = "C:\mytodo-data"
go run ./cmd/server
```

To run the API as a separate service, start the API-only command instead:

```powershell
$env:MYTODO_LISTEN_ADDR = "127.0.0.1:8081"
$env:MYTODO_DATA_DIR = "C:\mytodo-data"
go run ./cmd/api
```

Configure the web server or reverse proxy to send `/api/v1/` requests to
`http://127.0.0.1:8081/api/v1/` and serve all other paths from the packaged
Fyne web files. Keep the API and browser on the same public origin; the browser
client currently derives its API URL from `window.location.origin`, so direct
cross-origin API hosting is not configured. Confirm routing before trying to
log in: `GET https://your-host/api/v1/status` must return JSON, not a static-host
501 response. Do not route `/api/v1/` to the static-file handler.

For deployment, use HTTPS behind a reverse proxy, set a persistent
`MYTODO_DATA_DIR`, and configure the listener/reverse proxy deliberately. Do
not expose the development server directly to the public internet.

## Native client

The native app keeps local SQLite by default. To make it use the shared server,
set `MYTODO_API_URL` to the API root before starting it:

```powershell
$env:MYTODO_API_URL = "https://todo.example.com/api/v1"
.\mytodo.exe
```

Existing local databases are not uploaded automatically. A signed-in server
administrator can use **Import SQLite database** to explicitly replace the
server database with a verified SQLite backup (maximum upload size: 256 MiB).
Import invalidates active sessions.

The authenticated API also provides `GET /api/v1/admin/backup` for an
administrator-only consistent database snapshot. Restore/import remains
`POST /api/v1/admin/import`. The API runs database initialization and migrations
on startup; `internal/db/migration_test.go` is test code, not a production API
surface.

The app's local Windows database path is `%LOCALAPPDATA%\mytodo\mytodo.db`;
`MYTODO_DATA_DIR` overrides it. Browser users never open this file directly.
