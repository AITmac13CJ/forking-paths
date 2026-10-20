#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
  umask 077
  printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 24)" > .env
fi
if ! grep -q '^GARDEN_UID=' .env; then
  printf 'GARDEN_UID=%s\nGARDEN_GID=%s\n' "$(id -u)" "$(id -g)" >> .env
fi
chmod 600 .env
mkdir -p ../covers

sudo docker compose up -d --build
ready=0
for _ in $(seq 1 40); do
  if sudo docker compose exec -T postgres pg_isready -U garden -d garden >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "$ready" != 1 ]]; then
  echo "数据库没有起来" >&2
  exit 1
fi
