package app

import (
	"fmt"
	"regexp"
	"strings"
)

var deniedOperation = regexp.MustCompile(`(?i)(?:operation|action)\s*[\(\[]\s*(?:name/)?((?:cvm|vpc|lighthouse):[A-Za-z][A-Za-z0-9]{0,80})\s*[\)\]]`)
var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{1,80}$`)

// 这里只提供已核对的接口权限提示；明确的被拒绝 action 必须来自腾讯云错误消息。
var permissionHints = map[string]string{
	"cvm:DescribeRegions":                      "cvm:DescribeRegions",
	"vpc:DescribeSecurityGroups":               "cvm:DescribeSecurityGroups / vpc:DescribeSecurityGroups",
	"vpc:DescribeSecurityGroupPolicies":        "cvm:DescribeSecurityGroupPolicys",
	"vpc:CreateSecurityGroupPolicies":          "cvm:CreateSecurityGroupPolicy",
	"vpc:DeleteSecurityGroupPolicies":          "cvm:DeleteSecurityGroupPolicies / cvm:DeleteSecurityGroupPolicy",
	"vpc:ReplaceSecurityGroupPolicy":           "cvm:ModifySingleSecurityGroupPolicy",
	"lighthouse:DescribeInstances":             "lighthouse:DescribeInstances",
	"lighthouse:DescribeFirewallRules":         "lighthouse:DescribeFirewallRules",
	"lighthouse:CreateFirewallRules":           "lighthouse:CreateFirewallRules",
	"lighthouse:DeleteFirewallRules":           "lighthouse:DeleteFirewallRules",
	"lighthouse:ModifyFirewallRuleDescription": "lighthouse:ModifyFirewallRuleDescription",
}

func cloudError(e error, operation string) error {
	if e == nil {
		return nil
	}
	c, ok := e.(interface{ GetCode() string })
	if !ok {
		return fmt.Errorf("腾讯云连接失败（接口 %s），请检查网络或凭证", operation)
	}
	code := c.GetCode()
	detail := "腾讯云请求失败：" + code + "；接口：" + operation
	lower := strings.ToLower(code)
	if strings.Contains(lower, "unauthorizedoperation") || strings.Contains(lower, "nopermission") || strings.Contains(lower, "permissiondenied") {
		actual := ""
		if message, ok := e.(interface{ GetMessage() string }); ok {
			if match := deniedOperation.FindStringSubmatch(message.GetMessage()); len(match) == 2 {
				actual = match[1]
			}
		}
		if actual != "" {
			detail += "；被拒绝的权限：" + actual
		} else if hint := permissionHints[operation]; hint != "" {
			detail += "；建议检查权限：" + hint + "（腾讯云未返回具体拒绝 action）"
		}
		detail += "；请核对策略是否关联当前账号、资源范围及显式拒绝条件"
	}
	// 原始消息可能包含账号、资源或请求参数，只提取 action 和可用于排查的 RequestId。
	if request, ok := e.(interface{ GetRequestId() string }); ok && safeRequestID.MatchString(request.GetRequestId()) {
		detail += "；RequestId：" + request.GetRequestId()
	}
	return fmt.Errorf("%s", detail)
}
