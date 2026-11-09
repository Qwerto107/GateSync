<div align="center">

# GateSync

可自托管的云安全组端口放行工具。定时检测公网 IP，自动更新 VPC 安全组或 Lighthouse 实例防火墙的入站规则。

[快速部署](#快速部署) · [文档中心](docs/README.md) · [API 参考](docs/api.md) · [下载版本](https://github.com/Qwerto107/GateSync/releases)

**Go + React + TypeScript** · **SQLite** · **腾讯云**

一个容器或一个完整程序，同时提供管理页面和 API。

</div>

---

## 界面一览

| 页面 | 用途 |
| --- | --- |
| 放行规则 | 查看公网 IP、目标状态，管理端口并执行同步 |
| 计划任务 | 设置检查频率、Cron 与 IP 检测源，查看执行记录 |
| 云账号 | 管理云厂商凭证、读取对应服务地域、测试连接 |
| 系统设置 | 设置站点地址、Google 登录和访问密码 |
| 操作日志 | 筛选操作记录、展开规则变更、导出 CSV |

页面支持浅色、深色主题与窄屏布局。具体操作见 [使用指南](docs/usage.md)。

## 功能

| 能力 | 使用方式 |
| --- | --- |
| 云账号 | 管理多个云账号，动态获取对应服务可用地域，凭证加密保存；当前支持腾讯云 |
| 放行目标 | 选择 VPC 安全组或 Lighthouse 实例，支持直接输入资源 ID |
| 端口配置 | TCP / UDP、单端口或端口范围，每个目标可配置多个端口 |
| IP 来源 | 自动公网 IPv4 与多个自定义 IPv4 / CIDR 可同时选择，共用端口并自动去重 |
| 规则同步 | 单个或全部同步；先新增并核验新规则，再撤销旧规则 |
| 自动同步 | 1 / 5 / 15 分钟或五段 Cron，每个目标独立启停，后续检查重试失败目标 |
| 登录 | 访问密码或 Google OAuth，Google 账号通过完整邮箱白名单授权 |
| 操作追踪 | 记录 IP 检测、规则变更、账号与登录操作，支持筛选、分页和 CSV 导出 |

工具仅维护带有本实例归属标记、且存在于本地台账中的规则。删除目标或账号时默认保留云端规则，可选择同时撤销。

## 快速部署

推荐在本机或办公室内持续在线的设备上使用 Docker Compose。已有公开镜像版本后，只需下载 Compose 配置，无需克隆源码。

Linux 新建专用运行目录：

```sh
mkdir -p gatesync/data
cd gatesync
sudo chown 65532:65532 data
sudo chmod 700 data
curl -fL https://raw.githubusercontent.com/Qwerto107/GateSync/main/compose.yaml -o compose.yaml
```

新建 `.env`，设置首次初始化密码（12–72 字节）：

```dotenv
GATESYNC_ACCESS_PASSWORD='replace-with-your-random-password'
```

```sh
docker compose up -d --pull always
```

1. 打开 <http://localhost:13850>，使用初始化密码登录。
2. 在“系统设置”中配置站点地址、Google OAuth、邮箱白名单及后续访问密码。
3. 在“云账号”中添加腾讯云凭证和默认目标类型，按需读取地域并测试连接。
4. 在“放行规则”中选择安全组或 Lighthouse 实例，添加端口并同步。
5. 在“计划任务”中设置时区、检查频率和检测源，按需开启自动同步。

默认端口为 13850，仅本机可访问；数据直接保存在运行目录 `./data`，不使用命名卷。初始化密码只导入一次，之后可删除 `.env` 中的密码项，后台保存值不会被环境变量覆盖。

Windows、HTTPS、目录权限与升级见 [部署指南](docs/deployment.md)，权限 JSON 见 [CAM 授权](docs/cam.md)。直接运行程序见 [二进制部署](docs/binary.md)。Compose 和镜像是否已公开，以仓库及 GHCR 的实际发布结果为准。

## 支持范围

- 支持腾讯云现有安全组与 Lighthouse 实例防火墙的 IPv4 入站放行规则，不创建安全组或实例。
- 检测的是运行设备所在网络的出口 IP；不会根据浏览器访问者的 IP 自动授权。
- 当前按单实例运行，SQLite 与主密钥须放在本机持久化磁盘，并一起备份。
- 所有授权用户均具有管理权限；当前不提供用户角色、通知、IPv6 或其他云厂商接入。
- 同步成功表示规则已写入并核验，访问结果仍受安全组优先级、路由和主机防火墙影响。

## 文档导航

| 内容 | 文档 |
| --- | --- |
| 可复制的安全组、Lighthouse 权限策略 JSON | [CAM 授权](docs/cam.md) |
| 云账号、放行目标、自动同步与日志 | [使用指南](docs/usage.md) |
| Docker、环境变量、Google OAuth、反向代理与备份 | [部署指南](docs/deployment.md) |
| Linux、Windows、macOS 完整程序运行 | [二进制部署](docs/binary.md) |
| API 请求、参数与返回值 | [API 参考](docs/api.md) |
| 本地开发、构建与按需验证 | [开发指南](docs/development.md) |
| 二进制、多架构镜像与自动 Release | [发布流程](docs/releasing.md) |
| 版本变化 | [更新记录](docs/changelog.md) |
