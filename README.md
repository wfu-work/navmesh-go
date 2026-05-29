# NavMesh Go

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Build](https://github.com/wfu-work/navmesh-go/actions/workflows/build-navmesh.yml/badge.svg)](https://github.com/wfu-work/navmesh-go/actions/workflows/build-navmesh.yml)

NavMesh Go 是一个面向边缘设备的轻量远程接入网关。它适用于处在 NAT、内网、4G/5G 路由器或客户现场网络后的设备，让运维人员可以通过统一公网入口访问设备本机 SSH 或 Web 服务。

项目提供反向隧道客户端、SSH 网关、HTTP 映射网关、设备注册、Token 管理、访问策略、事件和审计日志等能力。核心目标是：不直接暴露设备端口，也能稳定访问私有网络中的边缘设备。

```bash
ssh root@test01.navfirst.com
```

```text
https://test01.qx.navfirst.com -> 边缘设备 127.0.0.1:7090
```

## 功能特性

- 支持没有公网 IP 的设备通过反向隧道接入。
- 支持 Linux 客户端一键安装。
- 支持通过中心 SSH 网关访问设备本机 SSHD。
- 支持按 Host 路由的 HTTP/HTTPS 设备本地服务映射。
- 支持全局注册 Token 首次接入。
- 支持每台设备独立 Token 激活、轮换和禁用。
- 设备注册时自动创建 SSH 映射。
- 支持设备分组和动态设备类型默认值。
- 支持 SSH 和 HTTP 映射访问策略。
- 支持连接会话、事件、审计日志和保留策略清理。
- 默认使用 SQLite，适合轻量私有化部署。
- GitHub Release 自动构建服务端和客户端裸二进制。

## 工作原理

```text
用户
  |
  | SSH / HTTP
  v
+---------------------+
| NavMesh Server      |
| - 管理 API           |
| - SSH Gateway       |
| - HTTP Gateway      |
| - QUIC Tunnel       |
+----------+----------+
           |
           | 反向隧道
           v
+---------------------+       本地 TCP       +------------------+
| navmesh-client      | -------------------> | SSHD / Web 服务  |
| 边缘设备侧           |                      | 127.0.0.1:22     |
+---------------------+                      +------------------+
```

设备始终主动连接中心服务。用户侧不需要安装本地代理客户端。

## 组件

| 组件 | 说明 |
| --- | --- |
| `navmesh-server` | 管理 API、隧道服务、SSH 网关、HTTP 映射网关 |
| `navmesh-client` | 设备侧客户端，负责保持反向隧道在线 |
| SQLite | 默认本地数据库，保存设备、会话、映射、事件和审计日志 |
| Caddy | 可选 HTTPS 前置网关，用于 HTTP 映射域名接入 |

## 快速开始

### 启动服务端

```bash
git clone https://github.com/wfu-work/navmesh-go.git
cd navmesh-go
go mod tidy
make run
```

默认服务端口：

| 服务 | 默认值 |
| --- | --- |
| 管理 API | `http://127.0.0.1:3007/api` |
| QUIC 隧道 | `:3008/udp` |
| SSH 网关 | 由 `ssh_listen` 配置 |
| HTTP 映射网关 | 由 `http_listen` 配置 |

健康检查：

```bash
curl http://127.0.0.1:3007/api/health
```

### 安装客户端

Linux 设备可使用最新 Release 一键安装：

```bash
curl -fsSL https://github.com/wfu-work/navmesh-go/releases/latest/download/install-client.sh | sudo sh -s -- \
  --server navmesh.navfirst.com \
  --api https://navmesh.navfirst.com \
  --port 3008 \
  --token xxxxxx
```

默认安装路径：

```text
/opt/navmesh/navmesh-client
/opt/navmesh/navmesh-client.json
/usr/local/bin/navmesh-client
/etc/systemd/system/navmesh-client.service
```

如果设备无法稳定访问 GitHub，可以把 `install-client.sh` 和对应平台二进制，例如 `navmesh-client-linux-arm64`，同步到自己的下载域名：

```bash
curl -fsSL https://navmesh.navfirst.com/api/downloads/install-client.sh | sudo sh -s -- \
  --download-base https://navmesh.navfirst.com/api/downloads \
  --server navmesh.navfirst.com \
  --api https://navmesh.navfirst.com \
  --port 3008 \
  --token xxxxxx
```

### 手动启动客户端

```bash
./navmesh-client \
  -server navmesh.navfirst.com \
  -api https://navmesh.navfirst.com \
  -port 3008 \
  -token xxxxxx \
  -sshPort 22
```

客户端可以不传 `sncode`、`alias`、`remark` 和 `type`。首次启动时会自动生成稳定的本地 `sncode`，并把设备状态保存到程序同级目录的 `navmesh-client.json`。

## 设备激活流程

NavMesh 将“首次注册”和“激活接入”分开处理。

1. 新设备使用全局注册 Token 接入。
2. 服务端创建或更新设备记录，并自动创建 SSH 映射。
3. 仅使用全局 Token 的设备保持未激活状态。
4. 管理员为设备生成专属 Token。
5. 设备使用专属 Token 重新接入后变为激活在线状态。

如果设备本地状态文件丢失，客户端可以再次使用全局 Token 注册。服务端会重置该设备 Token，并把设备重新置为未激活状态。

## 客户端在线升级

NavMesh 支持由管理端上传 `navmesh-client` 二进制，并通过设备心跳下发升级任务。

基本流程：

1. 管理端上传客户端二进制，例如 `navmesh-client-linux-arm64`。
2. 服务端保存文件、计算 SHA256，并生成可下载地址。
3. 管理端为指定设备创建升级任务。
4. 设备心跳返回升级命令。
5. 客户端下载二进制、校验 SHA256、备份旧文件并替换当前程序。
6. 客户端上报升级结果，并通过 systemd 重启 `navmesh-client` 服务。

如果设备无法访问 GitHub，可以把安装脚本的 `--download-base` 指向 NavMesh 后端下载接口：

```bash
curl -fsSL https://navmesh.navfirst.com/api/downloads/install-client.sh | sudo sh -s -- \
  --download-base https://navmesh.navfirst.com/api/downloads \
  --server navmesh.navfirst.com \
  --api https://navmesh.navfirst.com \
  --port 3008 \
  --token xxxxxx
```

后台上传的客户端二进制默认保存在 `local.oss-path/navmesh-client` 下。运行期配置 `client_download_base` 可指定公开下载域名，例如：

```text
https://navmesh.navfirst.com/api/downloads
```

上传接口示例：

```bash
curl -F file=@navmesh-client-linux-arm64 \
  -F version=v0.0.2 \
  -F os=linux \
  -F arch=arm64 \
  https://navmesh.navfirst.com/api/client-releases/upload
```

## 构建

构建服务端：

```bash
make build
```

构建客户端：

```bash
make client
```

构建全部：

```bash
make all
```

运行测试：

```bash
go test ./...
```

## GitHub Release

工作流 [build-navmesh.yml](.github/workflows/build-navmesh.yml) 会在以下场景触发：

- 推送 `v*` tag
- 手动执行 `workflow_dispatch`

Release 产物直接上传裸二进制，不再打压缩包：

```text
navmesh-server-linux-amd64
navmesh-client-linux-amd64
navmesh-server-linux-arm64
navmesh-client-linux-arm64
navmesh-server-darwin-arm64
navmesh-client-darwin-arm64
navmesh-server-windows-amd64.exe
navmesh-client-windows-amd64.exe
install-client.sh
```

## 配置

`config.yaml` 只保存服务启动必需配置，例如 API 端口、SQLite 路径、JWT、日志和设备首次注册 Token。

```yaml
system:
  app-name: "navmesh"
  addr: 3007
  db-type: sqlite
  router-prefix: /api

jwt:
  issuer: navmesh
  signing-key: "navmesh"

sqlite:
  db-name: navmesh
  path: ./data/

navmesh:
  data-dir: ./data/navmesh
  heartbeat-timeout: 90s
  device-register-token: "xxxxxx"
```

运行期配置，例如网关监听地址、域名、保留策略和限流参数，会保存到数据库中，并可通过 API 管理。

常用运行期配置：

| Key | 默认值 | 说明 |
| --- | --- | --- |
| `tunnel_listen` | `:3008` | QUIC 隧道监听地址 |
| `ssh_enabled` | `true` | 是否启用 SSH 网关 |
| `ssh_listen` | `:3010` | SSH 网关监听地址 |
| `http_mapping_enabled` | `true` | 是否启用 HTTP 映射网关 |
| `http_listen` | `:3009` | HTTP 映射网关监听地址 |
| `device_register_token` | `xxxxxx` | 设备首次注册 Token |
| `device_heartbeat_timeout` | `90s` | 设备离线判定超时时间 |
| `client_upgrade_enabled` | `true` | 是否允许心跳下发客户端升级任务 |
| `client_download_base` | 空 | 客户端二进制公开下载地址前缀 |

生产环境请务必修改默认 JWT 密钥和设备注册 Token。

## API 概览

默认 API 前缀：`/api`

| 接口 | 说明 |
| --- | --- |
| `POST /api/device/register` | 设备首次注册 |
| `POST /api/device/heartbeat` | 设备心跳 |
| `GET /api/devices/list` | 设备列表 |
| `POST /api/devices/:guid/tokens` | 创建设备 Token |
| `GET /api/device-groups/list` | 设备分组/类型列表 |
| `GET /api/ssh-entrypoints/list` | SSH 入口地址列表 |
| `GET /api/ssh-aliases/list` | SSH 别名列表 |
| `GET /api/port-mappings/list` | HTTP 映射列表 |
| `GET /api/custom-domains/list` | 自定义域名列表 |
| `GET /api/tunnel/connections` | 在线隧道连接 |
| `GET /api/tunnel-sessions/list` | SSH/HTTP 会话列表 |
| `GET /api/events/list` | 事件列表 |
| `GET /api/audit-logs/list` | 审计日志 |
| `GET /api/settings/list` | 运行期配置 |
| `GET /api/client-releases/list` | 客户端二进制列表 |
| `POST /api/client-releases/upload` | 上传客户端二进制 |
| `GET /api/downloads/:fileName` | 下载客户端二进制 |
| `GET /api/devices/:guid/upgrades` | 设备升级任务列表 |
| `POST /api/devices/:guid/upgrades` | 创建设备升级任务 |
| `POST /api/device/upgrade/report` | 客户端上报升级结果 |

管理端认证由 `nav-common-go-lib` 提供。

## SSH 路由说明

SSH 协议没有类似 HTTP `Host` 或 HTTPS SNI 的目标域名信息。如果多个设备域名都解析到同一个 `IP:22`，网关无法知道用户输入的是哪个域名。

要支持这种简洁命令：

```bash
ssh root@test01.navfirst.com
```

每个设备别名需要解析到可区分的 SSH 入口地址，推荐使用独立 IPv6：

```text
test01.navfirst.com AAAA 2400:xxxx::1001
test02.navfirst.com AAAA 2400:xxxx::1002
```

网关会根据 TCP 连接的本地目标 IP 路由到对应设备。

HTTP 映射没有这个限制，因为 HTTP 网关可以通过 `Host` 路由。

## HTTP 映射

HTTP 映射把公网域名路由到设备本机 TCP 服务：

```text
test01.qx.navfirst.com -> test01 -> 127.0.0.1:7090
```

生产环境建议使用 Caddy 或其他 HTTPS 前置网关负责 TLS，再反向代理到 NavMesh HTTP 映射网关。

部署示例：

- [deploy/Caddyfile.example](deploy/Caddyfile.example)
- [deploy/dns-records.example.txt](deploy/dns-records.example.txt)
- [deploy/navmesh.service.example](deploy/navmesh.service.example)
- [deploy/navmesh-client.service.example](deploy/navmesh-client.service.example)

## 目录结构

```text
.
├── apis/                 # HTTP API 处理器
├── cmd/navmesh-client/   # 设备侧客户端
├── deploy/               # 部署示例和安装脚本
├── domains/              # 数据模型
├── httpgateway/          # HTTP 映射网关
├── inits/                # 应用初始化
├── routers/              # API 路由
├── services/             # 业务服务
├── sshgateway/           # SSH TCP 网关
├── tunnel/               # QUIC 隧道协议
├── utils/                # 通用工具
├── webs/                 # 静态 Web 资源
├── config.yaml
├── makefile
└── README.md
```

## 安全建议

- 生产环境请修改 `navmesh.device-register-token`。
- 生产环境请修改 JWT 签名密钥。
- 不要在缺少认证和网络限制的情况下暴露管理 API。
- SSH 认证由目标设备 SSHD 透明处理。
- NavMesh 不保存目标设备 SSH 密码。
- 对外暴露 HTTP 映射时请使用 HTTPS。

## 贡献

欢迎提交 Issue 和 Pull Request。

较大改动建议先提交 Issue 讨论设计和兼容性影响。

推荐本地检查：

```bash
go test ./...
make all
```

## 许可证

NavMesh Go 基于 [MIT License](LICENSE) 开源。
