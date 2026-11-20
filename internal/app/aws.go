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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

type AWS struct{ client *ec2.Client }

func newAWS(c Credentials, region string) (Cloud, error) {
	if !regionPattern.MatchString(region) {
		return nil, fmt.Errorf("AWS 地域无效")
	}
	// 显式使用后台保存的凭证和地域，不读取主机的环境凭证或共享配置。
	cfg := aws.Config{
		Region: region,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: c.SecretID, SecretAccessKey: c.SecretKey}, nil
		}),
		HTTPClient:       &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		RetryMaxAttempts: 1,
	}
	return &AWS{client: ec2.NewFromConfig(cfg)}, nil
}

func awsError(err error, action string) error {
	if err == nil {
		return nil
	}
	operation := "ec2:" + action
	detail := "AWS 连接失败；接口：" + operation + "；请检查网络、地域或访问密钥"
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		code := apiError.ErrorCode()
		if len(code) == 0 || len(code) > 100 || strings.IndexFunc(code, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_')
		}) >= 0 {
			code = "UnknownError"
		}
		detail = "AWS 请求失败：" + code + "；接口：" + operation
		lower := strings.ToLower(code)
		if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "accessdenied") || strings.Contains(lower, "forbidden") {
			detail += "；建议检查 IAM 权限：" + operation + "（该接口所需权限）；请同时核对资源范围、权限边界及组织策略的显式拒绝"
		}
	}
	var response interface{ ServiceRequestID() string }
	if errors.As(err, &response) && safeRequestID.MatchString(response.ServiceRequestID()) {
		detail += "；RequestId：" + response.ServiceRequestID()
	}
	return errors.New(detail)
}

func (c *AWS) Regions(ctx context.Context) ([]Region, error) {
	res, e := c.client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{AllRegions: aws.Bool(false)})
	if e != nil {
		return nil, awsError(e, "DescribeRegions")
	}
	out := []Region{}
	for _, r := range res.Regions {
		if id := value(r.RegionName); id != "" && value(r.OptInStatus) != "not-opted-in" {
			out = append(out, Region{ID: id, Name: id})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *AWS) Groups(ctx context.Context) ([]Group, error) {
	out, token := []Group{}, ""
	seen := map[string]bool{}
	for page := 0; page < 1000; page++ {
		input := &ec2.DescribeSecurityGroupsInput{MaxResults: aws.Int32(100)}
		if token != "" {
			input.NextToken = aws.String(token)
		}
		res, e := c.client.DescribeSecurityGroups(ctx, input)
		if e != nil {
			return nil, awsError(e, "DescribeSecurityGroups")
		}
		for _, g := range res.SecurityGroups {
			if value(g.VpcId) != "" && value(g.GroupId) != "" {
				out = append(out, Group{ID: value(g.GroupId), Name: value(g.GroupName), Description: value(g.Description)})
			}
		}
		token = value(res.NextToken)
		if token == "" {
			return out, nil
		}
		if seen[token] {
			return nil, fmt.Errorf("AWS 资源分页未前进")
		}
		seen[token] = true
	}
	return nil, fmt.Errorf("AWS 资源列表过大，请缩小范围")
}

func (c *AWS) Rules(ctx context.Context, id string) (RuleSet, error) {
	out, token, rawRules := RuleSet{Rules: []Rule{}}, "", []string{}
	seen := map[string]bool{}
	for page := 0; ; page++ {
		input := &ec2.DescribeSecurityGroupRulesInput{MaxResults: aws.Int32(1000), Filters: []types.Filter{{Name: aws.String("group-id"), Values: []string{id}}}}
		if token != "" {
			input.NextToken = aws.String(token)
		}
		res, e := c.client.DescribeSecurityGroupRules(ctx, input)
		if e != nil {
			return out, awsError(e, "DescribeSecurityGroupRules")
		}
		for _, r := range res.SecurityGroupRules {
			if value(r.GroupId) != id {
				return out, fmt.Errorf("AWS 返回的规则不属于当前安全组，已停止同步")
			}
			// 标签和分页顺序不影响规则快照，出站及不支持的规则也纳入快照检查。
			sort.Slice(r.Tags, func(i, j int) bool {
				if value(r.Tags[i].Key) == value(r.Tags[j].Key) {
					return value(r.Tags[i].Value) < value(r.Tags[j].Value)
				}
				return value(r.Tags[i].Key) < value(r.Tags[j].Key)
			})
			raw, e := json.Marshal(r)
			if e != nil {
				return out, fmt.Errorf("AWS 规则快照无效")
			}
			rawRules = append(rawRules, string(raw))
			if r.IsEgress == nil || *r.IsEgress {
				continue
			}
			protocol := strings.ToUpper(value(r.IpProtocol))
			if protocol == "6" {
				protocol = "TCP"
			}
			if protocol == "17" {
				protocol = "UDP"
			}
			port := strconv.Itoa(int(value(r.FromPort)))
			if value(r.FromPort) != value(r.ToPort) {
				port += "-" + strconv.Itoa(int(value(r.ToPort)))
			}
			rule := Rule{ID: value(r.SecurityGroupRuleId), Protocol: protocol, Port: port, CIDR: value(r.CidrIpv4), Action: "UNSUPPORTED", Description: value(r.Description)}
			cidr, err := normalizeCIDR(rule.CIDR)
			if err == nil && rule.ID != "" && (protocol == "TCP" || protocol == "UDP") && r.FromPort != nil && r.ToPort != nil && *r.FromPort >= 1 && *r.ToPort >= *r.FromPort && *r.ToPort <= 65535 && value(r.CidrIpv6) == "" && value(r.PrefixListId) == "" && r.ReferencedGroupInfo == nil {
				rule.CIDR, rule.Action = cidr, "ACCEPT"
			}
			out.Rules = append(out.Rules, rule)
		}
		token = value(res.NextToken)
		if token == "" {
			break
		}
		if seen[token] || page >= 999 {
			return out, fmt.Errorf("AWS 规则分页异常或列表过大，已停止同步")
		}
		seen[token] = true
	}
	sort.Strings(rawRules)
	sum := sha256.Sum256([]byte(strings.Join(rawRules, "\n")))
	out.Version = hex.EncodeToString(sum[:])
	return out, nil
}

func (c *AWS) checkedRules(ctx context.Context, id, version string) (RuleSet, error) {
	if version == "" {
		return RuleSet{}, fmt.Errorf("缺少 AWS 规则快照，已停止修改")
	}
	set, e := c.Rules(ctx, id)
	if e != nil {
		return set, e
	}
	if set.Version != version {
		return set, fmt.Errorf("AWS 规则已变更，请重试")
	}
	return set, nil
}

func (c *AWS) Add(ctx context.Context, id, version string, rules []Rule) error {
	set, e := c.checkedRules(ctx, id, version)
	if e != nil {
		return e
	}
	permissions := []types.IpPermission{}
	for _, r := range rules {
		for _, existing := range set.Rules {
			if ruleKey(r) == ruleKey(existing) {
				return fmt.Errorf("AWS 已有同值规则，已停止新增，避免接管用户原有规则")
			}
		}
		parts := strings.Split(r.Port, "-")
		from, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("AWS 端口无效")
		}
		to := from
		if len(parts) == 2 {
			to, err = strconv.Atoi(parts[1])
		}
		cidr, cidrErr := normalizeCIDR(r.CIDR)
		if err != nil || len(parts) > 2 || from < 1 || to < from || to > 65535 || (r.Protocol != "TCP" && r.Protocol != "UDP") || cidrErr != nil || r.Action != "ACCEPT" {
			return fmt.Errorf("AWS 仅支持有效的 TCP / UDP IPv4 入站放行规则")
		}
		permissions = append(permissions, types.IpPermission{IpProtocol: aws.String(strings.ToLower(r.Protocol)), FromPort: aws.Int32(int32(from)), ToPort: aws.Int32(int32(to)), IpRanges: []types.IpRange{{CidrIp: aws.String(cidr), Description: aws.String(r.Description)}}})
	}
	if len(permissions) == 0 {
		return nil
	}
	_, e = c.client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{GroupId: aws.String(id), IpPermissions: permissions})
	return awsError(e, "AuthorizeSecurityGroupIngress")
}

