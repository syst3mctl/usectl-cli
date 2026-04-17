# usectl CLI installer for Windows
# Usage: iwr -useb https://manager.usectl.com/releases/install.ps1 | iex

$ErrorActionPreference = 'Stop'

$Binary     = "usectl.exe"
$BaseUrl    = "https://manager.usectl.com/releases/latest"
$InstallDir = "$env:LOCALAPPDATA\usectl"

# Detect architecture
$Arch = if ([System.Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }

$Filename = "usectl-windows-$Arch.exe"
$Url      = "$BaseUrl/$Filename"

Write-Host "==> Downloading usectl for windows/$Arch..."
Write-Host "    $Url"

# Create install directory if it doesn't exist
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
}

# Download to temp file, then move into place
$TmpFile = [System.IO.Path]::GetTempFileName() + ".exe"
try {
    Invoke-WebRequest -Uri $Url -OutFile $TmpFile -UseBasicParsing
    Move-Item -Path $TmpFile -Destination "$InstallDir\$Binary" -Force
} finally {
    if (Test-Path $TmpFile) { Remove-Item $TmpFile -Force -ErrorAction SilentlyContinue }
}

# Add to user PATH if not already present
$UserPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
    Write-Host "==> Added $InstallDir to your PATH"
    Write-Host "    Restart your terminal, or run to use immediately:"
    Write-Host "      `$env:PATH += ';$InstallDir'"
}

Write-Host ""
Write-Host "==> usectl installed successfully!"
Write-Host ""
Write-Host "Get started:"
Write-Host "  usectl login"
Write-Host "  usectl --help"
