# 同步运行时资源到 build/bin（便携运行 / 打包前）
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$src = Join-Path $root "assets"
$dst = Join-Path $root "build\bin\assets"

if (-not (Test-Path (Join-Path $src "bin\upscayl-bin.exe"))) {
  throw "缺少 assets/bin/upscayl-bin.exe，请先放入运行时资源"
}

New-Item -ItemType Directory -Force -Path $dst | Out-Null
robocopy (Join-Path $src "bin") (Join-Path $dst "bin") /E /NFL /NDL /NJH /NJS /nc /ns /np | Out-Null
robocopy (Join-Path $src "models") (Join-Path $dst "models") /E /NFL /NDL /NJH /NJS /nc /ns /np | Out-Null
# robocopy 成功码 0-7
if ($LASTEXITCODE -ge 8) {
  throw "同步 assets 失败，robocopy exit=$LASTEXITCODE"
}
$global:LASTEXITCODE = 0
Write-Host "已同步 assets -> build\bin\assets"
