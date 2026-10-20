# 小径分岔的花园

The Garden of Forking Paths。

一座个人花园：游戏、书籍、电影、音乐各是一条小径，画廊和筛选的用法接近 Notion 数据库。技术栈是 Vue、Go 和 PostgreSQL。项目代号 `forking-paths`。

默认打开「全部」里的本年抵达：所有小径里状态为已完成、面世日期不为空、抵达日期在今年的记录。选中「游戏」时，用词会回到通关、游玩平台、累计时长。书籍、电影和音乐各有自己的载体、经由和状态。

封面是程序生成的几何图形。编辑时可以贴一张自己的图片地址。

如果库里还留着旧的游戏表，启动时会把那些记录迁进游戏小径，并补上书籍、电影和音乐的示例。空库则四条小径一起写入示例。

## 启动

需要 Docker、Go 和 Node.js。数据库映射在本机 `5433` 端口，库名仍是 `gamelog`，这样已有的数据可以原地迁移。

```bash
docker compose up -d
cd backend && go run ./cmd/server
cd frontend && npm install && npm run dev
```

打开 http://localhost:5173 。后端接口在 http://localhost:8080 。

## 发布

本机编译前端和 Linux 程序，都通过之后才传到 Azure 虚机。虚机上单独起一个 Postgres 17，库名 `garden`，不碰虚机里原有的库，也不上传本机的记录和封面。启动时只建表，并写上四条小径的名称。

```bash
make publish
```

默认主机是 `walden@20.229.113.191`，可以用 `DEPLOY_HOST` 换。程序在虚机的 8091 端口，由原来的 Traefik 在 443 端口转给 `atimac13cj.com`。

证书用 Let's Encrypt，免费，到期前由 Traefik 自动续。域名现在的 DNS 在阿里云（万网）。要改到 Azure DNS：先 `az login`，再执行 `./deploy/azure-dns.sh`，然后在阿里云域名控制台把 DNS 服务器换成脚本打印的四个地址。解析生效后执行 `./deploy/enable-tls.sh`，之后用 https://atimac13cj.com 访问。

数据在 Docker 卷 `gamelog-data` 里。某条小径已经有记录时，不会再往里面灌示例。
