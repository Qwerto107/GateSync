package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/dara"
)

// 两类资源共用官方 OpenAPI 签名与请求客户端，接口差异留在适配器中。
type Aliyun struct {
	client       *openapi.Client
	region, kind string
}

type aliyunHTTPClient struct{}

func (aliyunHTTPClient) Call(request *http.Request, transport *http.Transport) (*http.Response, error) {
	client := &http.Client{Transport: transport, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client.Do(request)
}

func newAliyun(c Credentials, region, kind string) (Cloud, error) {
	if !regionPattern.MatchString(region) || !validProviderResource("aliyun", kind) {
		return nil, fmt.Errorf("阿里云地域或目标类型无效")
	}
	service := "ecs"
	if kind == "swas" {
		service = "swas"
	}
	client, e := openapi.NewClient(&openapi.Config{
		AccessKeyId: dara.String(c.SecretID), AccessKeySecret: dara.String(c.SecretKey),
		RegionId: dara.String(region), Endpoint: dara.String(service + "." + region + ".aliyuncs.com"),
		Protocol: dara.String("https"), ReadTimeout: dara.Int(20000), ConnectTimeout: dara.Int(5000),
		HttpClient: aliyunHTTPClient{},
	})
	if e != nil {
		return nil, fmt.Errorf("阿里云客户端初始化失败")
	}
	client.RetryOptions = &dara.RetryOptions{Retryable: false, MaxAttempts: 1}
	// 保留结构化错误，避免旧兼容包装丢失可脱敏提取的 Code 和 RequestId。
	client.DisableSDKError = dara.Bool(true)
	return &Aliyun{client: client, region: region, kind: kind}, nil
}

func (c *Aliyun) operation(action string) string {
	if c.kind == "swas" {
		return "swas-open:" + action
	}
	return "ecs:" + action
}

func aliyunError(err error, operation string) error {
	if err == nil {
		return nil
	}
	detail := "阿里云连接失败；接口：" + operation + "；请检查网络或访问密钥"
	var coded interface {
		GetCode() *string
		GetRequestId() *string
	}
	if errors.As(err, &coded) {
		code := value(coded.GetCode())
		if len(code) > 100 || strings.IndexFunc(code, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_')
		}) >= 0 {
			code = "UnknownError"
		}
		detail = "阿里云请求失败：" + code + "；接口：" + operation
		lower := strings.ToLower(code)
		if strings.Contains(lower, "forbidden") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "nopermission") || strings.Contains(lower, "accessdenied") {
			if operation == "ecs:DescribeRegions" || operation == "swas-open:ListRegions" {
				detail += "；请检查账号地域访问资格和有效凭证（此接口未列出可授权的 RAM action）"
			} else {
				detail += "；建议检查 RAM 权限：" + operation + "（该接口所需权限）；请核对策略关联、资源范围及显式拒绝条件"
			}
		}
		if id := value(coded.GetRequestId()); safeRequestID.MatchString(id) {
			detail += "；RequestId：" + id
		}
	}
	return errors.New(detail)
}

func (c *Aliyun) call(ctx context.Context, action string, params map[string]string, out any) error {
	query := map[string]*string{"RegionId": dara.String(c.region)}
	for k, v := range params {
		query[k] = dara.String(v)
	}
	version := "2014-05-26"
	if c.kind == "swas" {
		version = "2020-06-01"
	}
	result, e := c.client.CallApiWithCtx(ctx, &openapi.Params{Action: dara.String(action), Version: dara.String(version), Protocol: dara.String("HTTPS"), Pathname: dara.String("/"), Method: dara.String("POST"), AuthType: dara.String("AK"), Style: dara.String("RPC"), ReqBodyType: dara.String("formData"), BodyType: dara.String("json")}, &openapi.OpenApiRequest{Query: query}, &dara.RuntimeOptions{Autoretry: dara.Bool(false), MaxAttempts: dara.Int(1)})
	if e != nil {
		return aliyunError(e, c.operation(action))
	}
	if out == nil {
		return nil
	}
	raw, e := json.Marshal(result["body"])
	if e != nil || result["body"] == nil {
		return fmt.Errorf("阿里云响应内容无效；接口：%s", c.operation(action))
	}
	if e := json.Unmarshal(raw, out); e != nil {
		return fmt.Errorf("阿里云响应格式无效；接口：%s", c.operation(action))
	}
	return nil
}

