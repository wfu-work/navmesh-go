# NavMesh Go

NavMesh Go 是一个面向边缘设备的远程接入网关后端，目标是用类似 frp/ngrok 的反向隧道技术，让处在内网、NAT、4G/5G 网络或客户现场网络后的设备，可以被运维人员通过统一入口直接访问。

项目后端采用 Go 开发，并以 `github.com/wfu-work/nav-common-go-lib` 作为基础框架依赖。数据持久化优先使用 SQLite，降低单机部署、私有化部署和边缘侧部署成本。工程结构和初始化方式可参考 `/Users/wfu/Documents/works/xiaoxi/code/aegis/aegis-go`。

## 项目定位

NavMesh Go 不是服务器探针项目，也不是以指标采集为核心的监控系统。它的核心目标是“远程接入”：

- 边缘设备主动连接中心网关，解决设备没有公网 IP、NAT 后无法直连的问题。
- 用户侧不需要安装本地代理客户端，不需要先启动本地端口转发。
- 用户通过 SSH 直接连接统一入口，再由中心网关路由到目标设备。
- 设备不再通过难记的 `IP:端口` 区分，而是通过设备别名区分。
- 支持自定义端口映射，例如把边缘设备本机 `7090` 映射为 `test01.qx.navfirst.com`。
- 服务端提供设备注册、别名管理、隧道管理、权限控制、连接审计和会话状态。

目标体验：

```bash
ssh root@test01.navfirst.com
```

其中 `test01` 是设备别名，`root` 是目标设备上的 SSH 用户名。

HTTP 服务映射目标体验：

```text
边缘设备本机: http://127.0.0.1:7090
外部访问地址: https://test01.qx.navfirst.com
```

## 关键技术判断

### 是否采用类似 frp 的技术

是，整体思路更接近 frp/ngrok/Cloudflare Tunnel，而不是传统 VPN：

- 边缘设备侧主动向中心服务建立长连接。
- 中心服务持有设备在线连接和路由表。
- 用户访问中心服务的固定入口。
- 中心服务把用户连接桥接到对应设备的本地 SSH 服务，例如设备本机 `127.0.0.1:22`。
- 设备侧不需要暴露公网端口，也不需要用户知道设备真实 IP。

### “用户侧不安装代理”的含义

这里需要区分两端：

- 用户电脑侧：不需要安装本地代理、不需要本地 frpc、不需要本地端口转发，直接使用系统自带 `ssh`。
- 设备侧：可以安装独立 `navmesh-client`，也可以把连接能力内置进已有设备程序或网关固件中。边缘客户端不使用 YAML 配置文件，采用命令行参数启动，便于嵌入安装脚本、systemd 和设备出厂参数。

也就是说，不能完全没有设备侧组件。frp 类方案能避免“用户侧装代理”和“设备开公网端口”，但需要“设备侧主动连接中心”。

### `ssh root@test01.navfirst.com` 的限制

如果 `test01.navfirst.com`、`test02.navfirst.com` 都通过通配 DNS 解析到同一个中心网关 IP，并且都使用 SSH 默认 22 端口，那么中心网关在收到 TCP/SSH 连接时通常无法知道用户输入的是哪个域名。原因是 SSH 协议不像 HTTPS 那样有 SNI，连接里不会自动携带目标 Host。

因此，第一版不采用 `ssh root@test01@ssh.navfirst.com` 这种用户名编码方式，也不做 SSH Config 模拟。`ssh root@test01.navfirst.com` 要成立，必须让不同设备别名域名解析到不同的网关入口 IP，Go Gateway 再根据 TCP 连接的本地目标 IP 路由到对应设备。优先使用 IPv6 地址池实现：

| 方案 | 用户命令 | 是否需要用户侧代理 | 说明 |
| --- | --- | --- | --- |
| 方案 A：每设备独立 IPv6 或入口 IP | `ssh root@test01.navfirst.com` | 不需要 | DNS 把不同设备别名解析到不同入口 IP，网关按目标 IP 路由 |
| 方案 D：不同设备不同端口 | `ssh -p 22001 root@ssh.navfirst.com` | 不需要 | 技术简单，但不符合“不用 IP 加端口区分”的目标 |

第一版采用方案 A：

```text
test01.navfirst.com AAAA 2400:xxxx::1001
test02.navfirst.com AAAA 2400:xxxx::1002
```

路由规则：

```text
ssh root@test01.navfirst.com
        |
        v
DNS -> 2400:xxxx::1001
        |
        v
Go SSH Gateway 根据本地目标 IP 2400:xxxx::1001 找到设备 test01
```

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 语言 | Go |
| 基础框架 | `github.com/wfu-work/nav-common-go-lib` |
| 配置 | YAML，默认入口 `config.yaml` |
| 数据库 | SQLite |
| 管理 API | RESTful API，默认前缀 `/api` |
| 用户接入 | SSH Gateway，默认监听 `:22` 或 `:2222` |
| HTTP 映射 | HTTP/HTTPS Gateway，按 Host 路由并支持自定义域名绑定 |
| 设备隧道 | 设备主动连接中心，初期可用 WebSocket/TCP over TLS |
| 认证 | 管理端 JWT + 设备 Token + SSH 公钥/密码透传策略 |
| 日志 | zap，沿用 `nav-common-go-lib` 日志配置 |
| 本地存储 | `./data` 目录，包含 SQLite、会话记录、审计日志和缓存 |
| 构建 | Makefile |

