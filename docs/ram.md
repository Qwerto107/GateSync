# 阿里云 RAM 授权

[文档中心](README.md) · [使用指南](usage.md) · [腾讯云 CAM 授权](cam.md)

创建专用 RAM 用户，为所需资源类型关联下方自定义策略，将 AccessKey ID / AccessKey Secret 填入“云账号”，厂商选择“阿里云”。同时管理 ECS 和轻量应用服务器时，关联两份策略。凭证加密保存，已有账号不能切换厂商。

## ECS 安全组

支持现有 VPC 安全组的 IPv4 TCP / UDP 入站放行。可读取地域和安全组，新增规则、更新已归属规则的备注、按规则 ID 删除；不创建安全组，不管理出站规则或实例生命周期。

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecs:DescribeSecurityGroups",
        "ecs:DescribeSecurityGroupAttribute",
        "ecs:AuthorizeSecurityGroup",
        "ecs:ModifySecurityGroupRule",
        "ecs:RevokeSecurityGroup"
      ],
      "Resource": "*"
    }
  ]
}
```

接口与授权点见：[查询安全组](https://help.aliyun.com/zh/ecs/developer-reference/api-ecs-2014-05-26-describesecuritygroups)、[查询规则](https://help.aliyun.com/zh/ecs/developer-reference/api-ecs-2014-05-26-describesecuritygroupattribute)、[新增规则](https://help.aliyun.com/zh/ecs/developer-reference/api-ecs-2014-05-26-authorizesecuritygroup)、[修改规则](https://help.aliyun.com/zh/ecs/developer-reference/api-ecs-2014-05-26-modifysecuritygrouprule)、[删除规则](https://help.aliyun.com/zh/ecs/developer-reference/api-ecs-2014-05-26-revokesecuritygroup)。

## 轻量应用服务器

后台目标类型选择“轻量应用服务器防火墙”，地域如 `cn-hangzhou`、`ap-southeast-1`，资源 ID 是控制台中的 32 位实例 ID。可从列表选择，也可手动填写。

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "swas-open:ListInstances",
        "swas-open:ListFirewallRules",
        "swas-open:CreateFirewallRules",
        "swas-open:ModifyFirewallRule",
        "swas-open:DeleteFirewallRules"
      ],
      "Resource": "*"
    }
  ]
}
```

新增使用支持 `SourceCidrIp` 的批量接口，明确携带每条规则的来源；删除使用规则 ID，默认放行的 22 / 80 / 443 等用户规则不会被接管。一个目标的 IP 与端口组合最多 50 条，实际可创建数量还受实例剩余配额限制；先新增再删除需要预留配额。

接口与授权点见：[查询实例](https://help.aliyun.com/zh/simple-application-server/developer-reference/api-swas-open-2020-06-01-listinstances)、[查询规则](https://help.aliyun.com/zh/simple-application-server/developer-reference/api-swas-open-2020-06-01-listfirewallrules)、[批量新增](https://help.aliyun.com/zh/simple-application-server/developer-reference/api-swas-open-2020-06-01-createfirewallrules)、[修改规则](https://help.aliyun.com/zh/simple-application-server/developer-reference/api-swas-open-2020-06-01-modifyfirewallrule)、[批量删除](https://help.aliyun.com/zh/simple-application-server/developer-reference/api-swas-open-2020-06-01-deletefirewallrules)。

## 地域查询与资源范围

ECS 使用 `DescribeRegions`，轻量应用服务器使用 `ListRegions`。这两个接口的官方页面目前均未提供授权点，不将未经列明的 action 加入示例 JSON。地域读取失败时仍可手动填写，地区是否可用还取决于账号及站点。

来源：[ECS 地域](https://help.aliyun.com/zh/ecs/developer-reference/api-ecs-2014-05-26-describeregions)、[轻量服务器地域](https://help.aliyun.com/zh/simple-application-server/developer-reference/api-swas-open-2020-06-01-listregions)。

示例中的 `Resource: "*"` 覆盖当前账号下对应资源。需要收窄时按每个接口官方授权表配置资源 ARN，不能将所有操作统一替换成安全组 ARN：部分 ECS 操作仅支持全部资源，轻量服务器的操作还区分实例和防火墙规则资源。显式拒绝、权限边界和组织策略同样会影响调用。

## 同步与排查

- 保留现有多 IP、共享端口、手动及定时查询、合并通知行为。公网 IP 未变化时，定时任务不读取云端规则。
- 写入前重新读取完整规则快照，发现变化则停止；新增后核验匹配字段，再删除已确认归属的旧规则。稳定规则 ID 会保存到台账。
- 阿里云接口没有与腾讯云相同的服务端规则版本锁。快照检查不能消除检查之后的并发修改窗口，避免同时在控制台修改本工具管理的规则。
- 既有同值规则、非本工具归属规则和带额外匹配条件的 ECS 规则不接管；遇到冲突会保留旧规则并提示处理。
- 权限错误会显示接口、建议检查的 RAM action 和可用的 RequestId；原始错误消息中的账号、资源及请求内容不直接回显。
- 连接测试仅验证资源读取，不代表新增、删除和备注更新权限已具备。
