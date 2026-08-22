# 安全与信任

[English version](security.md)

Tailpath 依赖 Tailnet 提供网络可达性和连接身份。服务端通过 WhoIs 记录 reporter
provenance，不开放宽松 CORS。Reporter 是部署者控制的可信组件。

默认服务使用专用 Tailnet identity。与其他服务共用 tailscaled identity 是显式
降级模式，因为 peer 累计 counter 无法区分 Tailpath 和业务流量。

Tailpath 不记录 auth key 或完整 report body。公网 endpoint、节点标识和流量历史
只存储在本地 SQLite，不发送产品遥测。

Runtime 不能主动探测、抓包、修改策略或 Tailscale preferences。Devices API
enrichment 是可选只读能力，使用独立最小权限 credential。