## 系统架构

```text
用户电脑
  ssh root@test01.navfirst.com
          |
          v
+----------------------+
| NavMesh SSH Gateway  |
| - SSH 握手            |
| - 按目标 IP 路由设备   |
| - 权限校验             |
| - 会话审计             |
| - 连接桥接             |
+----------+-----------+
           |
           | 复用设备主动建立的反向隧道
           v
+----------------------+        本地 TCP        +------------------+
| Edge Tunnel Client   |  --------------------> | 设备本机 SSHD     |
| - 主动连中心          |                         | 127.0.0.1:22     |
| - 保持长连接          |                         +------------------+
| - 打开本地目标端口     |
+----------------------+
```

服务端同时包含四类入口：

- 管理 API：设备、别名、Token、权限、审计管理。
- 设备隧道入口：设备侧组件主动连接并保持在线。
- SSH 网关入口：用户直接使用标准 SSH 客户端连接。
- HTTP 网关入口：按域名 Host 将外部 HTTP/HTTPS 请求路由到设备本地端口。

## 核心流程

### 1. 设备注册

设备侧组件首次启动后：

- 使用命令行参数中的 `-server`、`-port`、`-token` 连接中心服务。
- 上报设备信息，例如 `-sncode`、`-deviceId`、`-type`、本机 IP 和客户端版本。
- 绑定或申请设备别名，默认使用 `-sncode`，例如 `test01`。
- 填写设备备注，例如 `-remark 深圳工厂1号测试网关`。
- 服务端保存设备 ID、别名、Token 状态和最近在线时间。

### 2. 设备保持反向隧道

设备侧组件长期维持到中心服务的加密长连接：

- 连接中心 `tunnel.navfirst.com`。
- 周期性发送心跳。
- 等待中心服务下发“打开本地连接”的请求。
- 当用户访问设备时，连接本机 `127.0.0.1:22` 并把流量桥接到中心网关。

### 3. 用户 SSH 接入

第一版推荐命令：

```bash
ssh root@test01.navfirst.com
```

中心 SSH 网关处理逻辑：

```text
1. 接收 SSH 连接
2. 读取 TCP 连接的本地目标 IP
3. 按目标 IP 查询绑定的设备别名 test01
4. 得到目标设备 SSH 用户 root
5. 查询设备是否在线
6. 校验当前用户是否有权访问 test01
7. 通过设备反向隧道创建一条到 127.0.0.1:22 的流
8. 在用户 SSH 连接和设备 SSHD 之间转发字节流
9. 记录会话开始、结束、来源 IP、目标设备、目标用户和结果
```

### 4. 自定义端口映射

设备可以把本地服务映射到外部域名。例如设备 `test01` 上有一个 Web 服务监听 `127.0.0.1:7090`：

```text
device alias: test01
local target: 127.0.0.1:7090
public host:  test01.qx.navfirst.com
```

外部用户访问：

```text
https://test01.qx.navfirst.com
```

中心 HTTP 网关处理逻辑：

```text
1. 接收 HTTP/HTTPS 请求
2. 读取 Host: test01.qx.navfirst.com
3. 解析设备别名 test01 和映射域 qx.navfirst.com
4. 查询 test01 是否在线
5. 查询 test01.qx.navfirst.com 绑定的本地目标端口 127.0.0.1:7090
6. 通过设备反向隧道创建一条到 127.0.0.1:7090 的流
7. 在外部 HTTP 请求和设备本地服务之间转发数据
8. 记录访问日志、流量、耗时和结果
```

端口映射第一版优先支持 HTTP/HTTPS 服务。纯 TCP 服务后续可以通过独立入口域名、SNI、端口或 SSH 子系统扩展。

### 5. 别名路由

设备别名必须满足：

- 全局唯一，默认取设备 `sncode`，便于直接用 `test01` 这类短别名路由。
- 可读、可记，例如 `test01`、`factory-gw-01`、`sz-plc-03`。
- 可绑定 DNS 记录或 SSH 登录名解析规则。
- 支持中文备注字段，例如 `深圳工厂 1 号测试网关`，备注不参与路由，只用于管理界面识别。
- 修改别名时保留审计日志。

## 核心模块规划

### 1. 服务端启动

- 读取 `config.yaml`。
- 初始化日志、SQLite、管理 API、SSH 网关、设备隧道入口。
- 自动创建必要数据目录，例如 `./data`、`./data/navmesh/audit`。
- 提供健康检查接口。

### 2. 设备管理

- 设备注册：生成或绑定设备 ID。
- 设备序列号：使用 `sncode` 作为设备接入主标识，默认也作为设备别名。
- 设备 ID：支持可选 `deviceId`，用于兼容已有业务系统设备编号。
- 设备类型：支持 `ssh`、`radar`、`radar-one`、`rain`、`hipnames`、`dic`、`ppp`、`sag`、`data` 等类型。
- 设备别名：创建、修改、全局唯一性校验、禁用。
- 设备备注：支持中文备注信息，便于管理人员识别设备用途和位置。
- 设备认证：每台设备使用独立 Token 接入中心服务。
- Token 管理：生成、启用、禁用、轮换设备 Token。
- 在线状态：维护连接状态、最后心跳、来源 IP、客户端版本。
- 设备分组：支持按客户、站点、区域、业务、标签筛选。

