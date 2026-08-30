# 项目定义

[English version](project.md)

Tailpath 回答一个运行时问题：Tailnet 中此刻什么在和什么通信、流量有多少，以及
Tailscale 实际使用了哪条路径。

## 范围内

- 从可信 observer 的 runtime peer view 中 best-effort 发现已知节点。除非可选接入
  控制面 directory source，否则永远不承诺完整 Tailnet inventory；directory 节点也
  不代表存在流量。即使启用 directory，Tailpath 也只承诺展示该 credential 当前可见
  的设备，不承诺完整 Tailnet inventory。
- 从 tailscaled 和 embedded tsnet 节点采集运行时状态。
- 将有方向的 observation 合并为逻辑流量边。
- 区分 Direct、DERP、Peer Relay 和 Unknown。
- 根据累计计数器 delta 计算速率。
- 保存路径事件和 observation provenance。
- 渲染稳定的实时图与 edge 详情。
- 通过统一协议扩展 Peer Relay、tsnet 和 tsbridge observer。
- 可选使用只读控制面设备目录补充展示元数据和搜索；该目录始终与 runtime
  observation 分层。

## 非目标

Tailpath 不是 ACL 可视化器、Admin Console、抓包工具、网络配置管理器、主动连通性
测试工具或完整 metrics 平台。移动客户端不需要安装 Tailpath agent。

## 产品边界

首个稳定版本面向单 Tailnet、单服务实例、250 个已知节点和 1000 条可见
active/recent edge。无法观察的信息保持 Unknown，不推断未观察到的数据路径。

Directory device 不代表设备在线、可观察或正在通信。Live 始终是 runtime data-plane
view；Devices workspace 是可选控制面目录，connected-to-control 与 Tailpath runtime
evidence 必须作为两个独立维度展示。
