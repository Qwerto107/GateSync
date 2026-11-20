# AWS IAM 授权

[文档中心](README.md) · [使用指南](usage.md) · [故障排查](deployment.md#故障排查)

## 账号与地域

添加云账号时选择 AWS，填写专用 IAM 用户的 Access Key ID / Secret Access Key。密钥仅在后端加密保存；编辑时留空保留原值。当前支持长期访问密钥，不支持需要 Session Token 的 STS 临时凭证、角色切换或读取主机凭证链。

目标类型为 EC2 / VPC 安全组，仅管理已有安全组的 TCP / UDP IPv4 入站规则。不创建安全组，不管理出站、IPv6、ICMP、前缀列表或安全组引用，不包含 Lightsail。

默认地域为 `us-east-1`。点击“读取可用地域”调用 `DescribeRegions`，只展示账号已启用的地域；API 不提供中文地域名，列表显示地域代码。中国区使用独立账号和密钥，应先填写 `cn-north-1` 或 `cn-northwest-1` 再查询，SDK 按地域解析对应站点的官方端点。读取失败时可手动填写地域。

来源：[地域查询](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeRegions.html)、[SDK 地域与凭证](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html)。

## 策略 JSON

将以下自定义策略关联到专用 IAM 用户。策略仅包含工具实际调用的操作；连接测试只验证安全组读取，不代表规则写入权限已经具备。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadRegionsAndSecurityGroups",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeRegions",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeSecurityGroupRules"
      ],
      "Resource": "*"
    },
    {
      "Sid": "ManageIngressRules",
      "Effect": "Allow",
      "Action": [
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:UpdateSecurityGroupRuleDescriptionsIngress",
        "ec2:RevokeSecurityGroupIngress"
      ],
      "Resource": "*"
    }
  ]
}
```

示例写入权限覆盖账号内安全组。生产使用可按官方授权表收窄写入的安全组 ARN 和地域条件；中国区 ARN 分区为 `aws-cn`，不要直接照搬全球区 ARN。上述三个读取操作不支持资源级 ARN，保留 `Resource: "*"`。权限边界、组织 SCP 和显式拒绝同样影响结果。

来源：[EC2 操作授权表](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ec2.html)、[新增入站规则](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_AuthorizeSecurityGroupIngress.html)、[更新入站备注](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_UpdateSecurityGroupRuleDescriptionsIngress.html)、[撤销入站规则](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_RevokeSecurityGroupIngress.html)。

## 备注与同步边界

- 本地备注保留完整原文。AWS 云端规则备注只保留其允许的英文、数字、空格和符号，再附加工具归属标记；过滤后为空使用 `GateSync`，过长可读部分按现有规则截短。中文不会发送到 AWS。
- 复用多个 IP 共用端口、自动去重、动态地址变化后同步、固定地址保留和合并通知。IP 不变时定时任务不读取安全组规则。
- 新增后重新读取并核验，再删除台账与归属标记匹配的旧规则。已有同值用户规则不接管；旧规则删除和备注更新均按稳定规则 ID 执行，核验后将 ID 绑定台账。
- AWS 没有本工具使用的服务端规则版本锁。写入前重读完整快照可发现已发生的变更，但无法消除检查之后的并发窗口，应避免同时修改本工具管理的规则。
- 遇到权限、配额、冲突或新规则核验失败时保留旧规则。先新增再删除需要预留规则配额；AWS 传播可能有延迟，核验未确认时可稍后手动重试。
- 权限错误显示脱敏错误码、接口、建议检查的 IAM action 和可用的 RequestId。建议检查项不代表已经确认缺少某一授权，原始错误中的资源或编码授权消息不回显。

来源：[安全组规则及备注限制](https://docs.aws.amazon.com/vpc/latest/userguide/security-group-rules.html)、[按安全组查询规则](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSecurityGroupRules.html)。
