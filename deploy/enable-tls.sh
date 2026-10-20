#!/bin/bash
# 域名已经指向虚机之后，再向 Let's Encrypt 申请证书。
set -euo pipefail

host="${DEPLOY_HOST:-walden@20.229.113.191}"
ip="$(dig +short A atimac13cj.com @8.8.8.8 | head -n 1)"
if [[ "$ip" != "20.229.113.191" ]]; then
  echo "atimac13cj.com 现在解析到 ${ip:-空}，还不是这台虚机" >&2
  exit 1
fi

ssh -o BatchMode=yes "$host" "kubectl annotate ingress garden traefik.ingress.kubernetes.io/router.tls.certresolver=letsencrypt --overwrite"
echo "已向 Let's Encrypt 申请证书。一两分钟后用 https://atimac13cj.com 打开。"
