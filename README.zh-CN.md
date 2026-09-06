# Tailpath

[English README](README.md)

Tailpath 是面向 Tailscale 网络的被动式实时拓扑与流量路径查看器。它展示哪些节点
正在通信、流量大小，以及观测到的路径是 Direct、DERP、Peer Relay 还是 Unknown。

> **当前状态：可用 Alpha。** Tailpath 已经在真实 Tailnet 中运行，Live、History、
> collector、Peer Relay、tsnet exporter 和可选 Devices 目录等核心流程目前均可使用。
> 在 1.0 之前接口与存储行为仍可能调整；剩余的平台验证和稳定化工作见
> [路线图](docs/roadmap.md)。

## 界面示例

以下截图由固定的 mock 数据生成。设备名、地址、流量、路径和设备目录记录全部为
synthetic 数据，不包含真实 Tailnet 信息。

### 实时拓扑

![使用 synthetic Direct、DERP、Peer Relay 和 Unknown 路径的 Tailpath Live 拓扑](docs/images/readme-live.png)

### 流量与路径历史

![使用 synthetic 流量和路径证据的 Tailpath History 工作面](docs/images/readme-history.png)

### 可选设备目录

![使用 synthetic Tailscale 设备目录的 Tailpath Devices 工作面](docs/images/readme-devices.png)

## 当前可用功能

- 展示 active/recent 流量关系、双向速率，以及 Direct、DERP、Peer Relay、Unknown
  四类路径的 Live 拓扑。
- 七天流量历史、路径变化、observer provenance 和 canonical node reconciliation。
- 被动读取普通 `tailscaled` 节点的 collector，并处理重连、resync、计数器重置和
  时钟偏差。
- Peer Relay session telemetry，以及供一个应用内多个 `tsnet` runtime 使用的
  exporter API。
- 可选、只读的 Devices API 目录，用于补充控制面元数据。设备出现在目录中不会
  创建流量边，也不表示设备在线。
- 自托管 Linux Compose server、OCI image、SQLite 持久化和原生 collector 压缩包。

## Alpha 支持边界

- Server 支持以 Linux Compose 部署；Linux collector 是当前主要支持的 collector
  运行方式。
- macOS 和 Windows 提供 collector preview 压缩包，平台支持仍在验证中。
- Tailpath 是 best-effort 工具，并且刻意避免持续主动探测；它只能展示已部署
  observer 能看到的流量。
- Tailpath 不承诺完整列出整个 Tailnet。可选 Devices API 只展示只读 OAuth
  credential 当前可见的目录；没有 runtime 流量的目录设备不会出现在 Live 中。
- 当前运行目标是一个 Tailnet 使用一套自托管 Tailpath server，支持最多 250 个
  node 和 1,000 条可见 logical edge。

## 部署与 Observer

中央服务以专用 `tsnet` 节点加入 Tailnet。普通 collector 每两秒读取一次本机
`tailscaled` LocalAPI，只有非控制节点的 peer counter 变化时才上报。这使观测保持
被动，不会因主动探测而改变正在观察的路径。

请使用当前 release 产物，并按照[部署说明](docs/deployment.zh-CN.md)配置 server、
原生 collector、可选 Devices API enrichment 和升级。嵌入 Tailscale 的应用可以使用
公开的 alpha `exporter` packages；仓库提供了可运行的
[多实例 tsnet 示例](examples/tsnet-multi/README.md)。

### tsbridge 集成

我们维护的 [tsbridge fork](https://github.com/GhostFlying/tsbridge) 可以把每个嵌入的
`tsnet.Server` 作为独立 Tailpath runtime 上报。当前
[Tailpath 集成预发布版本](https://github.com/GhostFlying/tsbridge/releases/tag/v0.16.0-tailpath.1)
要求 Tailpath v0.4.0 或更新版本；Tailpath 不可用时不会影响 tsbridge 自身服务。

在 fork 中配置：

```toml
[tailpath]
server_url = "http://tailpath.example.ts.net:8080"
reporter_hostname = "tsbridge-tailpath-reporter"
reporter_tags = ["tag:tailpath-reporter"]
```

Reporter 使用一个专用、持久的 `tsnet` identity 提交 observations；每个 tsbridge
service 在图中仍然是独立的 Tailnet 节点。该集成目前由 Tailpath fork 分发，不代表
上游 tsbridge 已提供支持。

## 原则

- 读取 Tailscale 已经维护的运行时状态。
- 不通过持续主动探测发现路径。
- 展示真实流量关系，而不是 ACL 可达关系。
- 保留每条 observation 的来源，缺失信息明确显示为 unknown。
- 拓扑和流量数据只保存在自托管实例中。

## 开发

在 dev container 中打开仓库，然后运行：

```sh
make bootstrap
make check
```

修改前请阅读[开发说明](docs/development.md)、[架构](docs/architecture.zh-CN.md)和
[贡献指南](CONTRIBUTING.md)。

## 许可证

Apache-2.0，见 [LICENSE](LICENSE)。
