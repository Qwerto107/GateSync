# 发布流程

[文档中心](README.md) · [开发指南](development.md) · [部署指南](deployment.md)

工作流为 [构建、检查与发布](../.github/workflows/ci.yml)。交付内嵌前端的完整程序和统一镜像。Compose 使用镜像和 ./data 目录，默认端口为 13850。源码构建命令见 [开发指南](development.md#完整程序与镜像)。

[触发](#触发方式) · [构建顺序](#构建顺序) · [产物](#产物) · [镜像标签](#统一镜像) · [权限](#仓库权限) · [发布步骤](#发布与部署)

## 触发方式

| 触发 | 检查 | 产物与发布 |
| --- | --- | --- |
| 分支提交或 PR 命中工作流路径 | Go 格式、静态检查、现有测试，前端类型检查与构建 | 不打包、不发布 |
| 手动运行 | 常规检查 | 七平台压缩包作为 Actions 产物，不创建 Release |
| 手动启用 publish_images | 常规检查 | 七平台打包并发布提交 SHA 镜像，仅允许 main |
| 手动启用 full_checks | 用竞态测试替代普通测试，加依赖漏洞检查和容器健康／首页检查 | 是否推送镜像由 publish_images 决定 |
| 推送 vX.Y.Z | 常规检查及依赖漏洞检查 | 七平台附件、五架构镜像和正式 Release，更新 latest |
| 推送 vX.Y.Z-rc.1 等 | 同正式版 | 预发布 Release 和版本镜像，不更新 latest |

`full_checks` 默认关闭。CI 不启动其他数据库服务。日常检查保留最新提交；版本发布和手动镜像发布不中途取消。

工作流路径包含前后端、脚本、Docker 配置、`.github`，以及参与打包的 `docs/binary.md`、`docs/releasing.md`。仅修改其他说明文档时，不触发常规分支检查。

## 构建顺序

1. 后端运行检查，前端只构建一次，保存内嵌页面产物。
2. 七个平台复用这份页面，以 `CGO_ENABLED=0` 和 `with_frontend` 构建、打包并计算 SHA-256。
3. 镜像任务校验并复用 Linux 压缩包中的程序，不重新编译前后端。
4. 标签发布在镜像成功后创建 Release 草稿，上传并重新下载校验附件，完成后公开。

必要任务失败时，不公开新 Release。重跑只补充缺失附件，已有校验文件必须与本次构建一致。

## 产物

完整程序的平台与文件名见 [二进制说明](binary.md#下载与校验)。每个平台提供压缩包和同名 `.sha256` 文件，共七组。包内的 README 来自 `docs/binary.md`。

Actions 产物 `gatesync-binaries-{平台}` 保留 30 天，前端产物 `gatesync-webui-dist` 保留 7 天。压缩包固定文件排序、时间和所有者。旧版本是否包含附件，以对应 Release 为准。

## 统一镜像

镜像地址为 `ghcr.io/qwerto107/gatesync`，支持 `linux/amd64`、`linux/arm64`、`linux/arm/v6`、`linux/arm/v5` 和 `linux/loong64`。

| 场景 | 镜像标签 |
| --- | --- |
| 正式版 v1.2.3 | 1.2.3、latest、sha-完整提交号 |
| 预发布 v1.2.3-rc.1 | 1.2.3-rc.1、sha-完整提交号 |
| 手动启用镜像发布 | sha-完整提交号 |

main 提交不自动发布 dev 镜像。运行层为 `scratch`，包含完整程序、CA 证书、时区及非 root 用户信息；健康检查执行 `/gatesync healthcheck`，没有 Shell 依赖。

## 仓库权限

检查和打包使用 `contents: read`，镜像任务额外使用 `packages: write`，Release 任务使用 `contents: write`，均通过内置 `GITHUB_TOKEN`。

GHCR 包需允许本仓库写入。匿名拉取还需把包可见性设为 Public；代码仓库公开不代表镜像自动公开。

## 发布与部署

1. 核对前端版本信息、界面版本及 [更新记录](changelog.md)，将“未发布”内容归入对应版本。
2. 验收后提交并推送代码，确认版本标签将指向该提交。
3. 创建并推送版本标签，由 Actions 构建镜像、附件和 Release。

以下版本号仅为格式示例：

```sh
git tag -a v1.2.3 -m "GateSync v1.2.3"
git push origin v1.2.3
```

Actions 自动创建 Release，无需本地发布 Release，也不移动已有标签。产物和版本的公开状态以该次工作流结果为准。

流水线只构建和交付，不保存服务器 SSH 凭据或更新部署。安装、升级和回退见 [部署指南](deployment.md#升级与回退)，直接运行程序见 [二进制部署](binary.md)。

## 下载产物

- [Actions 构建记录](https://github.com/Qwerto107/GateSync/actions/workflows/ci.yml)：手动运行成功后下载七平台压缩包。
- [GitHub Releases](https://github.com/Qwerto107/GateSync/releases)：标签流水线成功后公开版本附件。
- 统一镜像：`ghcr.io/qwerto107/gatesync`。首次公开拉取前，需在 GHCR 中将包设为 Public。

配置工作流不等于已经发布版本；当前可下载的内容以页面实际结果为准。
