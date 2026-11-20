package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type awsTestRow struct {
	ID          string `xml:"securityGroupRuleId"`
	GroupID     string `xml:"groupId"`
	Protocol    string `xml:"ipProtocol"`
	From        int    `xml:"fromPort"`
	To          int    `xml:"toPort"`
	CIDR        string `xml:"cidrIpv4,omitempty"`
	Description string `xml:"description"`
	Egress      bool   `xml:"isEgress"`
}

type awsTestCloud struct {
	t          *testing.T
	rows       []awsTestRow
	calls      []string
	sequence   int
	failAction string
	paginate   bool
	cycle      bool
	host       string
}

func (m *awsTestCloud) Do(r *http.Request) (*http.Response, error) {
	m.t.Helper()
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	if r.URL.Scheme != "https" || !strings.Contains(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
		m.t.Fatal("未使用 AWS HTTPS 签名请求")
	}
	m.host = r.URL.Host
	body, _ := io.ReadAll(r.Body)
	q, _ := url.ParseQuery(string(body))
	action := q.Get("Action")
	m.calls = append(m.calls, action)
	status, content := 200, ""
	if action == m.failAction {
		status, content = 403, `<Response><Errors><Error><Code>UnauthorizedOperation</Code><Message>private-secret resource-id encoded-authorization</Message></Error></Errors><RequestID>aws-request-123</RequestID></Response>`
	} else {
		switch action {
		case "DescribeRegions":
			content = `<regionInfo><item><regionName>us-east-1</regionName><optInStatus>opt-in-not-required</optInStatus></item><item><regionName>ap-southeast-1</regionName><optInStatus>opted-in</optInStatus></item><item><regionName>disabled-1</regionName><optInStatus>not-opted-in</optInStatus></item></regionInfo>`
		case "DescribeSecurityGroups":
			if q.Get("NextToken") == "" {
				content = `<nextToken>groups-page-2</nextToken><securityGroupInfo><item><groupId>sg-first</groupId><groupName>first</groupName><vpcId>vpc-test</vpcId></item></securityGroupInfo>`
			} else {
				content = `<securityGroupInfo><item><groupId>sg-second</groupId><groupName>second</groupName><vpcId>vpc-test</vpcId></item></securityGroupInfo>`
			}
		case "DescribeSecurityGroupRules":
			if q.Get("Filter.1.Name") != "group-id" || q.Get("Filter.1.Value.1") == "" {
				m.t.Fatal("缺少安全组过滤条件", q)
			}
			rows, token := m.rows, ""
			if m.paginate && len(rows) > 1 {
				if q.Get("NextToken") == "" {
					rows, token = rows[:1], "rules-page-2"
				} else {
					rows = rows[1:]
				}
			}
			if m.cycle {
				token = "stuck"
			}
			response := struct {
				XMLName xml.Name     `xml:"result"`
				Rows    []awsTestRow `xml:"securityGroupRuleSet>item"`
				Token   string       `xml:"nextToken,omitempty"`
			}{Rows: rows, Token: token}
			raw, err := xml.Marshal(response)
			if err != nil {
				m.t.Fatal(err)
			}
			content = strings.TrimSuffix(strings.TrimPrefix(string(raw), "<result>"), "</result>")
		case "AuthorizeSecurityGroupIngress":
			for i := 1; ; i++ {
				prefix := fmt.Sprintf("IpPermissions.%d.", i)
				if q.Get(prefix+"IpProtocol") == "" {
					break
				}
				from, _ := strconv.Atoi(q.Get(prefix + "FromPort"))
				to, _ := strconv.Atoi(q.Get(prefix + "ToPort"))
				m.sequence++
				row := awsTestRow{ID: fmt.Sprintf("sgr-%d", m.sequence), GroupID: q.Get("GroupId"), Protocol: q.Get(prefix + "IpProtocol"), From: from, To: to, CIDR: q.Get(prefix + "IpRanges.1.CidrIp"), Description: q.Get(prefix + "IpRanges.1.Description")}
				if row.CIDR == "" {
					m.t.Fatal("新增未明确指定来源")
				}
				m.rows = append(m.rows, row)
			}
			content = `<return>true</return>`
		case "RevokeSecurityGroupIngress":
			ids := map[string]bool{}
			for i := 1; q.Get(fmt.Sprintf("SecurityGroupRuleId.%d", i)) != ""; i++ {
				ids[q.Get(fmt.Sprintf("SecurityGroupRuleId.%d", i))] = true
			}
			if len(ids) == 0 {
				m.t.Fatal("撤销未使用稳定规则 ID")
			}
			keep := []awsTestRow{}
			for _, row := range m.rows {
				if !ids[row.ID] {
					keep = append(keep, row)
				}
			}
			m.rows, content = keep, `<return>true</return>`
		case "UpdateSecurityGroupRuleDescriptionsIngress":
			id, note := q.Get("SecurityGroupRuleDescription.1.SecurityGroupRuleId"), q.Get("SecurityGroupRuleDescription.1.Description")
			if id == "" || note == "" {
				m.t.Fatal("备注更新缺少 ID 或备注")
			}
			for i := range m.rows {
				if m.rows[i].ID == id {
					m.rows[i].Description = note
				}
			}
			content = `<return>true</return>`
		default:
			m.t.Fatal("意外接口", action)
		}
		content = "<" + action + `Response xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><requestId>aws-request-123</requestId>` + content + "</" + action + "Response>"
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/xml"}}, Body: io.NopCloser(strings.NewReader(content)), Request: r}, nil
}

func newAWSTest(t *testing.T, region string) (*AWS, *awsTestCloud) {
	t.Helper()
	cloud, err := newAWS(Credentials{SecretID: "test-key", SecretKey: "test-secret"}, region)
	if err != nil {
		t.Fatal(err)
	}
	c, mock := cloud.(*AWS), &awsTestCloud{t: t}
	opts := c.client.Options()
	opts.HTTPClient = mock
	c.client = ec2.New(opts)
	return c, mock
}

func TestAWSSyncKeepsForeignRulesFixedIPsAndBindsIDs(t *testing.T) {
	a, _ := testApp(t)
	c, m := newAWSTest(t, "us-east-1")
	a.aws = func(Credentials, string) (Cloud, error) { return c, nil }
	target := sourceTarget(t, a)
	account, _ := a.store.account(target.AccountID)
	account.Provider = "aws"
	if err := a.store.saveAccount(account, nil); err != nil {
		t.Fatal(err)
	}
	target.Region, target.Note, target.FixedIPs = "us-east-1", "办公室 SSH", []FixedIP{{CIDR: "1.1.1.1/32"}}
	m.rows = []awsTestRow{{ID: "sgr-user", GroupID: target.GroupID, Protocol: "tcp", From: 443, To: 443, CIDR: "0.0.0.0/0", Description: "User rule"}, {ID: "sgr-egress", GroupID: target.GroupID, Protocol: "tcp", From: 22, To: 22, CIDR: "8.8.8.8/32", Egress: true}}
	result, err := a.syncTarget(context.Background(), target, "8.8.8.8", "manual", false)
	if err != nil || len(m.rows) != 6 {
		t.Fatal("首次同步失败", err, len(m.rows))
	}
	var ledger []Rule
	if err := a.store.getMeta("owned:"+target.ID, &ledger); err != nil {
		t.Fatal(err)
	}
	for _, rule := range ledger {
		if rule.ID == "" || !strings.HasPrefix(rule.Description, "SSH [GS:") {
			t.Fatal("台账 ID 或中文过滤错误", rule)
		}
	}
	result, err = a.syncTarget(context.Background(), result, "9.9.9.9", "auto", false)
	if err != nil || len(m.rows) != 6 {
		t.Fatal("动态替换未保留固定 IP", err, len(m.rows))
	}
	result.Note = "办公室 New SSH"
	result, err = a.syncTarget(context.Background(), result, "9.9.9.9", "manual", false)
	if err != nil {
		t.Fatal("更新备注失败", err)
	}
	m.failAction = "AuthorizeSecurityGroupIngress"
	before := len(m.calls)
	if _, err = a.syncTarget(context.Background(), result, "4.4.4.4", "manual", false); err == nil || len(m.rows) != 6 {
		t.Fatal("失败后旧规则丢失", err)
	}
	attempts := 0
	for _, action := range m.calls[before:] {
		if action == m.failAction {
			attempts++
		}
	}
	if attempts != 1 || !strings.Contains(err.Error(), "IAM 权限：ec2:AuthorizeSecurityGroupIngress") || !strings.Contains(err.Error(), "aws-request-123") || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("错误脱敏、权限提示或无自动重试错误", err, attempts)
	}
	m.failAction = ""
	if _, err = a.syncTarget(context.Background(), result, "", "manual", true); err != nil || len(m.rows) != 2 {
		t.Fatal("撤销影响用户或出站规则", err, len(m.rows))
	}
}

