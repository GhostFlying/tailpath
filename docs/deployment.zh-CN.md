# 部署

[English version](deployment.md)

默认 Compose 服务运行 `tailpath server --network=tsnet`，将 tsnet state 和
SQLite 保存到 `/var/lib/tailpath`，首次启动使用 `TS_AUTHKEY` 加入 Tailnet，注册
完成后应移除该 key。

Linux collector 可原生运行或使用 host-network Compose profile，并只读挂载
tailscaled LocalAPI socket。macOS 和 Windows collector 使用原生二进制。

服务端 identity 必须专用于 Tailpath。共用 identity 的 tailscaled 模式必须显式
确认到该节点的所有流量都会隐藏为 system telemetry。

生产环境需要在线备份 SQLite、持久化 tsnet state，并逐版本升级。Migration 在
readiness 之前执行。
