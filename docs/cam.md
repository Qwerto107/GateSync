# 腾讯云 CAM 授权

[文档中心](README.md) · [使用指南](usage.md) · [部署指南](deployment.md)

建议创建专用 CAM 子账号，按使用的目标类型选择策略；同时管理两类目标时，可将两个策略分别关联到同一子账号。将生成的 SecretId / SecretKey 填入后台“云账号”。

下方 JSON 只包含地域、资源读取及规则新增、单条描述更新和删除权限，不包含创建或删除安全组、创建或终止实例、重置整套规则等权限。默认资源范围为本账号全部对应资源，限制方法见文末。

## VPC 安全组

```json
{
  "version": "2.0",
  "statement": [
    {
      "effect": "allow",
      "action": [
        "cvm:DescribeRegions",
        "cvm:DescribeSecurityGroups",
        "vpc:DescribeSecurityGroups"
      ],
      "resource": ["*"]
    },
    {
      "effect": "allow",
      "action": [
        "cvm:DescribeSecurityGroupPolicies",
        "cvm:DescribeSecurityGroupPolicys",
        "vpc:DescribeSecurityGroupPolicies",
        "cvm:CreateSecurityGroupPolicy",
        "cvm:ModifySingleSecurityGroupPolicy",
        "cvm:DeleteSecurityGroupPolicies",
        "cvm:DeleteSecurityGroupPolicy"
      ],
      "resource": ["*"]
    }
  ]
}
```

程序通过 VPC API 读取安全组与规则，策略包含 `vpc:DescribeSecurityGroups` 和 `vpc:DescribeSecurityGroupPolicies`。安全组资源路径仍使用 `qcs::cvm:...`，不能据此将所有 action 都写成 cvm 前缀。读取 action 可对照[腾讯云官方服务角色策略](https://cloud.tencent.com/document/product/598/117108)与[VPC CAM 操作表](https://cloud.tencent.com/document/product/598/98194)。

实际调用 `DescribeSecurityGroupPolicies` 时，腾讯云可能按旧操作名 `cvm:DescribeSecurityGroupPolicys` 鉴权；注意结尾是 **Policys**。本项目只读联调已收到要求该 action 的 UnauthorizedOperation，因此策略同时列出该操作。它也列在[官方 CVM CAM 操作表](https://cloud.tencent.com/document/product/598/69910)中，不能仅使用 API 名称中的 Policies 替代。

新增规则的 CAM 操作名为单数 `cvm:CreateSecurityGroupPolicy`，删除权限包含 CAM 支持的两种同用途命名；见[腾讯云 CAM 云服务器表](https://cloud.tencent.com/document/product/598/69910)和[官方策略操作示例](https://cloud.tencent.com/document/product/598/74184)。API 名、CAM action 与资源路径须分别核对。

### 安全组同步提示 UnauthorizedOperation

先确认自定义策略已关联到生成当前 SecretId 的子账号或角色，且不存在其他显式拒绝策略。仅创建或保存策略不会自动完成授权。

程序会在权限错误中显示接口、服务端明确返回的“被拒绝的权限”及 RequestId；未返回具体 action 时改为“建议检查权限”，不能据此排除策略未关联、资源范围不匹配或显式拒绝。请先依据服务端实际 action 排查。若读取安全组规则时提示缺少 `cvm:DescribeSecurityGroupPolicys`，在原策略的 statement 中补充以下读取权限并关联到调用者，再重新同步：

```json
{
  "effect": "allow",
  "action": [
    "cvm:DescribeSecurityGroupPolicys"
  ],
  "resource": ["*"]
}
```

此语句只授予读取规则权限。实际鉴权名可能与调用的 VPC API 名不同；仅添加 `vpc:DescribeSecurityGroupPolicies` 不能替代错误明确要求的旧操作名。`UnauthorizedOperation` 可能由不同操作或策略条件触发；若补充后仍失败，应结合失败接口、实际 action 和 RequestId 核对 CAM 生效策略、资源归属及限制条件，不要直接改为 `cvm:*` 或 `vpc:*`。

## Lighthouse 防火墙

```json
{
  "version": "2.0",
  "statement": [
    {
      "effect": "allow",
      "action": ["lighthouse:DescribeInstances"],
      "resource": ["*"]
    },
    {
      "effect": "allow",
      "action": [
        "lighthouse:DescribeFirewallRules",
        "lighthouse:CreateFirewallRules",
        "lighthouse:ModifyFirewallRuleDescription",
        "lighthouse:DeleteFirewallRules"
      ],
      "resource": ["*"]
    }
  ]
}
```

只管理轻量应用服务器的账号，将后台默认目标类型设为“Lighthouse 防火墙”，读取地域、实例和连接测试均使用 Lighthouse API。无需为此账号额外授予安全组权限。操作及资源范围见[官方 CAM Lighthouse 操作表](https://cloud.tencent.com/document/product/598/69972)与[可授权资源类型](https://cloud.tencent.com/document/product/1207/56101)。

Lighthouse 的 `DescribeRegions` API 存在，但当前 CAM 可授权操作表未列出同名 action。不要在策略中添加 `lighthouse:DescribeRegions`，否则控制台可能报“无效的 action”。程序仍通过该 API 查询地域；查询失败时可手动输入地域。API 名与 CAM 操作不能直接一一拼接。

## 云端备注更新权限

迁移旧标记或修改目标备注时，程序原地修改单条归属规则的描述，需要额外的 CAM action（上方完整策略已包含）：

| 类型 | API | CAM action |
| --- | --- | --- |
| 安全组 | ReplaceSecurityGroupPolicy | cvm:ModifySingleSecurityGroupPolicy |
| Lighthouse | ModifyFirewallRuleDescription | lighthouse:ModifyFirewallRuleDescription |

安全组保持原协议、端口、来源和动作，携带当前版本与索引，只修改已确认归属的单条规则。Lighthouse 更新前再次核对版本和规则唯一性。工具不调用重置整套规则的接口；权限不足或更新核验失败时，保留旧规则并返回错误。

安全组操作映射见 [API 2.0 / 3.0 对照](https://cloud.tencent.com/document/product/215/81827)与 [CVM CAM 表](https://cloud.tencent.com/document/product/598/69910)，Lighthouse 见 [CAM 表](https://cloud.tencent.com/document/product/598/69972)与 [单条描述修改 API](https://cloud.tencent.com/document/product/1207/55529)。

## 限制到指定资源

保留第一个语句的地域、列表读取权限，将第二个语句的 `resource` 改为需要管理的资源。替换其中地域、主账号 UIN 和资源 ID，可填写多个资源：

| 类型 | 资源示例 |
| --- | --- |
| 安全组 | `qcs::cvm:ap-guangzhou:uin/12345678:sg/sg-xxxxxxxx` |
| Lighthouse 实例 | `qcs::lighthouse:ap-guangzhou:uin/12345678:instance/lhins-xxxxxxxx` |

使用[CAM 策略生成器](https://cloud.tencent.com/document/product/598/37739)创建自定义策略并关联子账号。以上为策略示例，生效范围还受账号上其他允许、拒绝策略以及实际资源归属影响；未执行真实账号授权联调。

连接测试只验证当前类型的资源读取权限，不写入试探规则。请先用独立测试安全组或实例验收规则新增、单条描述更新和删除权限，再添加正式资源。
