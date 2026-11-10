# 开发指南

[文档中心](README.md) · [API 参考](api.md) · [发布流程](releasing.md)

[环境](#环境与结构) · [本地运行](#分离开发) · [构建](#完整程序与镜像) · [验证](#按需验证) · [实现约定](#数据与同步约定)

## 环境与结构

使用 Go 1.26（工具链为 `go.mod` 中的 Go 1.26.10）、Node.js 24 和 npm。SQLite 使用纯 Go 驱动，发行构建关闭 CGO；依赖由 `go.sum` 和 `web/package-lock.json` 固定。

```text
cmd/gatesync/          服务入口、退出处理与健康检查命令
internal/app/          认证、配置、SQLite、HTTP、调度和腾讯云适配
internal/webui/        按构建标签提供内嵌页面或本地目录
web/src/pages/        登录与四个主页面
web/src/components/   共享界面、账号和目标表单
scripts/              内嵌前端准备脚本
.github/              检查、打包与发布
compose.yaml          容器运行配置
docs/                 使用、部署和开发文档
```

## 分离开发

第一个 PowerShell 终端从源码根目录启动后端：

```powershell
$env:GATESYNC_ACCESS_PASSWORD = 'replace-with-your-random-password'
go run ./cmd/gatesync
```

第二个终端启动前端：

```powershell
cd web
npm ci
npm run dev
```

打开 <http://127.0.0.1:5173>。Vite 将 `/api` 代理到后端 `127.0.0.1:13850`。数据默认保存在根目录 `data/`，Go 不自动加载 `.env`。

未启用 `with_frontend` 时，后端默认从 `web/dist` 提供页面；未构建该目录时，可使用 Vite 页面访问后端。Google 回调按后台“系统设置”的公开地址生成，建议在完整程序的同源站点测试 OAuth。

## 完整程序与镜像

### 构建完整程序

从源码根目录执行：

```sh
npm --prefix web ci --no-audit --no-fund
npm --prefix web run build
node scripts/prepare-webui.mjs
```

准备脚本只复制前端构建产物到 `internal/webui/dist`，拒绝隐藏文件、源码映射和符号链接。该目录由构建生成，不提交版本控制。

```sh
CGO_ENABLED=0 go build -tags=with_frontend -trimpath -ldflags='-s -w' -o bin/gatesync ./cmd/gatesync
```

PowerShell：

```powershell
$env:CGO_ENABLED = '0'
go build -tags=with_frontend -trimpath -ldflags='-s -w' -o bin/gatesync.exe ./cmd/gatesync
```

发行程序从任意工作目录均能提供内嵌页面；配置和数据路径仍按 [部署指南](deployment.md#启动参数) 解析。外部页面目录可通过 `GATESYNC_WEB_DIR` 覆盖。

### 构建镜像

```sh
docker build -t gatesync:local .
```

Dockerfile 默认 standalone 目标从源码构建；分发用 compose.yaml 只拉取镜像，不依赖源码或本地构建。CI 的 release 目标复用已打包并校验的 Linux 完整程序，运行层为 scratch，包含 CA 证书、时区和 UID/GID 65532 的用户信息，无需目标架构模拟器。

`gatesync healthcheck` 只读取监听地址并检查已运行的 HTTP 接口，不创建数据库。容器不依赖 Shell 或 wget 进行健康检查。

## 按需验证

日常 CI 只做 Go 格式、静态检查、现有单元测试和前端类型检查、构建。先前检查已通过且输入没有变化时，不重复运行。

```sh
gofmt -l cmd internal
go vet ./...
go test ./... -count=1 -timeout=300s
npm --prefix web run build
```

仅在需要时手动启用 CI 的 `full_checks`：竞态测试替代普通测试，增加可达依赖漏洞检查和简单容器健康、首页检查。标签构建也执行可达依赖漏洞检查。CI 的准确条件和构建矩阵见 [发布流程](releasing.md)。

不在 CI 中增加浏览器端到端、性能、覆盖率门槛或其他数据库服务。已有本地验证和外部联调限制见 [验证记录](VALIDATION.md)。

## 数据与同步约定

- SQLite 使用 WAL 与单连接；账号、目标和计划相关写操作通过应用锁串行化。
- SecretId / SecretKey 由后端使用 AES-256-GCM 加密，随机 nonce，并以账号 ID 绑定密文。
- 云厂商差异通过 `Cloud` 接口集中在适配器，腾讯云包含安全组与 Lighthouse 两种实现。
- 系统登录配置加密存入 SQLite；环境变量只在系统设置尚不存在时导入，后台修改不会被启动值覆盖。
- 创建前保存本地台账；新规则重新核验后才删除旧规则；删除同时检查随机归属备注、本地台账、规则版本和索引。
- 目标归属不可原地更换。部分撤销失败后，已撤销目标关闭自动同步。
- 定时任务只在公网 IP 变化时同步一次；未变化时不读取云端规则，同一 IP 的失败由手动同步重试。
- 通知事件通过统一 `notificationSender` 接口发送。渠道配置加密保存，任务写入 SQLite 后由独立后台队列发送；通知失败不回滚检测或同步结果。
- 主密钥与台账是恢复凭证、识别旧规则所需的数据，不能只保留数据库结构而丢弃这些内容。

## 代码与提交

关键逻辑使用中文注释，沿用现有技术栈和目录，保持改动集中。独立需求对应独立提交，遵循 Conventional Commits；代码须经用户验收并明确授权后才能提交和推送。
