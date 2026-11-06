#!/usr/bin/env bash
# Build net-bloop: Linux static binary plus a docker-loadable image tarball.
# Requires only a Go toolchain; Docker is optional.
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-0.2.6}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
GO="${GO:-go}"

mkdir -p build/linux-amd64 build/windows-amd64 dist

echo "==> Go: $($GO version)"
echo "==> linux/amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o build/linux-amd64/net-bloop ./cmd/net-bloop

echo "==> windows/amd64（Windows 试跑版）"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o build/windows-amd64/net-bloop.exe ./cmd/net-bloop

CA="build/ca-certificates.crt"
if [ ! -f "$CA" ]; then
  echo "==> 下载 CA 证书"
  if command -v curl >/dev/null; then
    curl -fsSL https://curl.se/ca/cacert.pem -o "$CA"
  else
    wget -qO "$CA" https://curl.se/ca/cacert.pem
  fi
fi

echo "==> 构建镜像打包工具（本机运行，用宿主机平台）"
(cd tools/imgtar && "$GO" build -o ../../build/imgtar .)

echo "==> 生成镜像 tar"
./build/imgtar -binary build/linux-amd64/net-bloop -ca "$CA" \
  -tag "net-bloop:$VERSION" -out "dist/net-bloop-$VERSION-linux-amd64.tar" -version "$VERSION"

echo
echo "完成："
ls -lh build/linux-amd64 build/windows-amd64 dist
cat <<EOF

NAS 部署：
  docker load -i dist/net-bloop-$VERSION-linux-amd64.tar
  docker compose up -d
管理面板： http://<NAS-IP>:16601   （首次打开创建管理员）
配置文件： /data/net-bloop.toml
EOF
