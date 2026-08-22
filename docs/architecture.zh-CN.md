# 架构

[English version](architecture.md)

```text
tailscaled collector --\
tsnet exporter --------+--> Tailnet HTTP ingest --> aggregator --> SQLite
Peer Relay exporter ---/                              |           |
                                                      +--> SSE --> Web graph
```

Collector 每两秒在本地采样，但只有非控制 peer 的 counter 变化时才发送 traffic
消息。Inventory 变化和稀疏空闲心跳使用独立消息。

服务端通过 Tailscale WhoIs 认证 reporter 连接。可信 reporter 可以描述另一个
observer，因此一个 tsbridge reporter 能为多个独立 tsnet 节点上报。

Tailscale 类型只能存在于 `internal/tailscaleadapter`。协议、聚合、存储和 UI
都使用 Tailpath 自己的 domain 类型，避免 LocalAPI 版本变化污染 wire 和数据库。

默认服务端使用专用 tsnet identity。Reporter 到该节点的流量归类为 system
telemetry，不做 counter 扣减，也不进入用户 activity。

当前拓扑保存在内存并可从 SQLite 恢复。SQLite 保存最新 observation、identity
binding、路径事件、十秒 traffic bucket 和 relay session；不保存两秒原始 status
或空闲心跳历史。
