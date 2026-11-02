# httpslatency installer for Windows: downloads the build for this CPU from 1ms.dog (which passes the GitHub release
# files on) or else from GitHub, checks its SHA-256 against the release's checksums.txt, installs it as
# %LOCALAPPDATA%\Programs\httpslatency\httpslatency.exe, adds that folder to your PATH and starts the test with its menu.
#
#   irm https://1ms.dog/i.ps1 | iex
#
# Environment: HTTPSLATENCY_VERSION (a tag such as v0.2.0; default latest), HTTPSLATENCY_DIR (install folder),
# HTTPSLATENCY_NO_RUN=1 (install only).
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'   # the progress bar makes Invoke-WebRequest many times slower

& {
  $repo = 'yt1988/https-latency-test-cli'
  $version = if ($env:HTTPSLATENCY_VERSION) { $env:HTTPSLATENCY_VERSION } else { 'latest' }
  $bases = if ($version -eq 'latest') {
    @('https://1ms.dog/dl/latest', "https://github.com/$repo/releases/latest/download")
  } else {
    @("https://1ms.dog/dl/$version", "https://github.com/$repo/releases/download/$version")
  }
  $arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') { 'arm64' } else { 'amd64' }
  $file = "httpslatency-windows-$arch.exe"
  $dir = if ($env:HTTPSLATENCY_DIR) { $env:HTTPSLATENCY_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\httpslatency' }
  $tmp = Join-Path ([IO.Path]::GetTempPath()) ("httpslatency-" + [guid]::NewGuid())
  New-Item -ItemType Directory -Force $tmp | Out-Null
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

  try {
    Write-Host "Downloading $file ($version)..."
    $got = $false
    foreach ($base in $bases) {
      try {
        Invoke-WebRequest -UseBasicParsing "$base/$file" -OutFile "$tmp\httpslatency.exe"
        Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile "$tmp\checksums.txt"
        $got = $true
        break
      } catch {
        Write-Host "  not from $base, trying the next source"
      }
    }
    if (-not $got) { throw "httpslatency install: download failed from: $($bases -join ' ')" }

    $want = (Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match " $([regex]::Escape($file))$" } | ForEach-Object { ($_ -split ' ')[0] }) | Select-Object -First 1
    if (-not $want) { throw "httpslatency install: no checksum for $file" }
    $have = (Get-FileHash -Algorithm SHA256 "$tmp\httpslatency.exe").Hash.ToLower()
    if ($have -ne $want) { throw "httpslatency install: checksum mismatch for $file (expected $want, got $have)" }

    New-Item -ItemType Directory -Force $dir | Out-Null
    Move-Item -Force "$tmp\httpslatency.exe" (Join-Path $dir 'httpslatency.exe')
  } finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }

  $exe = Join-Path $dir 'httpslatency.exe'
  Write-Host "Installed $(& $exe --version) to $exe"
  # on the user's PATH for new windows, and on this one's now
  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (-not (($userPath -split ';') -contains $dir)) {
    [Environment]::SetEnvironmentVariable('Path', ($(if ($userPath) { "$userPath;" } else { '' }) + $dir), 'User')
    Write-Host "Added $dir to your PATH."
  }
  if (-not (($env:Path -split ';') -contains $dir)) { $env:Path = "$env:Path;$dir" }
  Write-Host 'Run it again any time with: httpslatency   (httpslatency --help lists the options)'

  if (-not $env:HTTPSLATENCY_NO_RUN) {
    Write-Host ''
    & $exe
  }
}
