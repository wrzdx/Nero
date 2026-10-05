param(
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$')]
    [string]$ReleaseId = ('{0}-nero' -f (Get-Date).ToUniversalTime().ToString('yyyyMMdd-HHmmss')),
    [ValidateSet('amd64', 'arm64')]
    [string]$Architecture = 'amd64'
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$release = Join-Path $repo "out/releases/$ReleaseId"
$archive = "$release.tar.gz"
if ((Test-Path -LiteralPath $release) -or (Test-Path -LiteralPath $archive)) {
    throw "Release already exists: $ReleaseId. Choose a new ID."
}

Push-Location $repo
try {
    $npm = if ($IsWindows -or $env:OS -eq 'Windows_NT') { 'npm.cmd' } else { 'npm' }
    & $npm run build:ui
    if ($LASTEXITCODE -ne 0) { throw 'UI build failed.' }
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }

    New-Item -ItemType Directory -Path (Join-Path $release 'bin') -Force | Out-Null
    $previousGoOS, $previousGoArch, $previousCGO = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
    try {
        $env:GOOS = 'linux'
        $env:GOARCH = $Architecture
        $env:CGO_ENABLED = '0'
        & go build -trimpath -o (Join-Path $release 'bin/nero') ./cmd/nero
        if ($LASTEXITCODE -ne 0) { throw 'Linux binary build failed.' }
    } finally {
        $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $previousGoOS, $previousGoArch, $previousCGO
    }

    Copy-Item -LiteralPath (Join-Path $repo 'deploy/Dockerfile') -Destination $release
    Copy-Item -LiteralPath (Join-Path $repo 'deploy/compose.yaml') -Destination $release
    Copy-Item -LiteralPath (Join-Path $repo 'migrations') -Destination $release -Recurse
    New-Item -ItemType Directory -Path (Join-Path $release 'web') | Out-Null
    Copy-Item -LiteralPath (Join-Path $repo 'web/static') -Destination (Join-Path $release 'web') -Recurse
    Copy-Item -LiteralPath (Join-Path $repo 'docs') -Destination $release -Recurse
    Copy-Item -LiteralPath (Join-Path $repo 'README.md') -Destination $release

    $commit = & git -c "safe.directory=$repo" rev-parse HEAD
    if ($LASTEXITCODE -ne 0) { throw 'Cannot record source commit.' }
    $workingTree = & git -c "safe.directory=$repo" status --porcelain
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read working tree state.' }
    [ordered]@{
        releaseId = $ReleaseId
        builtAtUtc = (Get-Date).ToUniversalTime().ToString('o')
        goOS = 'linux'
        goArch = $Architecture
        sourceCommit = $commit.Trim()
        workingTreeModified = [bool]$workingTree
        binarySHA256 = (Get-FileHash -LiteralPath (Join-Path $release 'bin/nero') -Algorithm SHA256).Hash.ToLowerInvariant()
        cssSHA256 = (Get-FileHash -LiteralPath (Join-Path $release 'web/static/css/app.css') -Algorithm SHA256).Hash.ToLowerInvariant()
    } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $release 'release.json') -Encoding utf8

    & tar -czf $archive -C $release .
    if ($LASTEXITCODE -ne 0) { throw 'Release archive failed.' }
    $archiveHash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    [System.IO.File]::WriteAllText("$archive.sha256", "$archiveHash  $ReleaseId.tar.gz`n", [System.Text.Encoding]::ASCII)
    Write-Output "Release: $ReleaseId ($Architecture)"
    Write-Output "Archive: $archive"
    Write-Output "SHA256: $archiveHash"
} finally {
    Pop-Location
}
