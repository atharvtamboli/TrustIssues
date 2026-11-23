param(
    [Parameter(Mandatory)][string]$Username,
    [Parameter(Mandatory)][ValidateSet("holder", "issuer", "verifier")][string]$Role,
    [string]$Organization = "-"
)
$ErrorActionPreference = "Stop"
if ($Role -eq "issuer" -and $Organization -eq "-") {
    throw "Specify -Organization for an issuing institution."
}
$password = Read-Host "Password for $Username (12 to 72 UTF-8 bytes)" -AsSecureString
$pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($password)
Push-Location -LiteralPath $PSScriptRoot
try {
    $OutputEncoding = New-Object System.Text.UTF8Encoding($false)
    [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer) | go run . user-add $Username $Role $Organization
    if ($LASTEXITCODE -ne 0) { throw "Account creation failed." }
} finally {
    Pop-Location
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    $password.Dispose()
}
