#!/bin/bash
# 本机编译通过之后，把程序发布到 Azure 虚机。
# 不上传本机数据库和封面。线上是空的 garden 库，启动时只建表。
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
host="${DEPLOY_HOST:-walden@20.229.113.191}"
remote="forking-paths"
stage="$(mktemp -d)"
empty_public="$(mktemp -d)"
trap 'rm -rf "$stage" "$empty_public"' EXIT

echo "编译前端"
cp "$root/frontend/public/platinum.png" "$root/frontend/public/steam.png" "$root/frontend/public/xbox.png" "$empty_public/"
(cd "$root/frontend" && VITE_PUBLIC_DIR="$empty_public" npm run build)

echo "编译后端"
(cd "$root/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$stage/garden" ./cmd/server)
mkdir -p "$stage/frontend/dist"
cp -a "$root/frontend/dist/." "$stage/frontend/dist/"
cp "$root/deploy/Dockerfile" "$root/deploy/compose.yaml" "$root/deploy/up.sh" "$stage/"
chmod +x "$stage/garden" "$stage/up.sh"

echo "上传程序"
ssh -o BatchMode=yes "$host" "mkdir -p ~/$remote/app ~/$remote/covers"
rsync -az --delete --exclude .env "$stage/" "$host:$remote/app/"

echo "启动"
ssh -o BatchMode=yes "$host" "bash ~/$remote/app/up.sh"
rsync -az "$root/deploy/k8s/garden.yaml" "$host:$remote/app/garden.yaml"
ssh -o BatchMode=yes "$host" "kubectl apply -f ~/$remote/app/garden.yaml"
echo "已发布"
