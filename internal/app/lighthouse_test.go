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
)

func TestLighthouseDeleteRequiresExactRuleAndVersion(t *testing.T) {
	for _, mode := range []string{"valid", "duplicate", "version"} {
		t.Run(mode, func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("X-TC-Action") == "DescribeFirewallRules" {
					rules := []map[string]any{{"Protocol": "TCP", "Port": "22", "CidrBlock": "8.8.8.8/32", "Action": "ACCEPT", "FirewallRuleDescription": "gatesync:owned"}}
					if mode == "duplicate" {
						rules = append(rules, map[string]any{"Protocol": "TCP", "Port": "22", "CidrBlock": "8.8.8.8/32", "Action": "ACCEPT", "FirewallRuleDescription": "manual"})
					}
					version := 5
					if mode == "version" {
						version = 6
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Response": map[string]any{"FirewallVersion": version, "FirewallRuleSet": rules, "TotalCount": len(rules), "RequestId": "test"}})
					return
				}
				called = true
				if r.Header.Get("X-TC-Action") != "DeleteFirewallRules" {
					t.Error("意外操作", r.Header.Get("X-TC-Action"))
				}
				var body struct {
					InstanceID string `json:"InstanceId"`
					Version    uint64 `json:"FirewallVersion"`
					Rules      []struct {
						CIDR        string `json:"CidrBlock"`
						Description string `json:"FirewallRuleDescription"`
						Action      string `json:"Action"`
					} `json:"FirewallRules"`
				}
				if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
					t.Error(e)
				}
				if body.InstanceID != "lhins-test" || body.Version != 5 || len(body.Rules) != 1 || body.Rules[0].CIDR != "8.8.8.8/32" || body.Rules[0].Description != "gatesync:owned" || body.Rules[0].Action != "ACCEPT" {
					t.Error("删除缺少精确匹配条件", body)
				}
				_, _ = w.Write([]byte(`{"Response":{"RequestId":"test"}}`))
			}))
			defer server.Close()
			p := profile.NewClientProfile()
			p.HttpProfile.Endpoint = strings.TrimPrefix(server.URL, "http://")
			p.HttpProfile.Scheme = "HTTP"
			client, e := lighthouse.NewClient(common.NewCredential("test", "test"), "ap-guangzhou", p)
			if e != nil {
				t.Fatal(e)
			}
			c := &Lighthouse{client}
			e = c.Delete(context.Background(), "lhins-test", "5", []Rule{{Protocol: "TCP", Port: "22", CIDR: "8.8.8.8/32", Action: "ACCEPT", Description: "gatesync:owned"}})
			if mode == "valid" {
				if e != nil || !called {
					t.Fatal(e, called)
				}
			} else if e == nil || called {
				t.Fatal("不安全的删除未被阻止", mode, e, called)
			}
		})
	}
}