func TestAWSRefusesChangedSnapshotsAndForeignRules(t *testing.T) {
	c, m := newAWSTest(t, "us-east-1")
	ctx := context.Background()
	want := Rule{Protocol: "TCP", Port: "8000-8100", CIDR: "8.8.8.8/32", Action: "ACCEPT", Description: "GateSync [GS:test]"}
	set, err := c.Rules(ctx, "sg-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Add(ctx, "sg-test", set.Version, []Rule{want}); err != nil {
		t.Fatal(err)
	}
	set, err = c.Rules(ctx, "sg-test")
	if err != nil || set.Rules[0].Port != want.Port {
		t.Fatal(err, set)
	}
	if err = c.Add(ctx, "sg-test", set.Version, []Rule{want}); err == nil {
		t.Fatal("接管同值原有规则")
	}
	old := set.Rules[0]
	m.rows[0].Description = "Changed by user"
	before := len(m.calls)
	if err = c.Delete(ctx, "sg-test", set.Version, []Rule{old}); err == nil || len(m.rows) != 1 || len(m.calls) != before+1 {
		t.Fatal("快照变更后仍写入", err)
	}
	set, err = c.Rules(ctx, "sg-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Delete(ctx, "sg-test", set.Version, []Rule{old}); err == nil {
		t.Fatal("同 ID 内容变化后仍删除")
	}
	m.rows[0].Description, m.rows[0].ID = old.Description, "sgr-recreated"
	set, err = c.Rules(ctx, "sg-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(ownedRules(set.Rules, []Rule{old}, "[GS:test]")) != 0 {
		t.Fatal("接管同值重建规则")
	}
	if err = c.UpdateDescription(ctx, "sg-test", set.Version, old, "New"); err == nil {
		t.Fatal("旧 ID 仍能修改备注")
	}
}