func checkedAWSRule(set RuleSet, candidate Rule) error {
	if candidate.ID != "" {
		for _, r := range set.Rules {
			if r.ID == candidate.ID && ledgerKey(r) == ledgerKey(candidate) && r.Action == "ACCEPT" {
				return nil
			}
		}
	}
	return fmt.Errorf("AWS 规则 ID、归属或内容已变更，已停止修改")
}

func (c *AWS) Delete(ctx context.Context, id, version string, rules []Rule) error {
	set, e := c.checkedRules(ctx, id, version)
	if e != nil {
		return e
	}
	ids := []string{}
	for _, r := range rules {
		if e := checkedAWSRule(set, r); e != nil {
			return e
		}
		ids = append(ids, r.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	// 稳定规则 ID 防止同值重建规则被误删；业务层在撤销后再次读取核验。
	_, e = c.client.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{GroupId: aws.String(id), SecurityGroupRuleIds: ids})
	return awsError(e, "RevokeSecurityGroupIngress")
}

func (c *AWS) UpdateDescription(ctx context.Context, id, version string, old Rule, description string) error {
	set, e := c.checkedRules(ctx, id, version)
	if e != nil {
		return e
	}
	if e := checkedAWSRule(set, old); e != nil {
		return e
	}
	// 仅调用入站备注接口，不重写规则的匹配条件。
	_, e = c.client.UpdateSecurityGroupRuleDescriptionsIngress(ctx, &ec2.UpdateSecurityGroupRuleDescriptionsIngressInput{GroupId: aws.String(id), SecurityGroupRuleDescriptions: []types.SecurityGroupRuleDescription{{SecurityGroupRuleId: aws.String(old.ID), Description: aws.String(description)}}})
	return awsError(e, "UpdateSecurityGroupRuleDescriptionsIngress")
}

func awsDescription(note, marker string) string {
	// AWS 云端备注不接受中文；本地保留原文，仅过滤发送到云端的可读部分。
	note = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(" ._-:/()#,@[]+=;{}!$*", r) {
			return r
		}
		return ' '
	}, note)
	return cloudDescription(note, marker)
}
