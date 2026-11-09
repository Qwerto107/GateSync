# 二进制部署

[文档中心](https://github.com/Qwerto107/GateSync/blob/main/docs/README.md) · [部署指南](https://github.com/Qwerto107/GateSync/blob/main/docs/deployment.md)

发行程序已内嵌前端，同时提供管理页面和 API。解压后即可运行，无需安装 Go、Node.js 或 SQLite 动态库。本说明也随安装包提供。

[下载](#下载与校验) · [Linux](#linux) · [Windows](#windows) · [macOS](#macos) · [配置](#配置与数据) · [升级](#升级)

## 下载与校验

从 [GitHub Releases](https://github.com/Qwerto107/GateSync/releases) 下载压缩包及同名 `.sha256` 文件；手动构建的产物在 [Actions](https://github.com/Qwerto107/GateSync/actions/workflows/ci.yml) 中下载。附件是否存在，以对应运行或 Release 实际结果为准。

| 系统／架构 | 压缩包 |
| --- | --- |
| Linux x86-64 | gatesync-linux-amd64.tar.gz |
| Linux ARM64 | gatesync-linux-arm64.tar.gz |
| Linux ARMv6 | gatesync-linux-armv6.tar.gz |
| Linux ARMv5 | gatesync-linux-armv5.tar.gz |
| Linux LoongArch64 | gatesync-linux-loong64.tar.gz |
| Windows x86-64 | gatesync-windows-amd64.zip |
| macOS Apple Silicon | gatesync-darwin-arm64.tar.gz |

每份包包含程序和 README。ARMv5、ARMv6 按设备指令集选择；darwin 表示 macOS。

## Linux

在压缩包所在目录执行，以 x86-64 为例：

```sh
sha256sum -c gatesync-linux-amd64.tar.gz.sha256
tar -xzf gatesync-linux-amd64.tar.gz
cd gatesync-linux-amd64
export GATESYNC_ACCESS_PASSWORD='replace-with-your-random-password'
./gatesync
```

其他 Linux 平台替换包名。

## Windows

在 PowerShell 中执行：

```powershell
$archive = 'gatesync-windows-amd64.zip'
$expected = (Get-Content "$archive.sha256").Split(' ')[0]
if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne $expected) {
    throw '文件校验失败，请重新下载'
}
Expand-Archive $archive -DestinationPath .
Set-Location gatesync-windows-amd64
$env:GATESYNC_ACCESS_PASSWORD = 'replace-with-your-random-password'
./gatesync.exe
```

## macOS

```sh
shasum -a 256 -c gatesync-darwin-arm64.tar.gz.sha256
tar -xzf gatesync-darwin-arm64.tar.gz
cd gatesync-darwin-arm64
export GATESYNC_ACCESS_PASSWORD='replace-with-your-random-password'
./gatesync
```

## 配置与数据

默认打开 <http://localhost:13850>，使用访问密码或已配置的 Google 账号登录。直接运行不会读取 `.env`，通过终端或服务管理器设置环境变量。

初始化密码须为 12–72 字节，只在首次建立系统配置时导入；之后从后台修改密码，重启无需再设置初始化密码。重启会撤销旧会话。监听地址用 `GATESYNC_ADDRESS` 设置，Google OAuth 与公开地址见 [部署指南](https://github.com/Qwerto107/GateSync/blob/main/docs/deployment.md#启动参数)。

| 默认路径 | 内容 |
| --- | --- |
| data/gatesync.db | SQLite 配置、密文凭证、归属台账和日志 |
| data/master.key | 自动生成的加密主密钥 |
| data/gatesync.db-wal / data/gatesync.db-shm | SQLite 运行中的 WAL 文件 |

路径相对进程工作目录。运行账号须具有写入权限，长期运行应固定工作目录，或用 `GATESYNC_DATA_DIR` 指定绝对路径。数据库与原主密钥必须一起备份。

`GATESYNC_WEB_DIR` 留空使用内嵌页面；显式设置时改用对应目录。云账号、目标和计划任务通过管理页面配置。检测的是该设备所在网络的出口 IP，设备休眠期间不会自动同步。

运行 `./gatesync healthcheck`，Windows 使用 `./gatesync.exe healthcheck`，可检查 `GATESYNC_ADDRESS` 对应的已运行服务。返回非零表示失败，该命令不会读取密码或初始化数据库。

## 升级

停止旧进程，备份数据目录和原主密钥。校验并解压新版本，保留原启动环境和工作目录，再启动程序。前端随程序更新。

数据恢复、权限核对及数据库结构变化后的回退要求见 [备份与恢复](https://github.com/Qwerto107/GateSync/blob/main/docs/deployment.md#备份与恢复) 和 [升级与回退](https://github.com/Qwerto107/GateSync/blob/main/docs/deployment.md#升级与回退)。

## 源码打包

前端构建、内嵌准备及 Go 构建命令见 [开发指南](https://github.com/Qwerto107/GateSync/blob/main/docs/development.md#完整程序与镜像)。