func TestAWSRegionsPaginationPartitionsAndCancellation(t *testing.T) {
	c, m := newAWSTest(t, "cn-north-1")
	regions, err := c.Regions(context.Background())
	if err != nil || len(regions) != 2 || m.host != "ec2.cn-north-1.amazonaws.com.cn" {
		t.Fatal("地域或中国区 endpoint 错误", err, regions, m.host)
	}
	groups, err := c.Groups(context.Background())
	if err != nil || len(groups) != 2 {
		t.Fatal("安全组分页", err, groups)
	}
	m.rows = []awsTestRow{{ID: "sgr-one", GroupID: "sg-test", Protocol: "6", From: 22, To: 22, CIDR: "8.8.8.8/32"}, {ID: "sgr-two", GroupID: "sg-test", Protocol: "udp", From: 53, To: 53, CIDR: "1.1.1.1/32"}}
	m.paginate = true
	set, err := c.Rules(context.Background(), "sg-test")
	if err != nil || len(set.Rules) != 2 || set.Rules[0].Protocol != "TCP" {
		t.Fatal("规则分页或协议解析", err, set)
	}
	m.cycle = true
	if _, err = c.Rules(context.Background(), "sg-test"); err == nil {
		t.Fatal("未停止重复分页")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.Regions(ctx); err == nil {
		t.Fatal("忽略取消请求")
	}
	if got := awsDescription("中文\n😀", "[GS:test]"); got != "GateSync [GS:test]" {
		t.Fatal(got)
	}
}

func TestAWSAccountCredentialsAndResourceBoundaries(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	input := accountInput{Provider: "aws", ResourceType: "security-group", Name: "AWS", Region: "us-east-1", SecretID: "never-expose-aws-key", SecretKey: "never-expose-aws-secret"}
	w := request(t, a, "POST", "/api/accounts", input, cookie)
	var account Account
	json.Unmarshal(w.Body.Bytes(), &account)
	if w.Code != 200 || account.Provider != "aws" || strings.Contains(w.Body.String(), input.SecretID) || strings.Contains(w.Body.String(), input.SecretKey) {
		t.Fatal("账号保存或脱敏错误", w.Code)
	}
	creds, err := a.store.credentials(account.ID)
	if err != nil || creds.SecretKey != input.SecretKey {
		t.Fatal("凭证加密读回错误", err)
	}
	input.Provider = "aliyun"
	if w := request(t, a, "PUT", "/api/accounts/"+account.ID, input, cookie); w.Code != 400 {
		t.Fatal("允许更换厂商")
	}
	input.Provider, input.ResourceType = "aws", "lighthouse"
	if w := request(t, a, "PUT", "/api/accounts/"+account.ID, input, cookie); w.Code != 400 {
		t.Fatal("允许混用目标类型")
	}
	if validProviderResource("aws", "swas") || !validProviderResource("aws", "") {
		t.Fatal("AWS 资源范围错误")
	}
}