设备类型默认端口和域名参考现有 `navpn-client/vpn-client.go`：

| 设备类型 | 默认 Web 端口 | 默认映射域名 | 说明 |
| --- | --- | --- | --- |
| `ssh` | 无 | 无 | 只启用 SSH 反向接入 |
| `radar` | `8888` | `vpn-ipc.navfirst.com` | 雷达类设备 |
| `radar-one` | `8887` | `vpn-one.navfirst.com` | 单点雷达类设备 |
| `rain` | `8889` | `vpn-qx.navfirst.com` | 气象/雨量类设备 |
| `hipnames` | `8886` | `vpn-hipnames.navfirst.com` | Hipnames 设备 |
| `dic` | `8885` | `vpn-dic.navfirst.com` | DIC 设备 |
| `ppp` | `8884` | `vpn-ppp.navfirst.com` | PPP 设备 |
| `sag` | `8883` | `vpn-sag.navfirst.com` | SAG 设备 |
| `data` | `3002` | `vpn-data.navfirst.com` | 数据服务类设备 |

新系统也支持统一映射域，例如 `*.qx.navfirst.com`。如果传入 `-webDomain qx.navfirst.com`，则外部域名可生成为 `<sncode>.qx.navfirst.com`。

### 3. 隧道管理

- 设备主动连接中心服务。
- 服务端维护设备连接池。
- 支持一条设备连接上复用多条用户访问流。
- 支持打开本地 TCP 目标，例如 `127.0.0.1:22`。
- 支持打开自定义本地端口，例如 `127.0.0.1:7090`。
- SSH 目标固定为设备本机 SSHD，默认 `127.0.0.1:22`。
- 支持连接超时、断线重连、心跳保活。
- 支持限流、最大并发会话数和空闲超时。

### 4. SSH 网关

- 监听公网 SSH 入口，例如多个设备入口 IPv6 的 `:22`。
- 支持标准 OpenSSH 客户端。
- 第一版不从 SSH 用户名中解析设备别名，而是按 TCP 连接的本地目标 IP 路由设备。
- 支持设备别名域名解析到独立 IPv6 或独立入口 IP，例如 `test01.navfirst.com -> 2400:xxxx::1001`。
- 将用户 SSH 流量桥接到设备本机 SSHD，目标固定为本机 SSH 服务。
- 支持 SSH 公钥认证透传模式。
- 支持服务端统一鉴权模式，后续再扩展。

认证模式建议分阶段：

| 模式 | 说明 | 第一版建议 |
| --- | --- | --- |
| 透传认证 | 用户最终仍由目标设备 SSHD 认证，中心只负责路由和审计 | 优先实现 |
| 网关统一认证 | 中心先校验用户身份，再代理到设备 | 后续实现 |
| 临时凭证 | 中心生成短期访问凭证 | 后续实现 |

### 5. HTTP/HTTPS 映射网关

- 监听公网 HTTP/HTTPS 入口，支持通配域和用户自定义域名。
- 按请求 Host 解析设备别名和映射规则。
- 支持将 `test01.qx.navfirst.com` 转发到设备本机 `127.0.0.1:7090`。
- 支持一台设备配置多个服务映射，例如管理后台、摄像头页面、本地 API。
- 支持绑定用户自定义域名，例如 `my-device.example.com`。
- 支持启用、禁用、修改映射。
- 支持访问日志和失败原因记录。

映射规则示例：

| 外部域名 | 设备别名 | 本地目标 | 说明 |
| --- | --- | --- | --- |
| `test01.qx.navfirst.com` | `test01` | `127.0.0.1:7090` | 设备 Web 管理页面 |
| `test01-api.qx.navfirst.com` | `test01` | `127.0.0.1:8080` | 设备本地 API |
| `my-device.example.com` | `test01` | `127.0.0.1:7090` | 用户自定义域名 |

第一版建议外部域名支持系统自动生成，也支持绑定用户自定义域名。

```text
<设备别名>.qx.navfirst.com
```

也可以绑定：

```text
my-device.example.com
```

第一版只做 Host 到设备本地端口的一对一映射，不做路径级转发、同域多映射和多证书绑定。

### 6. 权限策略

第一版只支持单管理员，不做多用户、多角色和复杂 RBAC。权限策略保持简单：

- 管理员可以管理所有设备和映射。
- 哪个设备别名允许被 SSH 访问。
- 哪些 HTTP 映射允许外部访问。
- 是否允许 root 登录由目标设备 SSHD 决定。
- 支持设备禁用、Token 禁用、别名禁用。
- 支持映射禁用。
- 支持按设备组批量授权。

后续可扩展：

- 限制目标 SSH 用户。
- 限制来源 IP。
- 限制访问时间窗口。
- 会话审批。
- 命令审计或会话录像。

### 7. 连接审计

需要记录：

