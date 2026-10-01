# Install baton on Windows.
#
#   irm https://raw.githubusercontent.com/v-kravchenko/baton/main/install.ps1 | iex
#   & ([scriptblock]::Create((irm .../install.ps1))) -Integrate claude,opencode
#
# Environment: BATON_INSTALL_DIR (default %LOCALAPPDATA%\Programs\baton),
# BATON_VERSION (a tag such as v1.2.3; default latest).
param([string[]]$Integrate = @())
$ErrorActionPreference = 'Stop'

$repo = 'v-kravchenko/baton'
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  'AMD64' { 'amd64' }
  'ARM64' { 'arm64' }
  default { throw "unsupported architecture $env:PROCESSOR_ARCHITECTURE" }
}
$asset = "baton_windows_$arch.exe"
$dir = if ($env:BATON_INSTALL_DIR) { $env:BATON_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\baton' }
$base = if ($env:BATON_VERSION) { "https://github.com/$repo/releases/download/$env:BATON_VERSION" } else { "https://github.com/$repo/releases/latest/download" }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("baton-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  Write-Host "downloading $asset"
  Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile "$tmp\baton.exe"
  Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile "$tmp\checksums.txt"
  $want = (Get-Content "$tmp\checksums.txt" | ForEach-Object {
      $f = $_ -split '\s+'
      if ($f.Count -ge 2 -and ($f[1] -eq $asset -or $f[1] -eq "*$asset")) { $f[0] }
    } | Select-Object -First 1)
  if (-not $want) { throw "checksums.txt has no entry for $asset" }
  $got = (Get-FileHash -Algorithm SHA256 "$tmp\baton.exe").Hash.ToLower()
  if ($got -ne $want.ToLower()) { throw "SHA256 mismatch for ${asset}: got $got, want $want" }

  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $dst = Join-Path $dir 'baton.exe'
  if (Test-Path $dst) {
    # A running baton.exe cannot be overwritten, but it can be renamed.
    Remove-Item -Force "$dst.old" -ErrorAction SilentlyContinue
    Rename-Item -Path $dst -NewName 'baton.exe.old'
  }
  Move-Item -Force "$tmp\baton.exe" $dst
  Write-Host "installed $dst ($(& $dst version))"
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not ($userPath -split ';' | Where-Object { $_ -eq $dir })) {
  [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ";$dir").TrimStart(';'), 'User')
  $env:Path += ";$dir"
  Write-Host "added $dir to the user PATH (open a new terminal)"
}

& (Join-Path $dir 'baton.exe') config init

foreach ($a in ($Integrate -join ',' -split ',' | Where-Object { $_ })) {
  & (Join-Path $dir 'baton.exe') integrate $a
}
