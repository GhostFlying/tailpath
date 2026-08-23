# 部署

[English version](deployment.md)

默认 Compose 服务运行 `tailpath server --network=tsnet`，将 tsnet state 和
SQLite 保存到 `/var/lib/tailpath`，首次启动使用 `TS_AUTHKEY` 加入 Tailnet。
Production Compose model 把 `TAILPATH_AUTHKEY_FILE` 指定的宿主文件挂载为
`/run/secrets/tailscale-authkey`，container environment 和展开后的 Compose model
始终只有 `file:/run/secrets/tailscale-authkey`，不会包含 credential value。

首次 enrollment 前，在私有目录中创建 secret source。Image 以 nonroot 用户运行，
因此文件可以放在 mode-0700 的父目录中并设为 mode-0444：

```sh
install -d -m 0700 secrets
install -m 0600 /dev/null secrets/tailscale-authkey
read -r -s -p 'One-use tsnet auth key: ' tailpath_authkey
printf '\n'
printf '%s\n' "$tailpath_authkey" > secrets/tailscale-authkey
unset tailpath_authkey
chmod 0444 secrets/tailscale-authkey
docker compose up -d server
```

Server healthy 后清零 key value，但保留空的 source file 供后续 Compose recreate：

```sh
chmod u+w secrets/tailscale-authkey
: > secrets/tailscale-authkey
chmod 0444 secrets/tailscale-authkey
```

唯一的 `tailpath-data` volume 同时保存 `tailpath.db` 和 `tsnet/` identity 目录，
所以 `docker compose down` 后再 `docker compose up` 必须继续使用同一个已注册身份。
除非明确要一起删除数据库和 Tailnet identity，否则不要执行 `down -v`。

Linux collector 可原生运行或使用可选的 `collector` host-network Compose
profile，并只读挂载 tailscaled LocalAPI socket。Collector 通过 Tailnet hostname
或 Tailscale IP 访问 server，不使用 Docker service DNS；server 名称不是 `tailpath`
时需要设置 `TAILPATH_SERVER_URL`。内置 reporter 会显式绕过进程的 HTTP proxy
设置，以保留 WhoIs 所需的 Tailscale source identity。macOS 和 Windows collector
使用原生二进制。

原生 collector 支持 `TAILPATH_SERVER_URL` 和 `TAILPATH_SOCKET`，显式传入的
`--server`、`--socket` flags 优先级更高。`tailpath collector --check` 只读取一次
LocalAPI，以 JSON 输出 self identity、运行平台和 peer 数量；它不会连接 Tailpath
server，也不会主动 probe 任何 peer。

tailscaled server 模式未填写 listen host 时会使用本机第一个 Tailscale IP。
Wildcard、LAN 和其他非 Tailscale 地址默认被拒绝，只有显式传入
`--unsafe-allow-non-tailnet-listen` 才能绑定；使用该 override 后 API WhoIs 仍然
是强制的。

由于不同 Linux 主机的 socket 组 ID 不一致，Compose 中仅 collector 以 root
身份打开这个只读 socket，且没有可写宿主机挂载；server 仍使用镜像的
`nonroot` 用户运行。

服务端 identity 必须专用于 Tailpath。共用 identity 的 tailscaled 模式必须显式
确认到该节点的所有流量都会隐藏为 system telemetry。

Tailpath v0.2 不提供受支持的 backup/restore contract。只复制 `tailpath.db` 并不
完整，因为 `tsnet/` 目录保存 server identity；同时也没有承诺复制 live SQLite
文件是安全的。任何临时 volume copy 都属于 unsupported 操作。升级时应逐版本
进行，migration 会在 readiness 之前执行。
