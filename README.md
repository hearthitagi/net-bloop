# net-bloop（布噜）

只做三件事的网络面板：**动态域名（DDNS）**、**Web 反向代理**、**STUN 内网穿透**。
单二进制、无前端构建、TOML 配置，自带中文管理界面。

- 后端：Go 1.22+，静态链接
- 前端：原生 HTML/CSS/JS，`go:embed` 打进二进制，无 node_modules
- 镜像：scratch 基础，`docker save` 导出的 tar 约 9 MB
- 管理端口 `16601`；反代单监听 `33320`

原版 Lucky 的 WOL、计划任务、WebDAV、Docker 管理、Web 终端、WAF、OAuth、CGI 都没有移植，属于有意精简。

---

## 一、快速开始

### 方式 A：导入现成镜像（推荐）

`dist/` 下已有 `net-bloop-0.2.6-linux-amd64.tar`：

```bash
docker load -i net-bloop-0.2.6-linux-amd64.tar
mkdir -p /share/Container/net-bloop/data
cp net-bloop.toml /share/Container/net-bloop/data/
cd /share/Container/net-bloop      # 放好 docker-compose.yml
docker compose up -d
```

打开 `http://<NAS-IP>:16601`，**首次进来是创建管理员**（没有默认密码）。

### 方式 B：在 NAS 上构建

```bash
docker build -t net-bloop:0.2.6 .
docker compose up -d
```

需要 NAS 能访问 Docker Hub（拉 `golang:1.22-alpine`）。

### 方式 C：Windows 试跑

```powershell
.\build.ps1 -SkipImage
.\build\windows-amd64\net-bloop.exe -config net-bloop.toml
```

Windows 上 TCP 同端口打洞可能报 `Only one usage of each socket address`，
那是系统限制，Linux 内核没有这个问题；验证功能时优先用 UDP 规则。

---

## 二、QNAP TS-264C（Container Station）部署要点

TS-264C 是 x86_64，直接用 `linux/amd64` 镜像。

1. **导入镜像**：Container Station → 映像 → 导入 → 选 `.tar`。
2. **创建容器**：
   - 网络：**必须 host**。bridge 下 STUN 打洞和反代端口都不成立。
   - 存储：`/share/Container/net-bloop/data` → 容器 `/data`。
   - 环境变量：`TZ=Asia/Shanghai`（`NETBLOOP_CONFIG` 已写死在镜像里）。
3. **防火墙**：QuFirewall 放行内网到 `16601`；反代端口 `33320` 按需放行。
4. **路由器**：
   - 打洞模式：**不需要**任何端口映射，但要求运营商不是对称型 NAT；
   - 如果哪天拿到公网 IP：把公网端口（如 443）映射到 NAS 的 `33320`，
     同时把配置里的 `[[stun]]` 整段删掉，DDNS 会自己走 HTTP 查询拿 IP。

> Container Station 图形界面找不到 host 网络时，SSH 执行 `docker compose up -d`，
> `docker-compose.yml` 已固定 `network_mode: host`。

> **导入提示“文件格式无效”**：用 `dist/` 里的 tar。它是经典 docker save V1 格式
> （`manifest.json` + `repositories` + `<id>/layer.tar`），Container Station 能识别；
> OCI 风格的 tar 只有 `docker load` 认。

### 端口一览

| 端口 | 用途 |
| --- | --- |
| 16601/tcp | 管理面板（建议只在内网/VPN 访问） |
| 11111/tcp | 反代唯一监听：HTTPS + 明文 HTTP 同端口，按 SNI/Host 分发 |
| 22222/tcp | STUN 打洞监听（转发到 11111），端口在配置里可改 |

host 网络下不用做 `-p` 映射。

---

## 三、功能配置

### 1. 动态域名（DDNS）

服务商：**Cloudflare、阿里云、DNSPod/腾讯云、GoDaddy、Porkbun、自定义回调**。
记录类型 `A` / `AAAA` / `TXT`。

IP 来源：

| 来源 | 用途 |
| --- | --- |
| 自动 | 先取已打通的 STUN 公网地址，否则调用公共 IP 接口 |
| STUN 规则 | 固定用某条穿透规则得到的地址 |
| 本机网卡 | `interface:eth0`，IPv6 或内网场景 |
| 自定义地址 | `url:https://api.ipify.org`，返回纯文本 IP |
| 固定 IP | 手填 |

凭据只存 `/data/net-bloop.toml`，权限 `0600`。

**Cloudflare** 需要 API Token（`Zone.DNS:Edit`）。Zone ID 留空会自动查询；
根域名留空时按“最后两段”推导，`example.co.uk` 这类请手填。
`proxied` 留空表示不改动现有橙云/灰云设置，写 `true`/`false` 才会覆盖。

### 2. Web 反向代理

- **只有一个监听端口**（默认 33320），所有规则靠 SNI / Host 分流，支持 `*.example.com`；
- 端口、证书、是否 HTTPS、是否跳转都是**全局**的，规则里只有域名和上游；
- 上游可填多个，按轮询负载均衡；支持 WebSocket；
- 可选 BasicAuth、访问日志；
- TLS 与明文 HTTP 在同一端口用首字节分流，不用再开 80。

### 3. 证书（DNS-01）

