# Browser font scan compatibility

`scan.go.txt` is based on `github.com/go-text/typesetting/fontscan/scan.go` v0.3.4.
The upstream file imports `os/user`, which does not compile for Go's `js/wasm`
target. The local copy replaces `user.Current()` with `os.UserHomeDir()` and
skips host font scanning in browsers, where font discovery must come from the
browser renderer rather than a local filesystem.

`scripts/fyne-web.ps1` creates a temporary copy of the dependency with this
single-file patch and passes it through a temporary `-modfile` for Fyne web
packaging only. No dependency source in the module cache or regular module graph
is modified.
