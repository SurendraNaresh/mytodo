# Little Years Web Archive

The visitor website is served from `web/` by `cmd/server`. The same Go process
serves the `/api/v1` API, SQLite database, uploaded media, thumbnails, and the
browser SPA. Visitors can browse without an account. Comments require an
authenticated non-Visitor account.

## Local run

```powershell
$env:MYTODO_LISTEN_ADDR = "127.0.0.1:8080"
$env:MYTODO_DATA_DIR = "$HOME\mytodo-data"
$env:MYTODO_WEB_DIR = "web"
go run ./cmd/server
```

The first migration creates the `first-year` era, its starter album, five
default layout frames, and the remaining era tables from the checked-in
blueprint. Open `http://127.0.0.1:8080`. The first account can be bootstrapped
as an administrator through the existing setup flow. In the desktop app,
administrators use **Manage archive**, **Canvas layout**, and **Upload memory**.

## Email and donations

Passwordless signup requires SMTP configuration. Without it, the API returns
`503` rather than creating an account that cannot receive its sign-in link.
Set `MYTODO_PUBLIC_URL` to the public HTTPS origin and configure `SMTP_HOST`,
`SMTP_PORT` (default `587`), `SMTP_FROM`, and optionally `SMTP_USER` and
`SMTP_PASSWORD`. Links expire after 15 minutes and can only be used once.

Set `STRIPE_PAYMENT_LINK` to a Stripe Payment Link configured to let the
customer choose the amount. The donation slot redirects to that link and
renders its QR as PNG. The server accepts only Stripe payment-link hosts.

## Media

The shared server requires `ffprobe` to verify short-video duration and
`ffmpeg` to make video thumbnails. Photos are decoded and resized in an async
thumbnail task. Shorts longer than 60 seconds are rejected, never trimmed.
Uploads are limited to 128 MiB and deduplicated by SHA-256.

Files are stored below `$MYTODO_DATA_DIR/media/<era-slug>/<album-title>-<id>/`
as `<sha256>.<ext>`. JPEG thumbnails are stored in `media/thumbs/`. The desktop
drop-folder watcher consumes
`~/mytodo-drop/<era-slug>/<album-title>/<file>`; album folder names are
case-insensitive slugs of their titles. Unsupported files and failed uploads
are left in place for correction. Successful files are removed from the drop
folder after the server confirms storage.

The Canvas layout editor writes one row for each of `top`, `bottom`, `left`,
`right`, and `center`. Feature configuration is JSON; the visitor client reads
it at runtime, so layout changes need no rebuild or redeploy. Album-grid
configuration supports `{"columns": 1}` through `{"columns": 4}` and
`{"showCaptions": false}`. Comments support `{"heading":"Family notes"}`;
the title and donation features accept a `label` value.

## Event artifacts

Artifacts attach files and descriptions to a `voting_event`. Signed-in users
can browse artifacts for events they can see with `GET /api/v1/artifacts`,
optionally filtered by `event_id`; `GET /api/v1/artifacts/{id}` returns metadata
and `GET /api/v1/artifacts/{id}/file` streams the protected file. A JWT issued
by password or magic-link login is required for these reads. Only the current
Admin role can create, update, or delete artifacts with `POST /api/v1/artifacts`,
`PUT /api/v1/artifacts/{id}`, and `DELETE /api/v1/artifacts/{id}`.

Create and update requests use multipart fields `event_id`, `title`,
`description`, and an optional replacement `file`. New artifacts require a
file. PDF, PNG, JPEG, WebP, and UTF-8 text files are accepted up to 32 MiB;
extension and content signatures are checked. Files are stored under
`$MYTODO_DATA_DIR/artifacts/` with generated names. The original filename is
metadata only, and the stored path is never returned to clients. PDFs and
images are served inline; other files are downloads. Event visibility applies
to artifact reads and downloads.

## Fly deployment

Install and authenticate `flyctl`, then set `STRIPE_PAYMENT_LINK` and the SMTP
environment variables before running `scripts/host/provision.sh`. The script
creates the app and a 1 GiB volume mounted at `/data`, installs the generated
session secret, and deploys the Docker image. Fly provides TLS for the app
hostname. `scripts/host/tls.sh` is only for a separately managed VM.

There is no currently available free tier that combines a Go service with a
persistent SQLite volume: Fly volumes are billed, and Render's free web
services do not include persistent disks. The included Fly configuration is
persistent and low-footprint, but it is not a zero-cost hosting promise.

## Backups and health

Configure an `rclone` remote, for example `r2:mytodo-backups`, and set
`BACKUP_REMOTE`. For Cloudflare R2, configure the remote with the S3 provider,
Cloudflare account endpoint, and R2 access credentials. Run
`scripts/backup.sh` daily; it uses SQLite's online `.backup`, compresses the
snapshot, uploads it, and keeps the newest 30 dated archives.

Example crontab entries (adjust paths and log destination):

```cron
0 3 * * * /opt/mytodo/scripts/backup.sh >> /var/log/mytodo-backup.log 2>&1
*/5 * * * * /opt/mytodo/scripts/host/healthcheck.sh
```

Run `scripts/restore.sh YYYY-MM-DD` only after stopping the application. It
downloads and integrity-checks the archive, snapshots the current database,
restores the selected file, compares every table's row counts, and restores the
previous database if verification fails.

`GET /healthz` pings SQLite. Schedule `scripts/host/healthcheck.sh` with
`MYTODO_HEALTHCHECK_URL`; set either `ALERT_WEBHOOK_URL` for Discord or
`TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` for Telegram notifications.

## Android

Android remains secondary to the browser release. The current Fyne desktop app
contains administrator and schema-generation surfaces; a visitor-only
Android flavor still needs build-tagged module separation and a dedicated
`fyne-cross android` build pipeline.