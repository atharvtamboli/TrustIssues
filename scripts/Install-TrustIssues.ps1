param()
$ErrorActionPreference = 'Stop'
$destination = Join-Path $env:LOCALAPPDATA 'Programs/TrustIssues'
New-Item -ItemType Directory -Force -Path $destination | Out-Null
foreach ($name in @('trustissues.exe','Start-TrustIssues.ps1','Add-TrustIssuesUser.ps1','README.md','VERSION')) {
    $source = Join-Path $PSScriptRoot $name
    if (-not (Test-Path -LiteralPath $source)) { throw "Missing release file $name. Extract the full release archive." }
    $target = Join-Path $destination $name
    if ([IO.Path]::GetFullPath($source) -ne [IO.Path]::GetFullPath($target)) {
        Copy-Item -LiteralPath $source -Destination $target -Force
    }
}
$menu = Join-Path ([Environment]::GetFolderPath('Programs')) 'TrustIssues.lnk'
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($menu)
$shortcut.TargetPath = Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe'
$shortcut.Arguments = '-NoProfile -File "' + (Join-Path $destination 'Start-TrustIssues.ps1') + '"'
$shortcut.WorkingDirectory = $destination
$shortcut.Description = 'TrustIssues credential and career workspace'
$shortcut.Save()
Write-Host "Installed in $destination. Create accounts with Add-TrustIssuesUser.ps1, then open TrustIssues from Start."
