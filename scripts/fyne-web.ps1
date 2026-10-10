param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$FyneArgs
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$moduleCache = go env GOMODCACHE
$typesettingVersion = (go list -m -f '{{.Version}}' github.com/go-text/typesetting).Trim()
$sourcePath = Join-Path $moduleCache "github.com\go-text\typesetting@$typesettingVersion\fontscan\scan.go"
$replacementPath = Join-Path $projectRoot "third_party\go-text-fontscan\scan.go.txt"
if (!(Test-Path $sourcePath) -or !(Test-Path $replacementPath)) {
    throw "Could not find the upstream fontscan source or local browser patch."
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("mytodo-web-build-" + [guid]::NewGuid().ToString("N"))
$tempMod = Join-Path $tempRoot "mytodo.mod"
$tempSum = Join-Path $tempRoot "mytodo.sum"
$forkRoot = Join-Path $tempRoot "typesetting"
$previousGoFlags = $env:GOFLAGS
$exitCode = 1
try {
    New-Item -ItemType Directory -Force $forkRoot | Out-Null
    $sourceRoot = (Resolve-Path (Join-Path $moduleCache "github.com\go-text\typesetting@$typesettingVersion")).Path
    Get-ChildItem $sourceRoot -Recurse -Filter *.go |
        Where-Object { $_.Name -notlike '*_test.go' } |
        ForEach-Object {
            $relative = $_.FullName.Substring($sourceRoot.Length + 1)
            $destination = Join-Path $forkRoot $relative
            New-Item -ItemType Directory -Force (Split-Path $destination) | Out-Null
            Copy-Item $_.FullName $destination -Force
        }
    Copy-Item (Join-Path $sourceRoot "go.mod") (Join-Path $forkRoot "go.mod")
    Copy-Item $replacementPath (Join-Path $forkRoot "fontscan\scan.go") -Force

    Copy-Item (Join-Path $projectRoot "go.mod") $tempMod
    Copy-Item (Join-Path $projectRoot "go.sum") $tempSum
    $modContents = [System.IO.File]::ReadAllText($tempMod)
    $modContents += "`nreplace github.com/go-text/typesetting => $($forkRoot.Replace('\', '/'))`n"
    [System.IO.File]::WriteAllText($tempMod, $modContents, [System.Text.UTF8Encoding]::new($false))

    $env:GOFLAGS = (($previousGoFlags + " -modfile=$tempMod").Trim())
    & fyne @FyneArgs
    $exitCode = $LASTEXITCODE
} finally {
    $env:GOFLAGS = $previousGoFlags
    Remove-Item $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

exit $exitCode
