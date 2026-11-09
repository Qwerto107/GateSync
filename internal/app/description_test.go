package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	lighthouse "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/lighthouse/v20200324"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

func TestDescriptionAdaptersPreserveConditionsAndVersion(t *testing.T) {
	for _, kind := range []string{"security-group", "lighthouse"} {
		t.Run(kind, func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("X-TC-Action") == "DescribeFirewallRules" {
					_, _ = w.Write([]byte(`{"Response":{"FirewallVersion":5,"TotalCount":1,"FirewallRuleSet":[{"Protocol":"TCP","Port":"22","CidrBlock":"8.8.8.8/32","Action":"ACCEPT","FirewallRuleDescription":"old"}]}}`))
					return
				}
				called = true
				var body struct {
					Version uint64                     `json:"FirewallVersion"`
					Rule    lighthouse.FirewallRule    `json:"FirewallRule"`
					Set     vpc.SecurityGroupPolicySet `json:"SecurityGroupPolicySet"`
				}
				if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
					t.Fatal(e)
				}
				if kind == "lighthouse" {
					if r.Header.Get("X-TC-Action") != "ModifyFirewallRuleDescription" || body.Version != 5 || value(body.Rule.Protocol) != "TCP" || value(body.Rule.Port) != "22" || value(body.Rule.CidrBlock) != "8.8.8.8/32" || value(body.Rule.Action) != "ACCEPT" || value(body.Rule.FirewallRuleDescription) != "new" {
						t.Error("备注请求改变了规则或缺少版本", body)
					}
				} else {
					if r.Header.Get("X-TC-Action") != "ReplaceSecurityGroupPolicy" || value(body.Set.Version) != "5" || len(body.Set.Ingress) != 1 || len(body.Set.Egress) != 0 {
						t.Error("请求没有限制为单条入站规则", body)
						return
					}
					rule := body.Set.Ingress[0]
					if value(rule.PolicyIndex) != 7 || value(rule.Protocol) != "TCP" || value(rule.Port) != "22" || value(rule.CidrBlock) != "8.8.8.8/32" || value(rule.Action) != "ACCEPT" || value(rule.PolicyDescription) != "new" {
						t.Error("备注请求改变了规则", rule)
					}
				}
				_, _ = w.Write([]byte(`{"Response":{"RequestId":"test"}}`))
			}))
			defer server.Close()
			p := profile.NewClientProfile()
			p.HttpProfile.Endpoint = strings.TrimPrefix(server.URL, "http://")
			p.HttpProfile.Scheme = "HTTP"
			credential := common.NewCredential("test", "test")
			var cloud Cloud
			if kind == "lighthouse" {
				client, e := lighthouse.NewClient(credential, "ap-guangzhou", p)
				if e != nil {
					t.Fatal(e)
				}
				cloud = &Lighthouse{client}
			} else {
				client, e := vpc.NewClient(credential, "ap-guangzhou", p)
				if e != nil {
					t.Fatal(e)
				}
				cloud = &Tencent{client}
			}
			old := testRule("8.8.8.8/32", "old")
			old.Index = 7
			if e := cloud.UpdateDescription(context.Background(), "resource-test", "5", old, "new"); e != nil || !called {
				t.Fatal(e, called)
			}
		})
	}
}