- 用户来源 IP。
- SSH 网关用户名原文，例如 `root@test01`。
- 目标设备别名。
- 目标设备 ID。
- 目标本地地址，例如 `127.0.0.1:22`。
- 连接开始时间、结束时间、持续时间。
- 成功、失败、断开原因。
- 字节流量统计。
- HTTP 映射访问 Host、路径、状态码、耗时和上游失败原因。

第一版只做连接级审计，不做 SSH 命令级审计。

## 数据模型草案

初期建议表：

| 表 | 用途 |
| --- | --- |
| `devices` | 设备基础信息、`sncode`、`device_id`、设备类型、全局唯一别名、中文备注、状态、版本、来源 IP |
| `device_tokens` | 每设备独立 Token、状态、过期时间、轮换记录 |
| `device_connections` | 设备到中心的在线连接状态 |
| `device_heartbeats` | 设备心跳历史 |
| `tunnel_sessions` | 用户访问设备的隧道会话记录 |
| `ssh_aliases` | SSH 别名、入口 IP 和设备绑定关系 |
| `ssh_entrypoints` | 可分配的 SSH 入口 IPv4/IPv6 地址池 |
| `port_mappings` | 外部域名或用户自定义域名到设备本地服务端口的映射 |
| `http_access_logs` | HTTP/HTTPS 映射访问日志 |
| `access_policies` | 设备、设备组和映射访问策略 |
| `device_groups` | 设备分组 |
| `events` | 设备离线、连接失败、认证失败等事件 |
| `audit_logs` | 管理操作和配置变更审计 |
| `users` | 单管理员账号 |
| `settings` | 系统配置项 |

SQLite 初期直接承担关系数据和短周期状态数据。心跳、会话和审计表需要按时间、设备 ID、别名建索引，并通过保留策略定期清理。

## API 规划

默认接口前缀建议来自 `config.yaml`：

```yaml
system:
  router-prefix: /api
```

建议 API 分组：

| 路由 | 说明 |
| --- | --- |
| `/api/auth/*` | 登录、刷新 Token、退出 |
| `/api/devices/*` | 设备列表、详情、状态、删除 |
| `/api/device-tokens/*` | 设备 Token 创建、禁用、轮换 |
| `/api/device/register` | 设备注册或绑定 |
| `/api/device/heartbeat` | 设备心跳上报 |
| `/api/tunnel/connect` | 设备侧隧道接入入口 |
| `/api/ssh-aliases/*` | SSH 别名创建、修改、绑定、禁用 |
| `/api/port-mappings/*` | 自定义端口映射创建、修改、绑定、禁用 |
| `/api/access-policies/*` | 访问策略管理 |
| `/api/tunnel-sessions/*` | 会话查询、断开、统计 |
| `/api/http-access-logs/*` | HTTP 映射访问日志查询 |
| `/api/events/*` | 事件列表、确认、关闭 |
| `/api/audit-logs/*` | 审计日志查询 |
| `/api/settings/*` | 系统配置 |

SSH 网关不是普通 HTTP API，建议独立监听：

```text
0.0.0.0:22    # 生产环境
0.0.0.0:2222  # 本地开发或无 root 权限部署
```

HTTP/HTTPS 映射网关建议独立监听：

```text
0.0.0.0:80
0.0.0.0:443
```

## 配置规划

`config.yaml` 只保存服务启动必需的本地配置，例如 HTTP 管理 API 端口、数据库路径、JWT 基础密钥和数据目录。域名、网关监听地址、隧道端口、默认映射域、证书策略等运行期配置应通过管理页面写入 `settings` 表，不在配置文件中写死。

建议第一版 `config.yaml`：

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
  session-idle-timeout: 30m
  audit-retention-days: 90
  device-register-token: "navfirst@2020"
  default-target-host: "127.0.0.1"
  default-target-port: 22
