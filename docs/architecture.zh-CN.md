# 架构

[English version](architecture.md)

```text
tailscaled collector --\
tsnet exporter --------+--> Tailnet HTTP ingest --> aggregator --> SQLite
Peer Relay exporter ---/                              |           |
                                                      +--> SSE --> Web graph
Tailscale Devices API ----> directory synchronizer --/            |
                                                                    +--> Devices
```

Devices API 路径是可选只读能力，不是 observer，也不使用 observer protocol。一次成功
的完整快照只代表该 OAuth credential 当前可见的目录。它不能创建 traffic edge、不能
把节点标记为 observable/online，也不能改变 runtime freshness。因此纯目录设备的出现
或消失不会改变 Live graph 和 Live node count。

Collector 每两秒在本地采样，但只有非控制 peer 的 counter 变化时才发送 traffic
消息。Inventory 变化和稀疏空闲心跳使用独立消息。

服务端通过 Tailscale WhoIs 认证 reporter 连接。可信 reporter 可以描述另一个
observer，因此一个 tsbridge reporter 能为多个独立 tsnet 节点上报。

Tailscale 实现类型只存在于 `internal/tailscaleadapter`、
`internal/tailscalestatus` 和下面明确说明的 public adapter 边界。协议、聚合、存储和
UI 都使用 Tailpath 自己的 domain 类型，避免 LocalAPI 版本变化污染 wire 和数据库。

`exporter/tsnet` 是有意保留的 public adapter 边界：一个已配置的
`tsnet.Server` 或现有 `local.Client` 对应一个独立 Source。它只读取 LocalAPI
status，并与 native collector 共用相同的 identity/path 规范化逻辑。按 Tailscale
上游语义，从尚未启动的 server 获取 LocalClient 可能启动该 server；登录、ready、
重启和关闭仍由应用负责。adapter 不调用 `Up`、不探测 peer，也不修改 preferences。

可运行的 `examples/tsnet-multi` 使用三个持久化 runtime identity 和第四个专用
reporter identity 验证该模型；只有前三者注册为 Source。显式启用 lifecycle demo
后，程序会 withdraw 并从同一 state dir 重建其中一个 runtime，而进程级 reporter
session 保持在线。

默认服务端使用专用 tsnet identity。Reporter 到该节点的流量归类为 system
telemetry，不做 counter 扣减，也不进入 Live graph 和 edge activity 计数。该分类仍可通过
topology API 获取，并保留在运行时状态、SQLite traffic/history 和 provenance 中用于
诊断。普通 History 查询会排除这些边及其专属节点；只有显式传入诊断参数
`includeSystemTelemetry=true` 才会返回。未解析的 relay client 不会被猜测为该 identity。
共享 tailscaled identity 属于 degraded opt-in，因为它无法将控制流量与其他应用分开。

当前拓扑由内存提供读取。每个 accepted report、traffic bucket 和逻辑路径变更都在
同一个 SQLite transaction 中提交。Typed candidate state 首次立即 checkpoint，之后
最多每秒一次；checkpoint 同时记录其包含的最后 report rowid。只有 transaction 成功
后，ingest 才转移 candidate ownership 并通知 SSE。存储失败不会推进内存中的
sequence 或 inventory。每个 client 的 invalidation 会合并到 250ms window；browser
把 refresh burst 合并为一个 in-flight request 和最多一个 follow-up。

Directory refresh 使用相同的 candidate-state 规则。Application clone aggregator，
应用一个已验证的完整快照，并原子保存 runtime checkpoint 与 directory History
metadata；只有全部成功后才替换内存并发布合并后的 SSE invalidation。请求或存储失败
会保留 last-good directory layer 并标记 stale。未配置目录启动时清除当前 directory
layer，但保留 canonical redirect 和既有 History。

路径状态由一个粘性主路径和确定性排序的新鲜冲突证据集合组成。只要主路径仍有
新鲜证据支持，跨 observer 的 report 到达顺序就不会改变它；失去支持后，endpoint
证据优先于 relay 侧证据，并以 canonical node ID 确定性打破平局。只有规范化后的
主路径或冲突集合变化时才写 transition。Direct endpoint、relay VNI、session ID 和
采样时间只属于 provenance，DERP region 和 Peer Relay StableNodeID 才区分路径。
只要新鲜 edge provenance 仍引用一个已知 Peer Relay，该 relay node 就会保留在
可见拓扑中。

重启从最新 checkpoint 恢复 reporter sequence、observer 自己持有的 inventory
generation 和 membership、reporter 到 observer 的 ownership、identity alias、节点、
最新 observation 和 edge lifecycle，再只重放 rowid 更大的 report 并写入新
checkpoint。新 reporter 进程只能通过完整 hello 接管 observer；旧 session 的普通消息
不能重新取得 ownership。每分钟的 maintenance 只删除已被 committed checkpoint 覆盖
的 raw report。SQLite 还保存十秒 traffic bucket，以及带 supporting provenance 的
聚合路径变更。