- 内置 ACME 客户端，**只用 DNS-01**，这是签发通配符证书的唯一方式；
- 当前实现支持 Cloudflare（`cert.auth.token` 需要 `Zone.DNS:Edit`）；
- 一张证书可以覆盖多个名字，例如 `kissshot.site`、`*.kissshot.site`、`*.stun.kissshot.site`；
- 每 6 小时检查一次，剩余不足 `renew_before`（默认 30 天）就续期，失败保留旧证书；
- 证书写到 `/data/cert/fullchain.pem` 和 `privkey.pem`，反代按 mtime **热加载**，续期不用重启；
- 也可以 `web.cert_file` / `web.key_file` 指定手动证书，此时 ACME 证书不生效；
- 首次启动时证书还没签发好也没关系：端口先用明文服务，文件一落地，下一次握手自动升级成 HTTPS。

### 4. STUN 内网穿透

两种模式，别选错：

| 模式 | 行为 | 什么时候用 |
| --- | --- | --- |
| `forward` 固定转发 | 只探测公网 IP（可选填公网端口），**不开监听** | 路由器已经把端口固定映射到 NAS |
| `punch` 打洞转发 | 本地监听 `local_port`，持有 NAT 分配的公网端口，把连接转发到 `target` | 没有固定映射，靠打洞拿临时端口 |

**硬性前提**

1. 打洞要求运营商 NAT 是 Full Cone（NAT1），对称型 NAT 打不了；
2. 容器必须 host 网络；
3. TCP 打洞依赖 `SO_REUSEPORT` 做同端口保活，Linux 正常，Windows 可能失败；
4. 打洞的公网端口由 NAT 分配，**会变**，URL 里的端口跟着变。

#### Webhook：地址变化时回调

勾选后地址变化才触发，可用变量：

| 变量 | 含义 |
| --- | --- |
| `{{.IP}}` `{{.Port}}` `{{.Addr}}` | 新公网地址 |
| `{{.OldIP}}` `{{.OldPort}}` `{{.OldAddr}}` | 上一次的公网地址 |
| `{{.Name}}` `{{.Proto}}` | 规则名、协议 |
| `{{.LocalPort}}` `{{.Target}}` | 本地端口、转发目标 |
| `{{.Time}}` | 触发时间（RFC3339） |

只在“端口也会变”的场景才需要；域名跟 IP 走的话用 DDNS 即可。

### 5. 出站代理

`[net] proxy`（面板「设置 → 出站代理」）作用于所有主动出站请求：
STUN 列表拉取、ACME、DDNS 服务商 API、Webhook。填 `http://127.0.0.1:7890` 这类地址。

### 6. STUN 服务器列表

默认每天从公开列表更新一次，多源回退：自定义镜像 → `raw.githubusercontent.com` → 内置兜底。
结果缓存在 `/data/stun-servers.txt`，少于 3 条视为失败并继续用旧列表。
`[stun_list] servers` 手填时优先级最高，规则里的 `servers` 再优先于全局。

---

## 四、配置与运维

- 配置文件：容器内 `/data/net-bloop.toml`；不存在时启动会自动写一份
- 密码：Argon2id，不可逆；首次启动没有密码，靠界面创建
- 备份：设置页「导出配置 / 导入配置」（导入保留管理端口与密码）
- 热加载：界面上的增删改立即生效；`SIGHUP` 会重读配置文件
- 日志：界面「日志」页 SSE 实时查看，按模块与规则过滤
- 环境变量：`NETBLOOP_CONFIG` 可改配置路径，`-config` 参数优先

**安全建议**

- 不要把 `16601` 暴露到公网，走内网或 VPN；
- 登录失败 6 次锁定该 IP 30 秒；
- 要远程访问面板，用反代 + BasicAuth，别直接开 16601。

---

## 五、开发与构建

```bash
go build ./... && go vet ./... && go test ./...

./build.sh        # Linux/macOS
.\build.ps1       # Windows
```

产物：

```
build/windows-amd64/net-bloop.exe
build/linux-amd64/net-bloop
dist/net-bloop-0.2.6-linux-amd64.tar
```

国内网络：

```bash
export GOPROXY=https://goproxy.cn,direct
export GOTOOLCHAIN=local   # 防止 go 擅自升级 go 指令版本
```

### 目录结构

```
cmd/net-bloop/           启动、信号处理
internal/config/         配置结构、默认值、校验、热加载
internal/ddns/           调度器 + 六种服务商实现
internal/proxy/          单监听、SNI/Host 路由、TLS 分流、证书热加载
internal/cert/           ACME DNS-01 签发与续期
internal/stun/           探测、TCP/UDP 打洞转发、服务器列表、Webhook
internal/netx/           带出站代理的 HTTP 客户端、公网 IP 查询
internal/admin/          鉴权、CRUD API、SSE、内嵌 UI
tools/imgtar/            无 Docker 环境打包镜像 tar 的小工具
```

---

## 六、已知限制

- 原版 v2 的加密配置无法导入，规则要重新录；
- 证书只实现了 Cloudflare 的 DNS-01；
- 打洞能否成功取决于运营商 NAT 类型与防火墙，软件层面绕不过对称型 NAT；
- 打洞的公网端口会变，这是 NAT 决定的，不是程序缺陷；
- 未经真实外网入站长测（开发环境只能验证 STUN 映射与内网转发）。

### 首次部署自检顺序

1. 打开 `http://<NAS-IP>:16601`，创建管理员；
2. 看首页证书卡：剩余天数正常说明 DNS-01 通了（失败先查 token 权限和出站）；
3. 内网直连：`curl -k -H "Host: <某个域名>" https://<NAS-IP>:33320/`；
4. 新建一条 **UDP** 打洞规则（目标 `127.0.0.1:16601`），确认状态 active 并显示公网地址；
5. 手机切流量访问 `http://<公网IP>:<公网端口>`，能开即通；
6. 再测 TCP 规则与 12 条域名分发。

---

MIT License。功能设计参考 Lucky 项目，代码为独立重构实现。
