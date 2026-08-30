# 安全与信任

[English version](security.md)

Tailpath 依赖 Tailnet 提供网络可达性和连接身份。所有 `/api/v1/*` 请求，包括
topology、history、SSE 和 report ingest，都必须通过 Tailscale WhoIs。服务端不开放
宽松 CORS。Reporter 是部署者控制的可信组件。

tailscaled 模式默认只绑定本机 Tailscale IP。绑定 wildcard、LAN 或其他非 Tailscale
地址必须显式使用 `--unsafe-allow-non-tailnet-listen`；该 flag 只改变 listener，永远
不会关闭 API WhoIs。

Compose collector 仅为适配不同宿主机的 LocalAPI socket 组 ID 而以 root
运行；socket 只读挂载，且不应为该服务增加可写宿主机挂载。Server 以非特权
用户运行。

默认服务使用专用 Tailnet identity。与其他服务共用 tailscaled identity 是显式
降级模式，因为 peer 累计 counter 无法区分 Tailpath 和业务流量。

Tailpath 不记录 auth key 或完整 report body。公网 endpoint、节点标识和流量历史
只存储在本地 SQLite，不发送产品遥测。

Container 部署应通过 `TS_AUTHKEY=file:/run/secrets/...` 传递 reusable auth key，
enrollment 后清零 secret。Environment 和 Compose model 中只保存文件路径，不保存
credential 值。

Runtime 不能主动探测、抓包、修改策略或 Tailscale preferences。

Devices API enrichment 是可选只读能力，使用独立 OAuth client，scope 固定为
`devices:core:read`。Tailpath 不接受 API key，也不请求 device posture、ACL、user
或任何 write scope。Client secret 只从文件读取，不得进入 environment、SQLite、
runtime checkpoint、日志、API error 或 dogfood evidence。只保留规范化的设备 identity
和展示字段；raw API response、用户数据、route、posture 和管理字段立即丢弃。远端错误
只向 UI 暴露 unauthorized、forbidden、rate-limited、unavailable、timeout 或
invalid-response 等固定分类，不转发 response body 或 credential 细节。
