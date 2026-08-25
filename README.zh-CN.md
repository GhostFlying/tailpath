# Tailpath

Tailpath 是面向 Tailscale 网络的被动式实时拓扑与流量路径查看器。它展示哪些节点
正在通信、流量大小，以及当前路径是 Direct、DERP、Peer Relay 还是 Unknown。

> Tailpath 正在开发中，首个可用纵向版本的进度见[路线图](docs/roadmap.md)。

[English README](README.md)

## 原则

- 读取 Tailscale 已经维护的运行时状态。
- 不通过持续主动探测发现路径。
- 展示真实流量关系，而不是 ACL 可达关系。
- 保留每条 observation 的来源，缺失信息明确显示为 unknown。
- 拓扑和流量数据只保存在自托管实例中。

## 计划部署方式

中央服务默认使用专用 tsnet 节点。Collector 每两秒读取一次本机 tailscaled 状态，
只有非控制节点的流量计数发生变化时才提交 traffic sample。服务端统一下发默认一分钟
的空闲心跳，并用它关联控制 freshness window。

发布产物是一个 `tailpath` 多子命令二进制和一个 OCI image。v0.2 以 Linux
server 和 collector 为支持目标；macOS 和 Windows collector 均提供可构建、
可安装的 preview，但不声明真实节点支持。Server 仅支持 Linux Compose 部署。

原生 collector 安装脚本和各平台后台服务方式见[部署说明](docs/deployment.zh-CN.md)。

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
