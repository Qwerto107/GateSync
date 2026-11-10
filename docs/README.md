# 文档中心

[项目首页](../README.md) · [下载版本](https://github.com/Qwerto107/GateSync/releases)

## 使用

| 要做的事 | 文档 |
| --- | --- |
| 添加云账号、配置端口、同步公网 IP | [使用指南](usage.md) |
| 设置自动检查、查看同步记录 | [计划任务](usage.md#计划任务) · [操作日志](usage.md#操作日志) |
| 配置飞书机器人与自定义卡片模板 | [通知渠道](notifications.md) |
| 通过脚本调用接口 | [API 参考](api.md) |
| 复制安全组或 Lighthouse 的 CAM 策略 | [CAM 授权](cam.md) |
| 查看版本变化 | [更新记录](changelog.md) |

## 部署

推荐使用 Docker Compose。发行程序包含前端，可直接运行；两种方式共用配置和运维约定。

| 要做的事 | 文档 |
| --- | --- |
| Docker 安装、设置 Google OAuth、HTTPS 和数据路径 | [部署指南](deployment.md) |
| 直接运行 Linux、Windows 或 macOS 程序 | [二进制部署](binary.md) |
| 备份、恢复、升级或排查故障 | [备份与恢复](deployment.md#备份与恢复) · [升级与回退](deployment.md#升级与回退) · [故障排查](deployment.md#故障排查) |

## 开发

| 要做的事 | 文档 |
| --- | --- |
| 本地开发、源码构建与按需验证 | [开发指南](development.md) |
| 配置 CI、构建产物与发布版本 | [发布流程](releasing.md) |
| 查看已有本地验证范围 | [验证记录](VALIDATION.md) |

## 默认地址

| 运行方式 | 地址 | 命令执行位置 |
| --- | --- | --- |
| Docker Compose | <http://localhost:13850> | 独立运行目录，含 compose.yaml、首次初始化 .env 和 ./data |
| 发行程序 | <http://localhost:13850> | 解压后的程序目录 |
| 本地前端开发 | <http://127.0.0.1:5173> | 源码仓库，前后端分别启动 |

发行程序与镜像的公开状态，以对应仓库的 Actions 和 Release 实际结果为准。
