# 部署指南

[文档中心](README.md) · [使用指南](usage.md) · [二进制部署](binary.md) · [CAM 授权](cam.md)

[Docker](#docker-compose) · [参数](#启动参数) · [Google](#google-oauth) · [HTTPS](#反向代理与局域网) · [备份](#备份与恢复) · [升级](#升级与回退)

## Docker Compose

需要 Docker Engine 和 Docker Compose，无需下载源码。Linux / macOS 在独立目录下载配置：

```sh
mkdir -p gatesync/data
cd gatesync
sudo chown 65532:65532 data
sudo chmod 700 data
curl -fL https://raw.githubusercontent.com/Qwerto107/GateSync/main/compose.yaml -o compose.yaml
```

Windows PowerShell：

```powershell
New-Item -ItemType Directory -Path gatesync/data -Force
Set-Location gatesync
Invoke-WebRequest https://raw.githubusercontent.com/Qwerto107/GateSync/main/compose.yaml -OutFile compose.yaml
```

在该目录创建 `.env`，填写首次初始化密码：

```dotenv
GATESYNC_ACCESS_PASSWORD=replace-with-a-long-random-password
```

密码须为 12–72 字节。启动并查看日志：

```sh
docker compose up -d --pull always
docker compose logs -f
```

打开 <http://localhost:13850>。默认镜像为 `ghcr.io/qwerto107/gatesync:latest`，须在仓库完成镜像发布并将 GHCR 包设为 Public 后才能拉取。可将 Compose 中的 image 改为已发布的具体版本，标签与架构见 [发布流程](releasing.md#统一镜像)。

### 数据目录

数据直接保存在 Compose 所在目录的 `./data`，绑定挂载至容器 `/data`，不使用 Docker 命名卷。运行用户为 `65532:65532`；Linux 须保证该用户可读写专用数据目录，Windows Docker Desktop 须允许共享该目录。运行层为 scratch，不包含 Shell。

`docker compose down` 不删除主机的 `./data`。删除数据目录会丢失配置、凭证和加密主密钥；更新 Compose 时请保留该目录。

### 后台配置

首次密码登录后，在后台完成配置，无需继续添加登录环境变量：

| 页面 | 设置 |
| --- | --- |
| 系统设置 | 公开访问地址、Google OAuth、邮箱白名单、访问密码 |
| 云账号 | 云凭证、默认资源类型和地域 |
| 放行规则 | 云端资源、端口、来源 IP 和自动同步开关 |
| 计划任务 | 检查频率、Cron、任务时区和检测源 |

初始化密码仅在尚未建立系统设置时读取。成功初始化后可删除 `.env` 中的密码；以后通过后台修改密码，保留旧环境变量也不会覆盖数据库内的设置。系统设置保存后立即生效，并撤销全部登录会话。

### 源码构建

在源码根目录执行：

```sh
docker build -t gatesync:local .
```

然后将 Compose 的 image 改为 `gatesync:local`，运行 `docker compose up -d --pull never`。发行 Compose 仅声明镜像，不包含 build 配置。

## 启动参数

Go 直接运行不自动读取 `.env`；通过终端或服务管理器设置环境变量。Compose 自动读取同目录 `.env`，但仅传入文件中声明的变量；默认只传初始化密码。

| 变量 | 默认值与用途 |
| --- | --- |
| GATESYNC_ADDRESS | 本地为 `127.0.0.1:13850`；镜像内为 `0.0.0.0:13850` |
| GATESYNC_DATA_DIR | 本地为 `data`；镜像内为 `/data` |
| GATESYNC_WEB_DIR | 留空时发行程序使用内嵌前端；显式设置时使用外部前端目录 |
| GATESYNC_ACCESS_PASSWORD | 仅首次初始化使用，12–72 字节；已有系统设置时可不填 |
| GATESYNC_MASTER_KEY | 可选 Base64 编码的 32 字节主密钥，优先于 `master.key`；Compose 需自行声明传入 |

公开地址、登录及任务配置以后台保存值为准。兼容旧部署时，原 `GATESYNC_PUBLIC_URL`、`GATESYNC_PASSWORD_HASH`、`GOOGLE_CLIENT_ID`、`GOOGLE_CLIENT_SECRET`、`GOOGLE_ALLOWED_EMAILS` 仅在首次建立系统设置时导入；旧 `TZ` 仅在任务配置尚无时区时导入。此后更改这些环境变量不会覆盖后台设置。

首次运行自动生成加密主密钥，后续启动必须保留原密钥。当前使用单实例 SQLite，不支持多个进程共享同一数据目录。

## Google OAuth

在 [Google Cloud 控制台](https://console.cloud.google.com/auth/clients)创建 Web 应用 OAuth 客户端，完成 OAuth 同意屏幕配置，并根据 Google 要求添加测试用户。

通过密码登录后台，在“系统设置”中填写：

| 配置 | 本机示例 |
| --- | --- |
| 公开访问地址 | `http://localhost:13850` |
| Google 登录 | 启用 |
| Client ID / Secret | Google 生成的 Web 客户端凭证 |
| 邮箱白名单 | 允许访问的完整邮箱，每行一个，不支持通配符 |

Google 客户端的授权重定向 URI 必须精确匹配：

```text
http://localhost:13850/api/auth/google/callback
```

保存后重新登录即可生效，无需重启容器。Secret 加密保存且不回显，编辑时留空保留已有值。仅允许由 Google 确认已验证且完整邮箱匹配白名单的账号；访问密码登录始终保留。

局域网多设备访问时，使用可解析的 HTTPS 域名，并同步更新后台公开地址和 Google 回调地址。Google 的回调安全规则不允许普通局域网 IP 的 HTTP 地址。登录使用 Authorization Code + PKCE、一次性 state、nonce 和服务端 ID Token 校验。会话时长见 [使用指南](usage.md#登录)。参考：[Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect)。

## 反向代理与局域网

默认端口映射为 `127.0.0.1:13850:13850`，仅供本机访问。办公室其他设备访问时，将 Compose ports 的宿主地址改为运行设备的局域网 IP，并在后台更新公开访问地址；公开地址设置不会改变监听地址或端口映射。

通过同机 HTTPS 反向代理访问时可保留环回映射：

```nginx
location / {
    proxy_pass http://127.0.0.1:13850;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

后台公开地址须与浏览器实际访问地址一致；HTTPS 地址会使会话 Cookie 使用 Secure 标志。程序不信任转发头来决定放行 IP，检测的是运行设备的出口，须核对 VPN、透明代理或 Docker Desktop 是否改变了出口。

设备须保持在线并禁止自动休眠。操作方法见 [使用指南](usage.md)。

## 备份与恢复

停止应用后，备份整个 `./data` 目录。文件至少包括：

| 文件 | 内容 |
| --- | --- |
| gatesync.db | 配置、密文凭证、系统设置、归属台账、会话和日志 |
| gatesync.db-wal / gatesync.db-shm | SQLite WAL 文件，存在时一并备份 |
| master.key | 自动生成的 32 字节加密主密钥 |

恢复数据库、原主密钥和归属台账的同一份快照，核对目录权限，再启动单个实例。使用环境变量主密钥时须恢复相同值。

仅有数据库无法解密已有凭证和系统设置；主密钥丢失后不能恢复。本地台账用于识别可删除的云规则，不应单独丢弃。

## 升级与回退

1. 停止服务，备份整个数据目录及启动环境。
2. 更新 Compose 或指定镜像版本，运行 `docker compose pull` 和 `docker compose up -d`。
3. 检查登录、账号、目标、任务设置和日志，再恢复定时运行。

从旧命名卷部署迁移时，先停止旧实例并完整导出数据到 `./data`，包括数据库、主密钥及存在的 WAL 文件；核对权限后再使用新 Compose。验收完成前保留旧卷备份，禁止两个实例同时使用同一数据库。

从旧 8080 端口升级时，核对端口映射，并在后台把公开访问地址和 Google 控制台回调 URI 同步改为实际地址。旧登录配置只导入一次，后续以后台为准。

回退前核对数据库结构兼容性；若不兼容，应恢复对应版本的数据快照。不要移动已有版本标签，镜像升级不会自动更新云端规则。

## 故障排查

| 现象 | 检查 |
| --- | --- |
| 首次启动提示缺少密码 | 设置 12–72 字节初始化密码；Go 本地运行不会自动加载 .env |
| 主密钥缺失或无法解密 | 恢复原主密钥，勿生成替代密钥覆盖 |
| 读取地域失败 | 核对云凭证、网络和服务返回的错误码；安全组地域查询需 cvm:DescribeRegions，勿添加无效的 lighthouse:DescribeRegions；可手动填写地域 |
| 腾讯云连接或同步失败 | 核对目标类型、地域、CAM 权限、配额及日志错误码，见 [CAM 授权](cam.md) |
| 页面 IP 与预期不符 | 检查运行设备出口、VPN 和网关；不是浏览器访问者 IP |
| 所有检测源不可用 | 检查外网连通，或配置其他返回纯文本 IPv4 的 HTTPS 源 |
| Google 登录失败 | 核对后台配置、回调 URI、测试用户与邮箱白名单 |
| 规则同步后仍无法访问 | 检查规则优先级、资源关联、路由和主机防火墙 |
| 定时任务不执行 | 确认全局及目标自动同步开启，设备在线，核对后台时区 / Cron |
| 数据目录权限不足 | 核对容器 UID/GID 65532 和宿主目录权限 |

健康检查执行 `gatesync healthcheck`，或 `docker compose exec gatesync /gatesync healthcheck`。该命令仅访问已运行的 HTTP 健康接口，不初始化数据库或读取密码。