func (c *Aliyun) Regions(ctx context.Context) ([]Region, error) {
	type region struct{ RegionId, LocalName string }
	var ecs struct{ Regions struct{ Region []region } }
	var swas struct{ Regions []region }
	items := []region{}
	if c.kind == "swas" {
		if e := c.call(ctx, "ListRegions", nil, &swas); e != nil {
			return nil, e
		}
		items = swas.Regions
	} else {
		if e := c.call(ctx, "DescribeRegions", map[string]string{"AcceptLanguage": "zh-CN"}, &ecs); e != nil {
			return nil, e
		}
		items = ecs.Regions.Region
	}
	out := []Region{}
	for _, r := range items {
		out = append(out, Region{r.RegionId, r.LocalName})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *Aliyun) Groups(ctx context.Context) ([]Group, error) {
	out := []Group{}
	token := ""
	for page := 1; ; page++ {
		if c.kind == "swas" {
			var res struct {
				TotalCount int
				Instances  []struct{ InstanceId, InstanceName, PublicIpAddress string }
			}
			if e := c.call(ctx, "ListInstances", map[string]string{"PageNumber": strconv.Itoa(page), "PageSize": "100"}, &res); e != nil {
				return nil, e
			}
			for _, g := range res.Instances {
				out = append(out, Group{g.InstanceId, g.InstanceName, g.PublicIpAddress})
			}
			if page*100 >= res.TotalCount {
				break
			}
		} else {
			var res struct {
				NextToken      string
				SecurityGroups struct {
					SecurityGroup []struct{ SecurityGroupId, SecurityGroupName, Description string }
				}
			}
			if e := c.call(ctx, "DescribeSecurityGroups", map[string]string{"MaxResults": "100", "NextToken": token, "NetworkType": "vpc", "ServiceManaged": "false"}, &res); e != nil {
				return nil, e
			}
			for _, g := range res.SecurityGroups.SecurityGroup {
				out = append(out, Group{g.SecurityGroupId, g.SecurityGroupName, g.Description})
			}
			if res.NextToken == "" {
				break
			}
			if res.NextToken == token {
				return nil, fmt.Errorf("阿里云资源分页未前进")
			}
			token = res.NextToken
		}
		if page >= 1000 {
			return nil, fmt.Errorf("阿里云资源列表过大，请缩小范围")
		}
	}
	return out, nil
}

func aliyunPort(port string) string {
	parts := strings.Split(port, "/")
	if len(parts) == 2 {
		if parts[0] == parts[1] {
			return parts[0]
		}
		return parts[0] + "-" + parts[1]
	}
	return port
}
func aliyunPortRange(port string) string {
	if strings.Contains(port, "-") {
		return strings.ReplaceAll(port, "-", "/")
	}
	return port + "/" + port
}

func (c *Aliyun) Rules(ctx context.Context, id string) (RuleSet, error) {
	out := RuleSet{Rules: []Rule{}}
	token, rawRules := "", []string{}
	for page := 1; ; page++ {
		var res struct {
			VpcId         string
			NextToken     string
			TotalCount    int
			Permissions   struct{ Permission []json.RawMessage }
			FirewallRules []json.RawMessage
		}
		params, action := map[string]string{"SecurityGroupId": id, "Direction": "ingress", "MaxResults": "1000", "NextToken": token}, "DescribeSecurityGroupAttribute"
		if c.kind == "swas" {
			action = "ListFirewallRules"
			params = map[string]string{"InstanceId": id, "PageSize": "100", "PageNumber": strconv.Itoa(page)}
		}
		if e := c.call(ctx, action, params, &res); e != nil {
			return out, e
		}
		if c.kind != "swas" && res.VpcId == "" {
			return out, fmt.Errorf("阿里云仅支持 VPC 安全组，云端未返回 VPC ID")
		}
		rows := res.Permissions.Permission
		if c.kind == "swas" {
			rows = res.FirewallRules
		}
		for _, raw := range rows {
			rawRules = append(rawRules, string(raw))
			var r struct{ SecurityGroupRuleId, RuleId, IpProtocol, RuleProtocol, PortRange, Port, SourceCidrIp, Policy, Description, Remark, Priority, SourcePortRange, DestCidrIp, Ipv6SourceCidrIp, SourceGroupId, SourcePrefixListId, PortRangeListId, Direction string }
			if e := json.Unmarshal(raw, &r); e != nil {
				return out, fmt.Errorf("阿里云规则格式无效")
			}
			rule := Rule{ID: r.SecurityGroupRuleId, Protocol: strings.ToUpper(r.IpProtocol), Port: aliyunPort(r.PortRange), CIDR: r.SourceCidrIp, Action: strings.ToUpper(r.Policy), Description: r.Description}
			if c.kind == "swas" {
				rule.ID, rule.Protocol, rule.Port, rule.Description = r.RuleId, strings.ToUpper(r.RuleProtocol), aliyunPort(r.Port), r.Remark
			}
			if cidr, e := normalizeCIDR(rule.CIDR); e == nil {
				rule.CIDR = cidr
			}
			// 带有额外匹配条件、不同优先级或非入站的 ECS 规则不属于本工具的简单放行规则。
			if c.kind != "swas" && (r.Priority != "1" || r.SourcePortRange != "" && r.SourcePortRange != "-1/-1" || r.DestCidrIp != "" && r.DestCidrIp != "0.0.0.0/0" || r.Ipv6SourceCidrIp != "" || r.SourceGroupId != "" || r.SourcePrefixListId != "" || r.PortRangeListId != "" || r.Direction != "ingress") {
				rule.Action = "UNSUPPORTED"
			}
			if rule.ID == "" {
				rule.Action = "UNSUPPORTED"
			}
			out.Rules = append(out.Rules, rule)
		}
		if c.kind == "swas" {
			if page*100 >= res.TotalCount {
				break
			}
		} else {
			if res.NextToken == "" {
				break
			}
			if res.NextToken == token {
				return out, fmt.Errorf("阿里云规则分页未前进")
			}
			token = res.NextToken
		}
		if page >= 1000 {
			return out, fmt.Errorf("阿里云规则列表过大，已停止同步")
		}
	}
	// API 无服务端 CAS 版本，以完整规则快照检测变更；写操作前重读，删除始终使用稳定 ID。
	sort.Strings(rawRules)
	sum := sha256.Sum256([]byte(strings.Join(rawRules, "\n")))
	out.Version = hex.EncodeToString(sum[:])
	return out, nil
}

func (c *Aliyun) checkedRules(ctx context.Context, id, version string) (RuleSet, error) {
	if version == "" {
		return RuleSet{}, fmt.Errorf("缺少阿里云规则快照，已停止修改")
	}
	set, e := c.Rules(ctx, id)
	if e != nil {
		return set, e
	}
	if set.Version != version {
		return set, fmt.Errorf("阿里云规则已变更，请重试")
	}
	return set, nil
}

func (c *Aliyun) Add(ctx context.Context, id, version string, rules []Rule) error {
	set, e := c.checkedRules(ctx, id, version)
	if e != nil {
		return e
	}
	for _, want := range rules {
		for _, existing := range set.Rules {
			if ruleKey(want) == ruleKey(existing) {
				return fmt.Errorf("阿里云已有同值规则，已停止新增，避免接管用户原有规则")
			}
		}
	}
	params := map[string]string{"SecurityGroupId": id, "ClientToken": randomID()}
	action, prefix := "AuthorizeSecurityGroup", "Permissions"
	if c.kind == "swas" {
		action, prefix = "CreateFirewallRules", "FirewallRules"
		delete(params, "SecurityGroupId")
		params["InstanceId"] = id
	}
	for i, r := range rules {
		key := fmt.Sprintf("%s.%d.", prefix, i+1)
		params[key+"SourceCidrIp"] = r.CIDR
		if c.kind == "swas" {
			params[key+"RuleProtocol"], params[key+"Port"], params[key+"Remark"] = r.Protocol, aliyunPortRange(r.Port), r.Description
		} else {
			params[key+"IpProtocol"], params[key+"PortRange"], params[key+"Description"] = strings.ToLower(r.Protocol), aliyunPortRange(r.Port), r.Description
			params[key+"Policy"], params[key+"Priority"], params[key+"NicType"] = "accept", "1", "intranet"
		}
	}
	return c.call(ctx, action, params, nil)
}

func checkedAliyunRule(set RuleSet, candidate Rule) error {
	if candidate.ID == "" {
		return fmt.Errorf("阿里云未返回规则 ID，已停止修改")
	}
	for _, r := range set.Rules {
		if r.ID == candidate.ID && ledgerKey(r) == ledgerKey(candidate) && r.Action == "ACCEPT" {
			return nil
		}
	}
	return fmt.Errorf("阿里云规则归属或内容已变更，已停止修改")
}

func (c *Aliyun) Delete(ctx context.Context, id, version string, rules []Rule) error {
	set, e := c.checkedRules(ctx, id, version)
	if e != nil {
		return e
	}
	for _, r := range rules {
		if e := checkedAliyunRule(set, r); e != nil {
			return e
		}
	}
	params, action, prefix := map[string]string{"SecurityGroupId": id}, "RevokeSecurityGroup", "SecurityGroupRuleId"
	if c.kind == "swas" {
		params, action, prefix = map[string]string{"InstanceId": id}, "DeleteFirewallRules", "RuleIds"
	}
	for i, r := range rules {
		params[fmt.Sprintf("%s.%d", prefix, i+1)] = r.ID
	}
	if len(rules) == 0 {
		return nil
	}
	return c.call(ctx, action, params, nil)
}

func (c *Aliyun) UpdateDescription(ctx context.Context, id, version string, old Rule, description string) error {
	set, e := c.checkedRules(ctx, id, version)
	if e != nil {
		return e
	}
	if e := checkedAliyunRule(set, old); e != nil {
		return e
	}
	params, action := map[string]string{"SecurityGroupId": id, "SecurityGroupRuleId": old.ID, "Description": description, "Priority": "1", "Policy": "accept"}, "ModifySecurityGroupRule"
	if c.kind == "swas" {
		action = "ModifyFirewallRule"
		params = map[string]string{"InstanceId": id, "RuleId": old.ID, "Remark": description, "RuleProtocol": old.Protocol, "Port": aliyunPortRange(old.Port), "SourceCidrIp": old.CIDR}
	}
	return c.call(ctx, action, params, nil)
}
