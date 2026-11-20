package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/dara"
)

type aliyunTestHTTP func(*http.Request, *http.Transport) (*http.Response, error)

func (f aliyunTestHTTP) Call(r *http.Request, tr *http.Transport) (*http.Response, error) {
	return f(r, tr)
}

type aliyunTestCloud struct {
	t        *testing.T
	kind     string
	rows     []map[string]any
	calls    []string
	sequence int
	failAdd  bool
}

func (m *aliyunTestCloud) Call(r *http.Request, _ *http.Transport) (*http.Response, error) {
	m.t.Helper()
	if r.URL.Scheme != "https" || !strings.HasSuffix(r.URL.Host, ".aliyuncs.com") || !strings.Contains(r.Header.Get("Authorization"), "ACS3-HMAC-SHA256") {
		m.t.Fatal("请求未使用阿里云签名和 HTTPS")
	}
	q := r.URL.Query()
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		for k, v := range form {
			q[k] = v
		}
	}
	action := r.Header.Get("x-acs-action")
	if action == "" {
		action = q.Get("Action")
	}
	if action == "" {
		for key, values := range r.Header {
			if strings.EqualFold(key, "x-acs-action") && len(values) > 0 {
				action = values[0]
			}
		}
	}
	m.calls = append(m.calls, action)
	result := map[string]any{"RequestId": "request-123"}
	switch action {
	case "DescribeSecurityGroupAttribute":
		result["Permissions"], result["VpcId"] = map[string]any{"Permission": m.rows}, "vpc-test"
	case "ListFirewallRules":
		result["FirewallRules"], result["TotalCount"] = m.rows, len(m.rows)
	case "AuthorizeSecurityGroup", "CreateFirewallRules":
		if m.failAdd {
			return &http.Response{StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"Code":"Forbidden.RAM","Message":"private-secret resource","RequestId":"request-123"}`))}, nil
		}
		prefix := "Permissions"
		if m.kind == "swas" {
			prefix = "FirewallRules"
		}
		for i := 1; ; i++ {
			key := fmt.Sprintf("%s.%d.", prefix, i)
			if q.Get(key+"SourceCidrIp") == "" {
				if i == 1 {
					m.t.Fatal("新增缺少来源 CIDR")
				}
				break
			}
			m.sequence++
			row := map[string]any{"SourceCidrIp": q.Get(key + "SourceCidrIp"), "Policy": "accept"}
			if m.kind == "swas" {
				row["RuleId"], row["RuleProtocol"], row["Port"], row["Remark"] = fmt.Sprintf("rule-%d", m.sequence), q.Get(key+"RuleProtocol"), q.Get(key+"Port"), q.Get(key+"Remark")
			} else {
				row["SecurityGroupRuleId"], row["IpProtocol"], row["PortRange"], row["Description"] = fmt.Sprintf("sgr-%d", m.sequence), q.Get(key+"IpProtocol"), q.Get(key+"PortRange"), q.Get(key+"Description")
				row["Priority"], row["Direction"] = q.Get(key+"Priority"), "ingress"
			}
			m.rows = append(m.rows, row)
		}
	case "RevokeSecurityGroup", "DeleteFirewallRules":
		prefix, idKey := "SecurityGroupRuleId", "SecurityGroupRuleId"
		if m.kind == "swas" {
			prefix, idKey = "RuleIds", "RuleId"
		}
		for i := 1; q.Get(fmt.Sprintf("%s.%d", prefix, i)) != ""; i++ {
			id := q.Get(fmt.Sprintf("%s.%d", prefix, i))
			for index, row := range m.rows {
				if row[idKey] == id {
					m.rows = append(m.rows[:index], m.rows[index+1:]...)
					break
				}
			}
		}
	case "ModifySecurityGroupRule", "ModifyFirewallRule":
		idKey, queryKey, noteKey := "SecurityGroupRuleId", "SecurityGroupRuleId", "Description"
		if m.kind == "swas" {
			idKey, queryKey, noteKey = "RuleId", "RuleId", "Remark"
			if q.Get("SourceCidrIp") == "" || q.Get("RuleProtocol") == "" || q.Get("Port") == "" {
				m.t.Fatal("修改备注丢失原匹配条件")
			}
		}
		for _, row := range m.rows {
			if row[idKey] == q.Get(queryKey) {
				row[noteKey] = q.Get(noteKey)
			}
		}
	case "DescribeRegions":
		result["Regions"] = map[string]any{"Region": []map[string]string{{"RegionId": "ap-southeast-1", "LocalName": "新加坡"}}}
	case "ListRegions":
		result["Regions"] = []map[string]string{{"RegionId": "cn-hangzhou", "LocalName": "杭州"}}
	case "DescribeSecurityGroups":
		if q.Get("NextToken") == "" {
			result["NextToken"] = "page-2"
			result["SecurityGroups"] = map[string]any{"SecurityGroup": []map[string]string{{"SecurityGroupId": "sg-one", "SecurityGroupName": "第一个"}}}
		} else {
			result["SecurityGroups"] = map[string]any{"SecurityGroup": []map[string]string{{"SecurityGroupId": "sg-two", "SecurityGroupName": "第二个"}}}
		}
	case "ListInstances":
		result["TotalCount"] = 101
		if q.Get("PageNumber") == "1" {
			result["Instances"] = []map[string]string{{"InstanceId": strings.Repeat("a", 32), "InstanceName": "第一个"}}
		} else {
			result["Instances"] = []map[string]string{{"InstanceId": strings.Repeat("b", 32), "InstanceName": "第二个"}}
		}
	default:
		m.t.Fatal("意外接口", action)
	}
	raw, _ := json.Marshal(result)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
}

func newAliyunTest(t *testing.T, kind string) (*Aliyun, *aliyunTestCloud) {
	t.Helper()
	cloud, e := newAliyun(Credentials{SecretID: "test-access-key", SecretKey: "test-secret"}, "ap-southeast-1", kind)
	if e != nil {
		t.Fatal(e)
	}
	c := cloud.(*Aliyun)
	m := &aliyunTestCloud{t: t, kind: kind, rows: []map[string]any{}}
	c.client.HttpClient = m
	return c, m
}

func TestAliyunSyncKeepsForeignRulesFixedIPsAndBindsIDs(t *testing.T) {
	for _, kind := range []string{"security-group", "swas"} {
		t.Run(kind, func(t *testing.T) {
			a, _ := testApp(t)
			c, m := newAliyunTest(t, kind)
			a.aliyun = func(Credentials, string, string) (Cloud, error) { return c, nil }
			target := sourceTarget(t, a)
			account, _ := a.store.account(target.AccountID)
			account.Provider = "aliyun"
			a.store.saveAccount(account, nil)
			target.ResourceType, target.Region, target.FixedIPs = kind, "ap-southeast-1", []FixedIP{{CIDR: "1.1.1.1/32"}}
			if kind == "swas" {
				target.GroupID = strings.Repeat("a", 32)
			}
			foreign := map[string]any{"SecurityGroupRuleId": "foreign", "RuleId": "foreign", "SourceCidrIp": "0.0.0.0/0", "IpProtocol": "tcp", "RuleProtocol": "TCP", "PortRange": "443/443", "Port": "443", "Policy": "accept", "Description": "用户规则", "Remark": "用户规则", "Priority": "1", "Direction": "ingress"}
			m.rows = append(m.rows, foreign)
			result, e := a.syncTarget(context.Background(), target, "8.8.8.8", "manual", false)
			if e != nil || len(m.rows) != 5 {
				t.Fatal(e, len(m.rows))
			}
			var ledger []Rule
			a.store.getMeta("owned:"+target.ID, &ledger)
			for _, r := range ledger {
				if r.ID == "" {
					t.Fatal("台账未绑定稳定规则 ID")
				}
			}
			result, e = a.syncTarget(context.Background(), result, "9.9.9.9", "auto", false)
			if e != nil || len(m.rows) != 5 {
				t.Fatal("动态变更未保留固定 IP", e, len(m.rows))
			}
			result.Note = "新备注"
			result, e = a.syncTarget(context.Background(), result, "9.9.9.9", "manual", false)
			if e != nil {
				t.Fatal(e)
			}
			m.failAdd = true
			if _, e = a.syncTarget(context.Background(), result, "4.4.4.4", "manual", false); e == nil || len(m.rows) != 5 {
				t.Fatal("失败时删除旧规则", e)
			}
			m.failAdd = false
			if _, e = a.syncTarget(context.Background(), result, "", "manual", true); e != nil || len(m.rows) != 1 || m.rows[0]["SourceCidrIp"] != "0.0.0.0/0" {
				t.Fatal("撤销误删用户规则", e, m.rows)
			}
		})
	}
}

func TestAliyunRefusesChangedSnapshotsAndSameValueForeignRules(t *testing.T) {
	for _, kind := range []string{"security-group", "swas"} {
		t.Run(kind, func(t *testing.T) {
			c, m := newAliyunTest(t, kind)
			want := Rule{Protocol: "TCP", Port: "8000-8100", CIDR: "8.8.8.8/32", Action: "ACCEPT", Description: "工具 [GS:test]"}
			set, e := c.Rules(context.Background(), "resource")
			if e != nil {
				t.Fatal(e)
			}
			if e := c.Add(context.Background(), "resource", set.Version, []Rule{want}); e != nil {
				t.Fatal(e)
			}
			set, e = c.Rules(context.Background(), "resource")
			if e != nil || set.Rules[0].Port != "8000-8100" {
				t.Fatal(e, set)
			}
			if e := c.Add(context.Background(), "resource", set.Version, []Rule{want}); e == nil {
				t.Fatal("接管同值已有规则")
			}
			candidate := set.Rules[0]
			if kind == "swas" {
				m.rows[0]["Remark"] = "用户接管"
			} else {
				m.rows[0]["Description"] = "用户接管"
			}
			if e := c.Delete(context.Background(), "resource", set.Version, []Rule{candidate}); e == nil || len(m.rows) != 1 {
				t.Fatal("快照改变后仍删除")
			}
			owned := ownedRules([]Rule{{ID: "new-id", Protocol: want.Protocol, Port: want.Port, CIDR: want.CIDR, Action: want.Action, Description: want.Description}}, []Rule{{ID: "old-id", Protocol: want.Protocol, Port: want.Port, CIDR: want.CIDR, Action: want.Action, Description: want.Description}}, "[GS:test]")
			if len(owned) != 0 {
				t.Fatal("按值接管被重建的规则")
			}
		})
	}
}

func TestAliyunRegionsGroupsCredentialsAndPermissionErrors(t *testing.T) {
	for _, kind := range []string{"security-group", "swas"} {
		c, mock := newAliyunTest(t, kind)
		regions, e := c.Regions(context.Background())
		if e != nil || len(regions) != 1 {
			t.Fatal(e, regions)
		}
		groups, e := c.Groups(context.Background())
		if e != nil || len(groups) != 2 {
			t.Fatal("分页错误", e, groups)
		}
		set, e := c.Rules(context.Background(), "resource")
		if e != nil {
			t.Fatal(e)
		}
		mock.failAdd = true
		e = c.Add(context.Background(), "resource", set.Version, []Rule{{Protocol: "TCP", Port: "22", CIDR: "8.8.8.8/32", Action: "ACCEPT", Description: "test"}})
		if e == nil || !strings.Contains(e.Error(), "RAM 权限") || !strings.Contains(e.Error(), "request-123") || strings.Contains(e.Error(), "private-secret") {
			t.Fatal("SDK 权限错误丢失或泄露原消息", e)
		}
	}
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	input := accountInput{Provider: "aliyun", ResourceType: "swas", Name: "阿里云", Region: "ap-southeast-1", SecretID: "never-expose-key", SecretKey: "never-expose-secret"}
	w := request(t, a, "POST", "/api/accounts", input, cookie)
	var account Account
	json.Unmarshal(w.Body.Bytes(), &account)
	if w.Code != 200 || account.Provider != "aliyun" || strings.Contains(w.Body.String(), input.SecretKey) || strings.Contains(w.Body.String(), input.SecretID) {
		t.Fatal(w.Code, w.Body.String())
	}
	input.Provider = "tencent"
	if w := request(t, a, "PUT", "/api/accounts/"+account.ID, input, cookie); w.Code != 400 {
		t.Fatal("允许更换已有厂商")
	}
	target := Target{AccountID: account.ID, ResourceType: "swas", GroupID: strings.Repeat("a", 32), Region: "ap-southeast-1", IPMode: "fixed", FixedIPs: []FixedIP{{CIDR: "1.1.1.1"}}, Ports: []Port{{Protocol: "TCP", Value: "22"}}}
	if e := validateTarget(&target); e != nil {
		t.Fatal(e)
	}
	if validProviderResource("aliyun", "lighthouse") || validProviderResource("tencent", "swas") {
		t.Fatal("资源类型跨厂商混用")
	}
	e := aliyunError(&openapi.ClientError{Code: dara.String("Forbidden.RAM"), Message: dara.String("private-secret resource-id"), RequestId: dara.String("request-123")}, "ecs:AuthorizeSecurityGroup")
	if !strings.Contains(e.Error(), "RAM 权限：ecs:AuthorizeSecurityGroup") || !strings.Contains(e.Error(), "request-123") || strings.Contains(e.Error(), "private-secret") {
		t.Fatal(e)
	}
	regionError := aliyunError(&openapi.ClientError{Code: dara.String("Unauthorized.EmptyRegion")}, "ecs:DescribeRegions")
	if strings.Contains(regionError.Error(), "RAM 权限：ecs:DescribeRegions") || !strings.Contains(regionError.Error(), "地域访问资格") {
		t.Fatal("地域查询误报可授权 action", regionError)
	}
}

func TestAliyunRulePaginationAndCancelledRequests(t *testing.T) {
	for _, kind := range []string{"security-group", "swas"} {
		c, _ := newAliyunTest(t, kind)
		calls := 0
		c.client.HttpClient = aliyunTestHTTP(func(r *http.Request, _ *http.Transport) (*http.Response, error) {
			calls++
			id, next := "first", "page-2"
			if r.URL.Query().Get("NextToken") != "" || r.URL.Query().Get("PageNumber") == "2" {
				id, next = "second", ""
			}
			row := map[string]any{"SecurityGroupRuleId": id, "RuleId": id, "SourceCidrIp": "8.8.8.8/32", "Policy": "accept", "IpProtocol": "tcp", "RuleProtocol": "TCP", "PortRange": "22/22", "Port": "22", "Priority": "1", "Direction": "ingress"}
			body := map[string]any{"VpcId": "vpc-test", "NextToken": next, "Permissions": map[string]any{"Permission": []any{row}}, "FirewallRules": []any{row}, "TotalCount": 101}
			raw, _ := json.Marshal(body)
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		})
		set, e := c.Rules(context.Background(), "resource")
		if e != nil || len(set.Rules) != 2 || calls != 2 || set.Rules[0].ID == set.Rules[1].ID {
			t.Fatal("规则分页丢失", kind, e, set)
		}
		c.client.HttpClient = aliyunTestHTTP(func(r *http.Request, _ *http.Transport) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		_, e = c.Rules(ctx, "resource")
		cancel()
		if e == nil {
			t.Fatal("忽略请求取消")
		}
	}
}
