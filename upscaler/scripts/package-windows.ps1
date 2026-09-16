# 构建 Windows 安装包（含 upscayl-bin 与模型）
$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)

& "$PSScriptRoot\sync-assets.ps1"

Write-Host "开始 wails build -nsis ..."
wails build -nsis
if ($LASTEXITCODE -ne 0) {
  throw "wails build -nsis 失败"
}

& "$PSScriptRoot\sync-assets.ps1"
Write-Host "完成。安装包与可执行文件在 build\bin\"
Get-ChildItem "build\bin" | Format-Table Name, Length, LastWriteTime
