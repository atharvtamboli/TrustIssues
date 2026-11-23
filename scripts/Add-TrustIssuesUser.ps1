param(
    [Parameter(Mandatory)][string]$Username,
    [Parameter(Mandatory)][ValidateSet('holder','issuer','verifier')][string]$Role,
    [string]$Organization = '-'
)
$ErrorActionPreference = 'Stop'
if (-not $env:FATBOT_DATA) { $env:FATBOT_DATA = Join-Path $env:LOCALAPPDATA 'TrustIssues/data' }
$password = Read-Host "Password for $Username (12 to 72 UTF-8 bytes)" -AsSecureString
$pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($password)
try {
    $OutputEncoding = New-Object System.Text.UTF8Encoding($false)
    [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer) | & (Join-Path $PSScriptRoot 'trustissues.exe') user-add $Username $Role $Organization
    if ($LASTEXITCODE -ne 0) { throw 'Account creation failed. Stop the server before creating accounts.' }
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    $password.Dispose()
}
