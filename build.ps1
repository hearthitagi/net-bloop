#Requires -Version 5.1
<#
.SYNOPSIS
  构建 net-bloop：Windows 试跑版、Linux 静态二进制，以及可 docker load 的镜像 tar。
.EXAMPLE
  .\build.ps1
  .\build.ps1 -Version 0.2.6
#>
param(
  [string]$Version = "0.2.6",
  [switch]$SkipImage
)

$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $MyInvocation.MyCommand.Path)

$env:GOTOOLCHAIN = "local"
if (-not $env:GOPROXY) { $env:GOPROXY = "https://goproxy.cn,direct" }

$go = $env:GO
if (-not $go) {
  $cmd = Get-Command go -ErrorAction SilentlyContinue
  if ($cmd) { $go = $cmd.Source }
}
if (-not $go -or -not (Test-Path $go)) {
  throw "未找到 go。请把 Go 加入 PATH，或设置 `$env:GO='C:\path\to\go.exe'"
}

Write-Host "==> Go: $go"
& $go version

New-Item -ItemType Directory -Force build, build\windows-amd64, build\linux-amd64, dist | Out-Null

Write-Host "==> 构建 Windows amd64"
$env:CGO_ENABLED = "0"; $env:GOOS = "windows"; $env:GOARCH = "amd64"
& $go build -trimpath -ldflags "-s -w -X main.version=$Version" -o build\windows-amd64\net-bloop.exe ./cmd/net-bloop
if ($LASTEXITCODE -ne 0) { throw "Windows 构建失败" }

Write-Host "==> 构建 Linux amd64（静态）"
$env:CGO_ENABLED = "0"; $env:GOOS = "linux"; $env:GOARCH = "amd64"
& $go build -trimpath -ldflags "-s -w -X main.version=$Version" -o build\linux-amd64\net-bloop ./cmd/net-bloop
if ($LASTEXITCODE -ne 0) { throw "Linux 构建失败" }

if ($SkipImage) {
  Write-Host "==> 跳过镜像 tar（-SkipImage）"
  exit 0
}

Write-Host "==> 准备 CA 证书"
$ca = "build\ca-certificates.crt"
if (-not (Test-Path $ca)) {
  Invoke-WebRequest -Uri https://curl.se/ca/cacert.pem -OutFile $ca -UseBasicParsing
}

Write-Host "==> 构建镜像工具"
Push-Location tools\imgtar
try {
  $env:GOOS = "windows"; $env:GOARCH = "amd64"
  & $go build -o ..\..\build\imgtar.exe .
  if ($LASTEXITCODE -ne 0) { throw "imgtar 构建失败" }
} finally {
  Pop-Location
}

Write-Host "==> 打包镜像 tar"
$tar = "dist\net-bloop-$Version-linux-amd64.tar"
.\build\imgtar.exe -binary build\linux-amd64\net-bloop -ca $ca -tag "net-bloop:$Version" -out $tar -version $Version
if ($LASTEXITCODE -ne 0) { throw "镜像打包失败" }

Write-Host ""
Write-Host "完成："
Get-ChildItem build\windows-amd64, build\linux-amd64, dist -File |
  Select-Object FullName, @{n = "MB"; e = { [math]::Round($_.Length / 1MB, 2) } } |
  Format-Table -AutoSize
Write-Host "NAS 上执行：docker load -i $tar"
Write-Host "然后：docker compose up -d"
