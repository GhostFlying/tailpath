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

当前拓扑由内存提供读取，但每个 accepted report 都会把完整 runtime state 持久化到
SQLite。Ingest 先 clone 并验证候选状态，再在同一事务中保存 report、runtime
state、traffic bucket 和逻辑路径变更，最后才提交内存并通知 SSE。存储失败不会推进
内存中的 sequence 或 inventory。

路径变更按逻辑路径身份比较。Observer-local Direct endpoint 只属于 provenance
属性；相反两侧 observer 报告同一条 Direct 连接的不同端点时不会产生新 transition。
DERP region 或 Peer Relay node 的变化仍然是逻辑路径变更。只要新鲜 edge
provenance 仍引用一个已知 Peer Relay，该 relay node 就会保留在可见拓扑中。

重启直接恢复 reporter sequence、observer 自己持有的 inventory generation 和
membership、reporter 到 observer 的 ownership、identity alias、节点、最新
observation 和 edge lifecycle。新 reporter 进程只能通过完整 hello 接管 observer；
旧 session 的普通消息不能重新取得 ownership。恢复不依赖有保留期限的 raw report
重放。SQLite 还保存十秒 traffic bucket，以及带 supporting provenance 的聚合路径变更。
