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

当前拓扑由内存提供读取。每个 accepted report、traffic bucket 和逻辑路径变更都在
同一个 SQLite transaction 中提交。Typed candidate state 首次立即 checkpoint，之后
最多每秒一次；checkpoint 同时记录其包含的最后 report rowid。只有 transaction 成功
后，ingest 才转移 candidate ownership 并通知 SSE。存储失败不会推进内存中的
sequence 或 inventory。每个 client 的 invalidation 会合并到 250ms window；browser
把 refresh burst 合并为一个 in-flight request 和最多一个 follow-up。

路径变更按逻辑路径身份比较。Observer-local Direct endpoint 只属于 provenance
属性；相反两侧 observer 报告同一条 Direct 连接的不同端点时不会产生新 transition。
DERP region 或 Peer Relay node 的变化仍然是逻辑路径变更。只要新鲜 edge
provenance 仍引用一个已知 Peer Relay，该 relay node 就会保留在可见拓扑中。

重启从最新 checkpoint 恢复 reporter sequence、observer 自己持有的 inventory
generation 和 membership、reporter 到 observer 的 ownership、identity alias、节点、
最新 observation 和 edge lifecycle，再只重放 rowid 更大的 report 并写入新
checkpoint。新 reporter 进程只能通过完整 hello 接管 observer；旧 session 的普通消息
不能重新取得 ownership。每分钟的 maintenance 只删除已被 committed checkpoint 覆盖
的 raw report。SQLite 还保存十秒 traffic bucket，以及带 supporting provenance 的
聚合路径变更。