```

管理页面可配置项写入 `settings` 表，初期建议包括：

| 配置项 | 示例值 | 说明 |
| --- | --- | --- |
| `public_domain` | `navfirst.com` | 系统主域名 |
| `ssh_gateway_domain` | `ssh.navfirst.com` | SSH 网关域名 |
| `http_mapping_domain` | `qx.navfirst.com` | 系统默认 HTTP 映射域 |
| `ssh_listen` | `:22` | SSH Gateway 生产监听地址 |
| `http_listen` | `:8080` | HTTP Gateway 监听地址，Caddy 反代到这里 |
| `https_listen` | `:8443` | 可选 HTTPS Gateway 监听地址，使用 Caddy 时可关闭 |
| `tunnel_listen` | `:3008` | QUIC 隧道监听地址 |
| `allow_custom_domain` | `true` | 是否允许用户绑定自定义域名 |
| `default_ssh_port` | `22` | 默认设备本机 SSH 端口 |
| `session_idle_timeout` | `30m` | 会话空闲超时 |

这些配置需要支持页面修改、保存、审计和必要的服务重载。监听端口类配置修改后，第一版可以要求重启服务生效；域名、默认映射域、自定义域名开关等配置应尽量运行期生效。

后续补充：

- 管理员初始账号配置。
- 设备注册是否允许自助申请。
- 设备注册 Token 支持通过 `navmesh.device-register-token` 配置；未配置 YAML 时使用管理页保存的 `device_register_token`，再回落到 `navfirst@2020`。
- Token 默认有效期。
- SSH 网关 Host Key 路径。
- 通配 DNS 和自定义域名绑定配置提示。
- HTTP/HTTPS 证书配置。
- 会话并发限制。
- 审计日志保留天数。

## DNS 与连接方式规划

### 第一版推荐

DNS：

```text
test01.navfirst.com AAAA <分配给 test01 的网关 IPv6>
test02.navfirst.com AAAA <分配给 test02 的网关 IPv6>
*.qx.navfirst.com A <中心网关公网 IP>
```

用户连接：

```bash
ssh root@test01.navfirst.com
```

优点：

- 不需要用户侧安装代理。
- 不需要每台设备独立端口。
- 用户命令符合 `ssh root@test01.navfirst.com` 的目标体验。
- 中心网关能按 TCP 连接本地目标 IP 可靠路由到设备。

缺点：

- 需要公网 IPv6 地址池，或者为每台设备分配独立公网入口 IP。
- 如果所有设备域名都解析到同一个 IP:22，SSH 网关无法区分设备。

HTTP 映射访问：

```text
https://test01.qx.navfirst.com
```

其中：

```text
test01            设备全局唯一别名
qx.navfirst.com   默认 HTTP 映射域
```

中心网关通过 HTTP Host 头可以可靠识别子域名或自定义域名，因此 HTTP/HTTPS 场景支持系统生成域名和用户自定义域名绑定。这个能力依赖 HTTPS 的 SNI 和 HTTP 的 Host Header。

### SSH 入口 IP 要求

第一版要支持 `ssh root@test01.navfirst.com`，必须为设备别名分配可区分的 SSH 入口地址：

```text
test01.navfirst.com -> 2400:xxxx::1001
test02.navfirst.com -> 2400:xxxx::1002
```

Go Gateway 监听这些入口地址的 22 端口，并通过连接的本地目标 IP 找到设备。IPv6 地址池是最推荐的方式；如果部署环境没有 IPv6 地址池，则需要为设备分配独立 IPv4，或重新评估 SSH 接入格式。

## 目录规划

建议服务端目录结构：

```text
.
├── apis/
├── domains/
├── inits/
├── routers/
├── services/
├── tunnel/
├── sshgateway/
├── httpgateway/
├── utils/
├── data/
├── config.yaml
├── go.mod
├── makefile
└── README.md
```

后续如果在同仓库提供服务端和设备侧连接组件，可以增加：

```text
cmd/
├── server/
└── client/
```

其中：

- `cmd/server`：中心服务，包含管理 API、隧道入口、SSH 网关。
- `cmd/client`：设备侧连接组件，负责主动连接中心并桥接本机 SSHD。

## 开发流程

### 1. 初始化依赖

```bash
go mod init navmesh-go
go get github.com/wfu-work/nav-common-go-lib
go mod tidy
```

### 2. 本地运行

```bash
make run
```

服务默认监听：

```text
http://127.0.0.1:3007
```

本地 SSH 网关建议监听：

```text
127.0.0.1:2222
```

本地测试命令：

```bash
ssh -p 2222 -l root@test01 127.0.0.1
```

### 3. 构建

```bash
make build
```

## 设备侧部署思路

边缘设备需要运行一个轻量连接组件。它不是监控探针，职责只围绕远程接入：

- 主动连接中心隧道入口。
- 维持心跳和在线状态。
- 接收中心发来的新连接请求。
- 连接本机 SSHD，例如 `127.0.0.1:22`。
- 连接本机自定义服务，例如 `127.0.0.1:7090`。
- 在中心网关和设备本地服务之间转发 TCP 流。

推荐 Linux 安装目录：

```text
/usr/local/bin/navmesh-client              # 设备侧连接组件
/var/lib/navmesh-client/                   # 本地状态、设备 ID
/var/log/navmesh-client/                   # 日志
/etc/systemd/system/navmesh-client.service # systemd 服务文件
```

边缘客户端不使用 YAML 配置文件，采用带参数启动。参数设计参考：

```bash
./navmesh-client \
  -server navpn.navfirst.com \
  -port 8880 \
  -token navfirst@2020 \
  -sncode test01 \
  -deviceId 1050001 \
  -type rain \
  -sshPort 22 \
  -webPort 7090 \
  -webDomain qx.navfirst.com \
  -remark 深圳工厂1号测试网关
```

参数说明：

| 参数 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-server` | 否 | `navpn.navfirst.com` | 中心隧道服务地址 |
| `-port` | 否 | `8880` | 中心隧道服务端口 |
| `-token` | 否 | `navfirst@2020` | 设备接入 Token |
| `-sncode` | 是 | 无 | 设备序列号，默认作为全局唯一设备别名 |
| `-deviceId` | 否 | 空 | 业务系统设备 ID |
| `-type` | 是 | `radar` | 设备类型，例如 `ssh`、`radar`、`rain`、`data` |
| `-remark` | 否 | 空 | 中文备注，例如设备位置、用途 |
| `-sshPort` | 否 | `22` | 设备本机 SSH 端口 |
| `-webPort` | 否 | 按设备类型推导 | 设备本机 Web 服务端口 |
| `-webDomain` | 按场景 | 按设备类型推导 | 外部 Web 映射域名，例如 `qx.navfirst.com` |

