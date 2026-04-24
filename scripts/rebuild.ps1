# 前后端一键重建脚本
# 用法: .\scripts\rebuild.ps1

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# 刷新 PATH（确保 winget 安装的 Node.js 等工具可用）
$env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

Write-Host "=== 构建前端 ===" -ForegroundColor Cyan
Set-Location frontend
npm run build
Set-Location $root

Write-Host "=== 重建后端镜像并重启所有服务 ===" -ForegroundColor Cyan
docker compose up -d --build app
docker compose restart frontend

Write-Host "=== 完成 ===" -ForegroundColor Green
docker ps --format "table {{.Names}}`t{{.Status}}`t{{.Ports}}"
