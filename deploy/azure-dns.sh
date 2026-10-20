#!/bin/bash
# 在 Azure DNS 建 atimac13cj.com，并写上指向虚机的 A 记录。
# 需要先 az login。建好之后，还要到域名注册商把 DNS 服务器改成这里打印的四个地址。
set -euo pipefail

zone=atimac13cj.com
group="${AZURE_DNS_RG:-Routine}"
ip="${GARDEN_IP:-20.229.113.191}"

az group create --name "$group" --location westeurope --output none
az network dns zone create --resource-group "$group" --name "$zone" --output none
az network dns record-set a add-record --resource-group "$group" --zone-name "$zone" --record-set-name "@" --ipv4-address "$ip" --output none
az network dns record-set a add-record --resource-group "$group" --zone-name "$zone" --record-set-name "www" --ipv4-address "$ip" --output none

echo "把注册商的 DNS 服务器改成："
az network dns zone show --resource-group "$group" --name "$zone" --query nameServers --output tsv
