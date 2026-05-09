# install.ps1 — Bootstrap installer for the ndf CLI on Windows.
#
# Detects the architecture, downloads the matching binary from the latest
# GitHub Release into %LOCALAPPDATA%\Programs\nandu\ndf.exe, and adds that
# directory to the user's PATH if it isn't there already. Idempotent.
#
# Usage (PowerShell):
#   iwr -useb https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.ps1 | iex
#
# Specific version:
#   $env:NDF_VERSION = "v2.0.0"; iwr -useb ... | iex
#
# Notes:
#   - No admin rights required — installs into per-user %LOCALAPPDATA%.
#   - Adds to user PATH (HKCU), not system PATH (HKLM). Restart terminal
#     after install for the PATH change to take effect.
#   - Until v2.0.0 binaries are signed, Windows SmartScreen will prompt
#     once on first run ("More info → Run anyway"). v2.0.1 will be signed
#     under nandu.ai GmbH's Authenticode certificate; the prompt disappears
#     thereafter.

$ErrorActionPreference = "Stop"

$Repo    = "nandu-org/nandu-dev-framework-cli"
$BinDir  = Join-Path $env:LOCALAPPDATA "Programs\nandu"
$BinPath = Join-Path $BinDir "ndf.exe"

function Write-Info($msg) { Write-Host "install: $msg" }
function Write-Err($msg)  { Write-Host "install: error: $msg" -ForegroundColor Red; exit 1 }

# ---------- arch detection ----------
$arch = $env:PROCESSOR_ARCHITECTURE
switch -Regex ($arch) {
    "AMD64|x64"     { $goarch = "amd64" }
    "ARM64"         {
        Write-Err "Windows ARM64 is not yet a release target. Manually download from https://github.com/$Repo/releases or build from source."
    }
    default         {
        Write-Err "unsupported architecture: $arch. Manually download from https://github.com/$Repo/releases"
    }
}
$artifact = "ndf-windows-$goarch.exe"
Write-Info "detected windows/$goarch"

# ---------- resolve version ----------
$version = if ($env:NDF_VERSION) { $env:NDF_VERSION } else { $null }
if (-not $version) {
    Write-Info "resolving latest release..."
    try {
        $release = Invoke-RestMethod -UseBasicParsing -Uri "https://api.github.com/repos/$Repo/releases/latest"
        $version = $release.tag_name
    } catch {
        Write-Err "could not resolve latest release. Set `$env:NDF_VERSION = 'vX.Y.Z' and retry, or check https://github.com/$Repo/releases"
    }
    if (-not $version) {
        Write-Err "GitHub returned no tag_name for the latest release."
    }
}
Write-Info "installing $version"

# ---------- download ----------
$downloadUrl = "https://github.com/$Repo/releases/download/$version/$artifact"
$checksumUrl = "https://github.com/$Repo/releases/download/$version/checksums.txt"

if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
}
$tmp = Join-Path $env:TEMP "ndf-install-$(Get-Random).exe"

Write-Info "downloading $artifact..."
try {
    Invoke-WebRequest -UseBasicParsing -Uri $downloadUrl -OutFile $tmp
} catch {
    Write-Err "download failed: $downloadUrl  ($_)"
}

# ---------- verify checksum ----------
Write-Info "verifying checksum..."
try {
    $checksums = Invoke-RestMethod -UseBasicParsing -Uri $checksumUrl
    $expectedLine = ($checksums -split "`n") | Where-Object { $_ -match "\s+$([regex]::Escape($artifact))\s*$" } | Select-Object -First 1
    if ($expectedLine) {
        $expectedSha = ($expectedLine -split "\s+")[0].Trim().ToLower()
        $gotSha = (Get-FileHash -Algorithm SHA256 -Path $tmp).Hash.ToLower()
        if ($gotSha -ne $expectedSha) {
            Remove-Item $tmp -Force
            Write-Err "checksum mismatch! expected $expectedSha, got $gotSha. Refusing to install."
        }
        Write-Info "checksum ok"
    } else {
        Write-Info "no checksum available for $artifact in checksums.txt; skipping verification."
    }
} catch {
    Write-Info "could not fetch checksums.txt; skipping verification."
}

# ---------- install ----------
Move-Item -Path $tmp -Destination $BinPath -Force
Write-Info "installed $BinPath"

# ---------- PATH setup ----------
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
$pathDirs = @()
if ($userPath) { $pathDirs = $userPath -split ";" | Where-Object { $_ -ne "" } }

$alreadyOnPath = $false
foreach ($d in $pathDirs) {
    if ($d.TrimEnd("\") -ieq $BinDir.TrimEnd("\")) {
        $alreadyOnPath = $true
        break
    }
}

if ($alreadyOnPath) {
    Write-Info "$BinDir is already on user PATH; nothing to add."
} else {
    $newPath = if ($userPath) { "$userPath;$BinDir" } else { $BinDir }
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
    Write-Info "added $BinDir to user PATH"
    Write-Info "RESTART your terminal (or open a new PowerShell window) for the PATH change to take effect."
}

# ---------- verify ----------
Write-Host ""
Write-Info "install complete."
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1) Open a new PowerShell window (so PATH picks up the new entry)."
Write-Host "  2) ndf version              (verify install)"
Write-Host "  3) ndf login                (set your tokens — interactive, hidden input)"
Write-Host "  4) cd <project>; ndf init --fieldnotes-repo=<owner/repo>"
Write-Host ""
Write-Host "First-run note: Windows SmartScreen may show 'Windows protected your"
Write-Host "PC' the first time you run ndf.exe. Click 'More info' then 'Run anyway'."
Write-Host "v2.0.1 will be code-signed; this prompt will disappear after that update."
