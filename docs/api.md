# API 参考

[文档中心](README.md) · [使用指南](usage.md) · [开发指南](development.md)

[约定](#请求约定) · [登录](#登录与会话) · [状态](#状态与设置) · [账号](#云账号) · [目标](#放行目标) · [日志](#操作日志)

## 请求约定

基础路径为 `/api`，默认站点 <http://localhost:13850>。除健康、登录配置、登录及 Google 授权接口外，均须携带 `gatesync_session` Cookie。API 不返回云端 SecretKey 或完整 SecretId。

POST / PUT / PATCH / DELETE 请求必须设置 `X-GateSync-Request: 1`；JSON 请求再设置 `Content-Type: application/json`。JSON 请求体上限为 64 KiB，不接受未知字段或多个 JSON 值。浏览器请求的 Origin 必须匹配服务允许的来源，不提供跨站 CORS。

错误格式：

```json
{"error":"请先登录"}
```

| 状态码 | 含义 |
| --- | --- |
| 200 | 成功；保存目标后须继续检查返回对象的同步状态 |
| 303 | Google 授权回调完成或失败后跳转 |
| 400 | 参数或 JSON 不合法 |
| 401 | 未登录、会话失效或密码错误 |
| 403 | 请求来源或专用请求头校验失败 |
| 404 | 接口、账号或目标不存在 |
| 409 | 排序请求与当前目标列表不一致 |
| 429 | 密码登录或 OAuth 请求过于频繁 |
| 500 | 本地存储或内部操作失败 |
| 502 | 检测源或腾讯云操作失败 |

腾讯云错误包含脱敏错误码、调用接口和有效 RequestId。权限错误优先显示服务端明确返回的“被拒绝的权限”；未返回具体 action 时，仅显示对应接口的“建议检查权限”，不代表已经确认缺少该授权。应同时核对策略关联、资源范围和显式拒绝条件。原始消息及其中账号、资源或请求参数不回显。不得在客户端日志中记录真实凭证或 Cookie。

## 登录与会话

| 方法与路径 | 用途 |
| --- | --- |
| GET /health | 无需登录，返回 `{"status":"ok"}` |
| GET /auth/config | 返回 `{"google":true}` 或 false，表示是否配置 Google 登录 |
| POST /auth/login | 访问密码登录 |
| POST /auth/logout | 撤销当前会话并清除 Cookie |
| GET /auth/google | 启动 Google 授权跳转 |
| GET /auth/google/callback | 服务端校验授权结果后跳转首页 |

登录请求：

```json
{"password":"你的访问密码","remember":true}
```

成功返回 `{"ok":true}` 并设置 HttpOnly 会话 Cookie。remember 为 true 时最多 7 天，否则最多 12 小时；Google 登录会话最多 12 小时。服务重启后所有旧会话失效。

脚本示例：

```sh
curl --fail --cookie-jar session.txt \
  -H 'X-GateSync-Request: 1' -H 'Content-Type: application/json' \
  --data '{"password":"replace-with-your-password","remember":false}' \
  http://localhost:13850/api/auth/login
curl --fail --cookie session.txt http://localhost:13850/api/state
```

示例密码需要替换，实际使用可从受控文件或交互输入传入。session.txt 包含登录会话，应妥善保管。

## 状态与设置

| 方法与路径 | 返回 |
| --- | --- |
| GET /state | accounts、targets、settings、ip、nextCheck、identity |
| POST /ip/detect | 重新检测运行设备出口 IPv4，返回 IP 状态 |
| PUT /settings | 保存全局自动同步设置 |
| GET /system | 读取脱敏系统设置 |
| PUT /system | 保存系统及登录设置，并撤销全部会话 |

IP 状态包含 `ip`、`source`、`checkedAt`、`error`。nextCheck 为 RFC 3339 时间，任务关闭时为空。页面状态是本地记录及当前 IP 的比较结果，实时云端核验在同步时执行。

GET /state 及页面刷新只读取公网 IP 缓存，不发起外部查询。POST /ip/detect 始终主动检测。保存或编辑目标时仅使用可用缓存；若缓存为空或上次检测失败，保留已有规则并提示检测。

设置请求：

```json
{
  "enabled": true,
  "detectOnManualSync": true,
  "detectOnSchedule": true,
  "interval": 5,
  "cron": "",
  "timezone": "Asia/Shanghai",
  "sources": ["https://api.ipify.org", "https://ifconfig.me/ip"]
}
```

detectOnManualSync 与 detectOnSchedule 可独立设置，分别控制显式手动同步及后台定时同步前是否查询公网 IP，缺省均为 true。关闭时使用最近一次成功且未被失败状态标记的缓存，不自动补查。全部同步每轮最多检测一次，仅有固定 IP 目标时不检测；定时查询仍受全局和目标自动同步开关控制。定时任务仅在动态目标的公网 IP 变化时自动同步一次，地址未变或检测失败时不访问云端规则接口。每个目标的自动处理地址持久化；无历史地址时只建立基线，同一 IP 的同步失败须手动重试。纯固定 IP 目标不参与定时核对。

interval 支持 1–10080 分钟的整数，界面提供 1 / 5 / 10 / 15 / 30 / 60 分钟预设及自定义间隔。cron 非空时优先使用五段表达式，按 timezone 指定的有效 IANA 时区执行，留空默认为 Asia/Shanghai。sources 为 1–3 个公开 HTTPS 地址，只允许 443 端口和纯文本 IPv4 响应。

系统设置读取返回 publicUrl、googleEnabled、googleClientId、hasGoogleSecret、googleAllowedEmails，不返回密码哈希或 Google Secret。保存示例：

```json
{
  "publicUrl": "http://localhost:13850",
  "googleEnabled": false,
  "googleClientId": "",
  "googleClientSecret": "",
  "googleAllowedEmails": [],
  "currentPassword": "",
  "newPassword": ""
}
```

googleClientSecret 留空保留原密钥；newPassword 留空保留原密码，修改时须提供正确的 currentPassword，新密码为 12–72 字节。启用 Google 登录须同时配置 Client ID、Secret 和完整邮箱白名单。公开地址须为不含路径的 HTTP(S) 地址，不会更改服务监听端口。成功返回 `{"ok":true,"reauth":true}`，所有会话与未完成的 Google 授权立即失效。

## 云账号

| 方法与路径 | 请求及返回 |
| --- | --- |
| POST /accounts | 创建账号，返回脱敏账号对象 |
| PUT /accounts/{id} | 更新账号，返回脱敏账号对象 |
| POST /accounts/test | 测试表单中的凭证，返回 count 和 message |
| POST /accounts/{id}/test | 测试已保存账号并更新连接状态 |
| GET /accounts/{id}/regions?resourceType=lighthouse | 返回对应服务可用地域，省略类型使用账号默认类型 |
| POST /accounts/regions | 使用表单凭证读取地域，字段与表单测试相同 |
| GET /accounts/{id}/groups?region=ap-guangzhou&resourceType=security-group | 返回资源数组，省略参数使用账号默认地域、类型 |
| DELETE /accounts/{id}?revoke=false | 删除账号及本地目标，返回 ok |

创建、编辑的请求字段：

```json
{
  "name": "腾讯云 · 办公室",
  "resourceType": "security-group",
  "region": "ap-guangzhou",
  "secretId": "你的 SecretId",
  "secretKey": "你的 SecretKey"
}
```

编辑时 secretId 或 secretKey 留空表示保留对应原值。表单测试使用相同字段，并额外传入 `id`；新账号的 id 为空，已有账号可据此复用未修改的密钥。仅验证读取权限，不写入测试规则。

name 为可选目标名称，去除首尾空白后最多 60 个字符，旧目标可不提供，仅用于后台展示，不替代云端备注。

resourceType 为 security-group 或 lighthouse，省略默认为 security-group；同一账号可创建两类目标。账号对象字段为 id、name、provider、resourceType、region、maskedKey、status、testedAt、targetCount；当前 provider 为 tencent。

地域接口返回 `[{"id":"ap-guangzhou","name":"华南地区(广州)"}]`，只保留对应服务 AVAILABLE 的地域。安全组通过 CVM 查询，Lighthouse 使用自身地域接口；调用失败返回错误，前端可手动填写地域。资源对象字段为 id、name、description：安全组 ID 为 sg- 开头，Lighthouse 实例 ID 为 lhins- 开头。连接测试及列表接口只读取，不创建测试规则；权限 JSON 见 [CAM 授权](cam.md)。

删除的 revoke 默认为 false；显式 true 才撤销本工具创建的云规则。撤销未全部成功时返回错误并保留本地记录。

## 放行目标

| 方法与路径 | 用途 |
| --- | --- |
| POST /targets | 保存新目标并立即尝试同步 |
| PUT /targets/{id} | 更新目标并立即尝试同步 |
| PUT /targets/order | 保存完整目标展示顺序，不触发同步 |
| PATCH /targets/{id}/auto | 仅修改自动同步开关 |
| POST /targets/{id}/sync | 立即同步单个目标 |
| POST /sync | 立即同步所有目标 |
| DELETE /targets/{id}?revoke=false | 删除目标，按需撤销云规则 |

创建、编辑请求：

```json
{
  "name": "办公室 SSH",
  "accountId": "账号 ID",
  "resourceType": "security-group",
  "region": "ap-guangzhou",
  "groupId": "sg-xxxxxxxx",
  "groupName": "office-ssh",
  "ports": [{"protocol": "TCP", "value": "22"}],
  "ipMode": "dynamic",
  "fixedIps": [
    {"cidr": "8.8.8.8", "name": "备用出口"},
    {"cidr": "9.9.9.0/24", "name": "办公室网段"}
  ],
  "autoSync": true,
  "note": "办公室 SSH"
}
```

resourceType 为 security-group 或 lighthouse，默认 security-group；groupId / groupName 兼作 Lighthouse 的实例 ID / 名称，后者须使用 lhins- 开头的 ID。ipMode 为 dynamic 或 fixed，分别表示是否包含自动公网 IP；两者均可配置 fixedIps。fixedIps 为自定义 IPv4 / CIDR 对象数组，最多 20 项，name 可选、最多 90 字节；地址规范化后去重。fixed 模式至少提供一项，dynamic 可传空数组。所有来源共用 ports，来源与端口组合最多 100 条。旧 fixedCidr 字段仅在 fixedIps 缺省且模式为 fixed 时导入；显式空数组不会重新导入旧值。ports 为 1–50 项，protocol 为 TCP / UDP，value 为单端口或范围，范围为 1–65535。已有目标不可修改 resourceType、accountId、region 或 groupId。

排序请求为 `{"ids":["目标 ID 2","目标 ID 1"]}`，必须包含当前全部目标 ID 且无重复。成功返回 `{"ok":true}`，GET /state 的 targets 按保存顺序返回；旧目标默认按创建顺序，新目标追加末尾。成员已变化或无效时返回 409，原顺序保留，客户端应刷新列表后重试。该接口不更改目标配置或调用云 API。

自动同步开关请求为 `{"enabled":false}`，不会立即操作云端。

目标响应除上述配置外，还包含 id、currentCidr、currentCidrs、status、lastSync、error。status 为 pending / synced / outdated / failed / revoked。**创建或编辑可能返回 HTTP 200 且 status=failed，表示配置已保存，但同步未完成。**

currentCidrs 为上次已核验的来源列表；currentCidr 保留为兼容字段，在 dynamic 模式记录上次同步的动态地址，在 fixed 模式记录首个固定地址。检测失败时不将新检测失败的值替换为旧缓存继续清理规则；可补充固定规则，保留现有规则，返回失败状态提示未全部完成。

note 写入云端可读备注，附加稳定归属后缀。修改 note 时原地更新已确认归属规则的描述，旧格式标记可迁移；需要对应的单条备注修改权限，见 [CAM 授权](cam.md#云端备注更新权限)。

全部同步返回：

```json
{"success":2,"failed":1,"errors":{"失败目标ID":"脱敏错误原因"}}
```

单个目标失败不影响其他目标的结果。全局和目标自动同步开关不限制显式的手动同步。

删除 revoke=true 时只撤销本地台账内的归属规则，失败时保留本地目标。成功撤销但后续账号删除未完成的目标会停用自动同步。

## 操作日志

| 方法与路径 | 返回 |
| --- | --- |
| GET /logs | items、total、page；每页 20 条 |
| GET /logs/export | UTF-8 BOM CSV，最多最近 10,000 条 |

两个接口共用筛选参数：

| 参数 | 可选值 |
| --- | --- |
| days | 7 / 30 / 90，默认 7 |
| kind | ip / sync / account / target / settings / login |
| trigger | manual / auto |
| result | success / failed |
| q | 对象或详情搜索 |
| page | 从 1 开始，仅日志列表使用 |

日志项包含 id、time、kind、trigger、object、detail、success、changes。changes 为规则变更文本数组。时间使用 RFC 3339 UTC，CSV 导出应用相同筛选并转义潜在公式文本。
