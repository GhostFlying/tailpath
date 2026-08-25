# 部署

[English version](deployment.md)

默认 Compose 服务运行 `tailpath server --network=tsnet`，将 tsnet state 和
SQLite 保存到 `/var/lib/tailpath`，首次启动使用 `TS_AUTHKEY` 加入 Tailnet。
Production Compose model 把 `TAILPATH_AUTHKEY_FILE` 指定的宿主文件挂载为
`/run/secrets/tailscale-authkey`，container environment 和展开后的 Compose model
始终只有 `file:/run/secrets/tailscale-authkey`，不会包含 credential value。

首次 enrollment 前，在私有目录中创建 secret source。Image 以 nonroot 用户运行，
因此文件可以放在 mode-0700 的父目录中并设为 mode-0444：

```sh
install -d -m 0700 secrets
install -m 0600 /dev/null secrets/tailscale-authkey
read -r -s -p 'One-use tsnet auth key: ' tailpath_authkey
printf '\n'
printf '%s\n' "$tailpath_authkey" > secrets/tailscale-authkey
unset tailpath_authkey
chmod 0444 secrets/tailscale-authkey
docker compose up -d server
```

Server healthy 后清零 key value，但保留空的 source file 供后续 Compose recreate：

```sh
chmod u+w secrets/tailscale-authkey
: > secrets/tailscale-authkey
chmod 0444 secrets/tailscale-authkey
```

唯一的 `tailpath-data` volume 同时保存 `tailpath.db` 和 `tsnet/` identity 目录，
所以 `docker compose down` 后再 `docker compose up` 必须继续使用同一个已注册身份。
除非明确要一起删除数据库和 Tailnet identity，否则不要执行 `down -v`。

Linux collector 可原生运行或使用可选的 `collector` host-network Compose
profile，并只读挂载 tailscaled LocalAPI socket。Collector 通过 Tailnet hostname
或 Tailscale IP 访问 server，不使用 Docker service DNS；server 名称不是 `tailpath`
时需要设置 `TAILPATH_SERVER_URL`。内置 reporter 会显式绕过进程的 HTTP proxy
设置，以保留 WhoIs 所需的 Tailscale source identity。macOS 和 Windows collector
使用原生二进制。

原生 collector 支持 `TAILPATH_SERVER_URL` 和 `TAILPATH_SOCKET`，显式传入的
`--server`、`--socket` flags 优先级更高。`tailpath collector --check` 只读取一次
LocalAPI，以 JSON 输出 self identity、运行平台和 peer 数量；它不会连接 Tailpath
server，也不会主动 probe 任何 peer。

## 原生 collector archives

每个 GitHub Release 包含 amd64/arm64 的 Linux、macOS `tar.gz` 和 Windows `zip`。
每个 archive 只带当前平台的 collector binary、安装材料、README 和 license。
安装前应使用 `checksums.txt` 校验下载文件。

Linux 解压对应 archive 后执行：

```sh
sudo ./install.sh --server-url http://tailpath.example.ts.net:8080
sudo systemctl status tailpath-collector.service
```

Installer 将 binary 安装到 `/usr/local/bin`，创建 hardened systemd unit，并仅在
文件不存在时以 mode 0600 创建 `/etc/default/tailpath-collector`。非默认
LocalAPI socket 使用 `--socket PATH`。重复安装保留 operator 修改过的配置；
`sudo ./uninstall.sh` 保留配置，`sudo ./uninstall.sh --purge` 才删除。

macOS 必须使用当前登录的桌面用户安装，不能使用 `sudo`：

```sh
./install.sh --server-url http://tailpath.example.ts.net:8080
launchctl print "gui/$(id -u)/com.tailpath.collector"
```

Installer 使用 `~/Library/Application Support/Tailpath`、用户级 LaunchAgent 和
`~/Library/Logs/Tailpath`。它会被动检查 LocalAPI；Tailscale GUI safesocket
不可用时只警告，不阻止文件安装。`./uninstall.sh` 保留配置和日志，
`./uninstall.sh --purge` 才删除。macOS package 当前为 preview：CI、archive
layout 和 installer fixture 已通过，但在
[#58](https://github.com/GhostFlying/tailpath/issues/58) 的真实节点 qualification
完整通过前，不声明受支持的真实节点 contract。

Windows 解压 archive 后，在提升权限的 Windows PowerShell 5.1 中执行：

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install.ps1 -ServerUrl http://tailpath.example.ts.net:8080
Get-ScheduledTask -TaskName "Tailpath Collector"
```

Preview installer 使用 `%ProgramFiles%\Tailpath` 和以 SYSTEM 身份启动的 Scheduled
Task。Runner 最多保留五个约 5 MiB 的日志文件。重复安装保留 `collector.env`；
提升权限执行 `.\uninstall.ps1` 会保留配置和日志，`.\uninstall.ps1 -Purge` 才
删除。v0.2 不声明 Windows 真实节点支持，也不提供签名或 MSI。

tailscaled server 模式未填写 listen host 时会使用本机第一个 Tailscale IP。
Wildcard、LAN 和其他非 Tailscale 地址默认被拒绝，只有显式传入
`--unsafe-allow-non-tailnet-listen` 才能绑定；使用该 override 后 API WhoIs 仍然
是强制的。

由于不同 Linux 主机的 socket 组 ID 不一致，Compose 中仅 collector 以 root
身份打开这个只读 socket，且没有可写宿主机挂载；server 仍使用镜像的
`nonroot` 用户运行。

服务端 identity 必须专用于 Tailpath。共用 identity 的 tailscaled 模式必须显式
确认到该节点的所有流量都会隐藏为 system telemetry。

Tailpath v0.2 不提供受支持的 backup/restore contract。只复制 `tailpath.db` 并不
完整，因为 `tsnet/` 目录保存 server identity；同时也没有承诺复制 live SQLite
文件是安全的。任何临时 volume copy 都属于 unsupported 操作。升级时应逐版本
进行，migration 会在 readiness 之前执行。

SQLite 使用 WAL 和 `synchronous=NORMAL`。Process 或 operating-system crash 后
database 必须保持结构一致，但突然断电或 storage failure 可能丢失最新已提交的
observations。恢复后 collector 会重新同步当前 runtime state；中间的 history gap
不会被补写。
