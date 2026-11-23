param([switch]$NoBrowser)
$ErrorActionPreference = 'Stop'
if (-not $env:FATBOT_DATA) {
    $env:FATBOT_DATA = Join-Path $env:LOCALAPPDATA 'TrustIssues/data'
}
$binary = Join-Path $PSScriptRoot 'trustissues.exe'
if (-not (Test-Path -LiteralPath $binary)) { throw 'Extract the portable release before starting TrustIssues.' }
if (-not $NoBrowser) {
    $address = if ($env:FATBOT_ADDR) { $env:FATBOT_ADDR } else { '127.0.0.1:5500' }
    $scheme = if ($env:FATBOT_TLS_CERT) { 'https' } else { 'http' }
    Start-Process ($scheme + '://' + $address + '/login')
}
& $binary
exit $LASTEXITCODE
