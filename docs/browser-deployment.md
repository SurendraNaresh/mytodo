# Browser and shared database deployment

The browser build is a Fyne WebAssembly client. It does not open a SQLite file;
the Go API server owns the shared database. Browser users and native clients
configured for that server share the hosted data.

## Build and serve

The installed Fyne web packaging command currently fails because
`go-text/typesetting/fontscan/scan.go` imports `os/user`, which is unavailable
for Go's `js/wasm` target. Use the wrapper, which creates a temporary patched
dependency copy and temporary module file without changing `go.mod` or the Go
module cache:

```powershell
.\scripts\fyne-web.ps1 package -os web -app-id com.fyneio.mytodo -name mysoc
```

Serve the generated files from a static host or reverse proxy and route API
requests to the Go server. The Go server is API-only and does not serve `web/`.

For a single-process deployment, start the server with:

```powershell
$env:PORT = "9876"
$env:DB_FILENAME = "C:\mytodo-data\mytodo.db"
go run ./cmd/server
```

To run the API as a separate service, start the API-only command instead:

```powershell
$env:PORT = "9876"
$env:DB_FILENAME = "C:\mytodo-data\mytodo.db"
go run ./cmd/api
```

The browser client uses `window.location.origin` by default. For a separately
hosted UI, pass the API root in the page URL as `?api=<URL-encoded-api-root>`;
the API server must allow the UI origin in `CORS_ORIGINS`. Confirm routing
before logging in:
`GET https://your-host/api/v1/status` must return JSON.

For deployment, use HTTPS behind a reverse proxy, set a persistent
`DB_FILENAME`, and configure the listener deliberately. Do not expose the
development server directly to the public internet.

## Native client

The native app uses `local_data.db` for client-local settings, theme, profile,
and event cache. It connects to the shared server using `API_URL` in `.config`:

```powershell
$env:API_URL = "https://todo.example.com/api/v1"
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

The server database defaults to `./data/mytodo.db`. If it does not exist, the
starter `mytodo.db` is copied there before migrations run. `.config` supports
`PORT`, `DB_FILENAME`, `Local_Data_file`, `API_URL`, and `CORS_ORIGINS`;
environment variables override file values.

## Android Client

With Android SDK and NDK installed, build a release APK using a lowercase Fyne
target. Do not pass Go build flags such as `-p 8` to `--tags`:

```powershell
Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
fyne package --target android/arm64 --source-dir . --release --app-id com.mytodo.desktop
```

The Android app stores client data in its private Fyne storage directory. On
the login screen, enter `http://<server-ip>:9876/api/v1` in **API server URL**
and select **Check server**; the URL is saved on that device. `127.0.0.1` on a
phone refers to the phone, not the computer running the server. An `.config`
file on the development PC is not automatically copied into the APK.

If the APK still closes, connect the device with USB debugging enabled and
capture its startup log:

```powershell
adb logcat -c
adb shell monkey -p com.mytodo.desktop 1
adb logcat -d | Select-String -Pattern 'FATAL EXCEPTION|panic:|MyTodo|mytodo'
```

## Windows LAN access

The API listens on all interfaces at port `9876`. On the server PC, use
`ipconfig` to find its Wi-Fi/Ethernet IPv4 address. From another device on the
same non-guest network, check `http://192.168.1.10:9876/healthz` and
`http://192.168.1.10:9876/api/v1/status`, replacing the example address with
the server PC's address.

If Windows Firewall blocks the connection, allow inbound TCP 9876 on the
Private profile from an elevated PowerShell prompt:

```powershell
New-NetFirewallRule -DisplayName "MyTodo API 9876" -Direction Inbound -Action Allow -Protocol TCP -LocalPort 9876 -Profile Private
```

To serve a generated WASM directory from Python on port 9786, configure the
server's `.config` with the exact UI origin, for example
`CORS_ORIGINS=http://192.168.1.10:9786`, then run:

```powershell
python -m http.server 9786 --bind 0.0.0.0 --directory .\wasm-dir
```

Open the UI with the API root explicitly set (the example URL is already
encoded):

```text
http://192.168.1.10:9786/?api=http%3A%2F%2F192.168.1.10%3A9876%2Fapi%2Fv1
```

Allow inbound TCP 9786 as well if the other devices cannot reach the Python
static server. Use a trusted LAN only; use HTTPS and narrow `CORS_ORIGINS` for
non-local deployments.

## Native build target

Before building the Windows desktop or server, make sure a previous WASM build
has not left target variables in the shell:

```powershell
Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
go env GOOS GOARCH CGO_ENABLED GOFLAGS
go build -trimpath -ldflags="-s -w" -o .\mytodo-server.exe .\cmd\server
go build -p 8 -v -o .\mytodo-desktop.exe .
```

The native target should report `windows` and `amd64`. `GOOS=js` intentionally
excludes the server entry point and cannot build the Windows `os/user` support
used by the desktop's Fyne font scanning dependencies.
