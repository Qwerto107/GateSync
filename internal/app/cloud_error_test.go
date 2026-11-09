package app

import (
	"errors"
	"strings"
	"testing"

	tcerr "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

func TestPermissionErrorUsesServerActionAndRedactsMessage(t *testing.T) {
	e := tcerr.NewTencentCloudSDKError("UnauthorizedOperation", "you are not authorized to perform operation (cvm:DescribeSecurityGroupPolicys) resource private-account secret=do-not-expose", "request-123")
	message := cloudError(e, "vpc:DescribeSecurityGroupPolicies").Error()
	for _, expected := range []string{"被拒绝的权限：cvm:DescribeSecurityGroupPolicys", "接口：vpc:DescribeSecurityGroupPolicies", "RequestId：request-123"} {
		if !strings.Contains(message, expected) {
			t.Fatal(message)
		}
	}
	if strings.Contains(message, "private-account") || strings.Contains(message, "do-not-expose") || strings.Contains(message, "建议检查权限") {
		t.Fatal("回显了原始消息或混淆实际 action 与提示", message)
	}
}

func TestPermissionHintDoesNotClaimActualDeniedAction(t *testing.T) {
	e := tcerr.NewTencentCloudSDKError("UnauthorizedOperation.NoPermission", "no permission", "request-456")
	message := cloudError(e, "lighthouse:ModifyFirewallRuleDescription").Error()
	if !strings.Contains(message, "建议检查权限：lighthouse:ModifyFirewallRuleDescription") || !strings.Contains(message, "未返回具体拒绝 action") || strings.Contains(message, "被拒绝的权限") {
		t.Fatal(message)
	}
	message = cloudError(e, "lighthouse:DescribeRegions").Error()
	if strings.Contains(message, "建议检查权限") {
		t.Fatal("生成了未核实的 CAM action", message)
	}
	message = cloudError(tcerr.NewTencentCloudSDKError("AuthFailure.SecretIdNotFound", "secret=do-not-expose", "bad\nrequest"), "vpc:DescribeSecurityGroups").Error()
	if strings.Contains(message, "建议检查权限") || strings.Contains(message, "do-not-expose") || strings.Contains(message, "bad\nrequest") {
		t.Fatal(message)
	}
	if cloudError(nil, "vpc:DescribeSecurityGroups") != nil || strings.Contains(cloudError(errors.New("secret=do-not-expose"), "vpc:DescribeSecurityGroups").Error(), "do-not-expose") {
		t.Fatal("非 SDK 错误泄露消息")
	}
}