示例含义：

```text
sncode:    test01
type:      rain
remark:   深圳工厂1号测试网关
ssh:       ssh root@test01.navfirst.com
web:       https://test01.qx.navfirst.com -> 127.0.0.1:7090
```

systemd 服务示例：

```ini
[Unit]
Description=NavMesh Edge Client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/navmesh-client -server navpn.navfirst.com -port 8880 -token navfirst@2020 -sncode test01 -deviceId 1050001 -type rain -sshPort 22 -webPort 7090 -webDomain qx.navfirst.com -remark 深圳工厂1号测试网关
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

安装脚本后续只需要写入带参数的 `ExecStart`，不生成 `/etc/navmesh-client/config.yaml`。

## 实现顺序建议

第一阶段：服务端基础

- 初始化 `nav-common-go-lib`。
- 读取 `config.yaml`。
- 初始化 SQLite。
- 提供健康检查接口。
- 建立数据库迁移或自动建表机制。

第二阶段：设备接入与别名

- 实现设备注册和每设备独立 Token 校验。
- 实现 `sncode`、`deviceId`、`type`、`remark` 参数入库。
- 实现设备别名创建、绑定、全局唯一性校验。
- 实现设备中文备注字段。
- 实现按设备类型推导默认 Web 端口和默认映射域名。
- 实现设备心跳和在线状态。
- 实现设备列表和详情 API。

第三阶段：反向隧道

- 实现设备侧长连接入口。
- 实现服务端设备连接池。
- 实现流复用协议或先用“一连接一会话”的简单模型。
- 实现中心到设备本机 `127.0.0.1:22` 的 TCP 桥接。

第四阶段：SSH 网关

- 实现 Go SSH Server。
- 监听 `:2222` 本地开发端口。
- 实现 SSH 入口 IP 地址池和设备别名绑定。
- 按 TCP 连接本地目标 IP 查询设备别名。
- 查询设备在线状态。
- 将 SSH 连接桥接到设备反向隧道。
- 记录会话审计。

第五阶段：HTTP/HTTPS 端口映射

- 实现端口映射数据模型。
- 实现 `*.qx.navfirst.com` 和用户自定义域名 Host 解析。
- 实现 `test01.qx.navfirst.com` 到设备本机 `127.0.0.1:7090` 的映射。
- 实现用户自定义域名绑定、启用、禁用和冲突校验。
- 实现映射启用、禁用和访问日志。
- 增加 HTTP 网关本地开发端口。

第六阶段：权限与审计

- 实现设备访问策略。
- 实现会话列表、会话断开、连接失败原因。
- 实现 HTTP 访问日志查询。
- 实现审计日志。
- 实现设备禁用、Token 禁用、别名禁用。
- 实现端口映射禁用。

第七阶段：生产化

- 监听生产 `:22`。
- 配置 `test01.navfirst.com` 等设备别名 DNS 到独立入口 IPv6。
- 配置 `*.qx.navfirst.com` DNS。
- 支持用户自定义域名 CNAME/A 记录接入说明。
- 增加 SSH Host Key 管理。
- 增加 HTTPS 证书管理。
- 增加客户端安装脚本。
- 增加 systemd 启动参数模板，不生成客户端 YAML 配置文件。
- 增加连接限流、超时和重连策略。
- 增加多设备并发会话压测。

## 初期边界

初期不做以下内容：

- 不做服务器指标采集和安全巡检。
- 不做传统 VPN 地址池和虚拟网卡编排。
- 不要求用户电脑安装本地代理。
- 不使用 `IP:端口` 作为主要设备区分方式。
- 不接受 `ssh root@test01@ssh.navfirst.com` 作为第一版连接格式。
- 第一版 SSH 目标格式为 `ssh root@test01.navfirst.com`，技术前提是设备别名域名具备独立入口 IPv6 或独立入口 IP。
- 不做 SSH 命令级审计和会话录像，第一版只做连接级审计。
- 不保存目标设备 SSH 密码。
- HTTP/HTTPS 映射第一版按域名 Host 路由，不先做泛 TCP 子域名路由。
- HTTP/HTTPS 映射第一版不做路径级转发、同域多映射和多证书绑定。
- SSH 目标设备必须使用本机 SSHD。
- 边缘客户端不读取 YAML 配置文件，第一版只支持命令行参数和 systemd 参数启动。
- 不做多用户权限和复杂 RBAC，第一版只支持单管理员。

## 待确认问题

当前第一版核心约束已确认，暂无待确认问题。

## 当前实现状态

当前已完成第一阶段、第二阶段、第三阶段、第四阶段、第五阶段、第六阶段和第七阶段服务端代码：

- 服务启动：`main.go` 调用 `inits.Init()`，通过 `nav-common-go-lib/inits.SysInit` 读取 `config.yaml`、初始化日志、SQLite、系统表和 Gin HTTP 服务。
- 基础配置：`config.yaml` 只保留服务启动必需配置，域名、监听地址、默认映射域等运行期配置写入 `navmesh_settings`。
- 自动建表：已创建设备、设备 Token、连接、心跳、SSH 入口、端口映射、自定义域名、会话、HTTP 访问日志、事件、审计、settings 等业务表；用户、角色、登录等系统表由 `nav-common-go-lib` 初始化。
- 默认 settings：初始化 `public_domain`、`ssh_gateway_domain`、`http_mapping_domain`、`ssh_listen`、`tunnel_listen`、`device_register_token` 等配置项。
- 设备注册：`POST /api/device/register`，支持 `sncode`、`deviceId`、`type`、`remark`、`sshPort`、`webPort`、`webDomain` 入库。
- 设备类型：内置 `ssh`、`radar`、`radar-one`、`rain`、`hipnames`、`dic`、`ppp`、`sag`、`data` 的默认 Web 端口和默认域名。
- Token 校验：注册阶段使用 `device_register_token`，注册成功后写入设备 Token；心跳接口校验设备 Token。
- 设备心跳：`POST /api/device/heartbeat`，更新在线状态、来源 IP、本机 IP、主机名、客户端版本和最后在线时间。
- 管理接口：`GET /api/devices/list`、`GET /api/devices/:guid`、`DELETE /api/devices/:guid`、`GET /api/devices/types/defaults`。
- Settings 接口：`GET /api/settings/list`、`PUT /api/settings/:key`。
- QUIC 隧道入口：服务启动后根据 `navmesh_settings.tunnel_listen` 启动 QUIC Server，默认监听 `:3008/udp`。
- 隧道认证：设备连接后通过首个 QUIC stream 发送 `hello` 帧，服务端校验设备 Token，并复用设备心跳逻辑更新在线状态。
- 连接池：已实现 `tunnel.DefaultManager`，按 `deviceGuid` 管理在线 QUIC 连接，记录连接时间、远端地址和最后活动时间。
- 连接入库：设备 QUIC 连接建立后写入 `navmesh_device_connections`，断开后标记为禁用状态。
- 基础协议帧：已定义 `hello`、`hello_ack`、`heartbeat`、`ping`、`pong`、`open_tcp`、`open_tcp_ack`、`error`。
- 基础流转发能力：服务端已提供 `OpenTCPStream(ctx, deviceGuid, targetHost, targetPort)`，用于后续 SSH/HTTP 网关向设备侧打开本地端口流。
- 隧道调试接口：`GET /api/tunnel/connections` 查询当前在线隧道连接。
- SSH Gateway：已实现透明 TCP SSH Proxy，服务端不终止 SSH 协议、不保存 SSH 密码，用户最终仍由设备本机 SSHD 完成认证。
- SSH 路由：根据 TCP 连接的本地目标 IP 查询 `navmesh_ssh_aliases` / `navmesh_ssh_entrypoints`，找到对应设备后通过 QUIC 隧道打开设备本机 `127.0.0.1:<sshPort>`。
- SSH 会话记录：建立 SSH 转发时写入 `navmesh_tunnel_sessions`，结束时记录字节数、结束时间和断开原因。
- SSH 管理接口：`GET /api/ssh-entrypoints/list`、`POST /api/ssh-entrypoints`、`GET /api/ssh-aliases/list`、`POST /api/ssh-aliases`、`DELETE /api/ssh-aliases/:id`。
- SSH 启动配置：根据 `navmesh_settings.ssh_enabled` 和 `navmesh_settings.ssh_listen` 启动 SSH Gateway，生产默认 `:22`。
- HTTP 映射网关：已实现独立 HTTP Gateway，根据 `navmesh_settings.http_mapping_enabled` 和 `navmesh_settings.http_listen` 启动，默认监听 `:8080`，用于接收 Caddy 反向代理后的请求。
- HTTP 路由方式：按请求 Host 精确查询 `navmesh_port_mappings.public_host`，支持系统生成域名和用户自定义域名；第一版只做 Host 到设备本地端口的一对一映射，不做路径级转发、同域多映射和多证书绑定。
- HTTP 转发链路：映射命中后通过 `tunnel.DefaultManager.OpenTCPStream(ctx, deviceGuid, targetHost, targetPort)` 打开设备侧本地端口，例如 `127.0.0.1:7090`，并把外部 HTTP 请求和设备本地服务响应桥接起来。
- HTTP 映射管理接口：`GET /api/port-mappings/list`、`POST /api/port-mappings`、`DELETE /api/port-mappings/:guid`。
- HTTP 访问日志接口：`GET /api/http-access-logs/list`，记录 Host、路径、来源 IP、状态码、耗时、入站/出站字节数和上游失败原因。
- 域名冲突校验：创建或更新端口映射时校验 `publicHost` 全局唯一，避免两个映射绑定同一个外部域名。
- 管理端认证：复用 `nav-common-go-lib` 的系统用户、登录、登出和当前用户接口，例如 `POST /api/login/in`、`POST /api/login/out`、`GET /api/user/token`。
- 管理端 JWT：由 `nav-common-go-lib` 登录流程签发，并通过基础框架中间件注入当前用户信息。
- 访问策略 ACL：已实现 `navmesh_access_policies` 管理接口，支持 `global`、`device`、`mapping` 三种 scope；没有匹配策略时默认放行，有匹配策略时按 `allowSsh` / `allowHttp` 控制。
- ACL 接入点：SSH Gateway 打开设备 SSHD 隧道前校验 `allowSsh`；HTTP Mapping Gateway 在 Host 命中后、打开设备本地端口前校验 `allowHttp`。
- 审计日志：已实现 `GET /api/audit-logs/list`，登录、修改密码、settings 保存、设备禁用、设备 Token 禁用、SSH 入口/别名保存、SSH 别名禁用、端口映射保存/禁用、访问策略保存/禁用都会写入 `navmesh_audit_logs`。
- 会话查询：已实现 `GET /api/tunnel-sessions/list`，支持按设备、会话类型、访问域名和状态查询 SSH 隧道会话。
- Token 禁用：已实现 `DELETE /api/devices/:guid/tokens/:tokenGuid`，用于禁用指定设备 Token。
- HTTP 访问日志查询增强：`GET /api/http-access-logs/list` 支持按 Host、设备、方法、路径和状态码过滤。
- 生产保留策略：已实现后台 retention cleaner，按 settings 清理审计日志、HTTP 访问日志、隧道会话、设备心跳和设备连接历史。
- 手动维护接口：已实现 `POST /api/maintenance/retention-cleanup`，管理员可手动触发保留策略清理，并写入审计日志。
- 生产部署样例：已补充 `deploy/Caddyfile.example`、`deploy/navmesh.service.example`、`deploy/navmesh-client.service.example`、`deploy/dns-records.example.txt`。
- Caddy 接入边界：Caddy 负责 HTTPS、泛域名、自定义域名和 TLS；Go Gateway 的 HTTP Mapping Gateway 默认只监听 `:8080` 接收 Caddy 反代请求。
- SSH 生产入口说明：SSH Gateway 可监听公网 `:22`，但它是透明 TCP Proxy，不终止 SSH 协议，因此不生成 SSH Host Key，Host Key 仍来自目标设备本机 SSHD。

编译验证：

```bash
go test ./...
make build
```

运行验证：

```bash
GET /api/health
POST /api/device/register
POST /api/device/heartbeat
POST /api/login/in
GET /api/user/token
GET /api/audit-logs/list
POST /api/maintenance/retention-cleanup
QUIC tunnel server started on :3008
SSH gateway route registered
HTTP mapping gateway started on :8080
```

设备侧客户端已补充第一版代码：

- 客户端入口：`cmd/navmesh-client/main.go`。
- 构建命令：`make client`，输出 `./navmesh-client`。
- 启动方式：只使用命令行参数，不读取 YAML 配置文件。
- 注册流程：启动后默认调用 `POST /api/device/register`，上报 `sncode`、`deviceId`、`type`、`remark`、`sshPort`、`webPort`、`webDomain`、主机名、本机 IP 和客户端版本。
- 隧道连接：注册后连接中心 QUIC 隧道入口，发送 `hello` 帧完成设备 Token 校验。
- 心跳保活：周期性发送 QUIC heartbeat，并调用 `POST /api/device/heartbeat` 更新设备在线状态。
- 断线重连：注册失败、QUIC 连接失败、隧道断开后自动重连；重连等待使用指数退避，默认从 `5s` 增长到最大 `60s`。
- 失败探测：连续心跳失败达到阈值后主动关闭当前 QUIC 连接并进入重连流程，避免设备侧长期停留在假在线状态。
- 优雅退出：收到 `SIGTERM` 或 `Ctrl-C` 时停止心跳并关闭 QUIC 连接。
- 本地端口桥接：收到服务端 `open_tcp` 后连接设备本机目标端口，例如 `127.0.0.1:22` 或 `127.0.0.1:7090`，并在 QUIC stream 和本地 TCP 之间双向转发。

客户端示例：

```bash
./navmesh-client \
  -server tunnel.navfirst.com \
  -port 3008 \
  -api https://navmesh-admin.navfirst.com \
  -token navfirst@2020 \
  -sncode test01 \
  -deviceId 1050001 \
  -type rain \
  -sshPort 22 \
  -webPort 7090 \
  -webDomain qx.navfirst.com \
  -heartbeat 30s \
  -heartbeatFail 3 \
  -reconnectWait 5s \
  -reconnectMax 60s \
  -requestTimeout 10s \
  -remark 深圳工厂1号测试网关
```

客户端重连相关参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-reconnectWait` | `5s` | 首次重连等待时间 |
| `-reconnectMax` | `60s` | 指数退避最大等待时间 |
| `-heartbeat` | `30s` | QUIC 和 HTTP 心跳间隔 |
| `-heartbeatFail` | `3` | 连续心跳失败多少次后主动断开并重连 |
| `-requestTimeout` | `10s` | HTTP 注册/心跳和本地端口连接超时 |

本地联调已验证：

```bash
./navmesh-client -server 127.0.0.1 -port 3008 -api http://127.0.0.1:3007 -token navfirst@2020 -sncode test-client-01 -type ssh -remark 本地联调设备
GET /api/tunnel/connections
```

下一步建议进入联调完善阶段：补充真实 SSHD 和本地 Web 服务端到端测试脚本、客户端安装脚本、客户端日志轮转、断网重连压测和 `open_tcp` 失败原因上报。
