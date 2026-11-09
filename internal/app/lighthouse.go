package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	lighthouse "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/lighthouse/v20200324"
)

type Lighthouse struct{ client *lighthouse.Client }

func resourceType(kind string) string {
	if kind == "" {
		return "security-group"
	}
	return kind
}
func validResourceType(kind string) bool {
	return resourceType(kind) == "security-group" || kind == "lighthouse"
}
func (a *App) cloudFor(c Credentials, region, kind string) (Cloud, error) {
	switch resourceType(kind) {
	case "security-group":
		return a.cloud(c, region)
	case "lighthouse":
		return a.lighthouse(c, region)
	default:
		return nil, fmt.Errorf("不支持的目标类型")
	}
}
func newLighthouse(c Credentials, region string) (Cloud, error) {
	p := profile.NewClientProfile()
	p.HttpProfile.ReqTimeout = 20
	client, e := lighthouse.NewClient(common.NewCredential(c.SecretID, c.SecretKey), region, p)
	return &Lighthouse{client}, e
}
func (c *Lighthouse) Groups(ctx context.Context) ([]Group, error) {
	out := []Group{}
	for offset := int64(0); ; offset += 100 {
		r := lighthouse.NewDescribeInstancesRequest()
		r.Offset = common.Int64Ptr(offset)
		r.Limit = common.Int64Ptr(100)
		res, e := c.client.DescribeInstancesWithContext(ctx, r)
		if e != nil {
			return nil, cloudError(e, "lighthouse:DescribeInstances")
		}
		for _, instance := range res.Response.InstanceSet {
			out = append(out, Group{ID: value(instance.InstanceId), Name: value(instance.InstanceName), Description: strings.Join(stringValues(instance.PublicAddresses), ", ")})
		}
		if offset+100 >= value(res.Response.TotalCount) {
			break
		}
	}
	return out, nil
}
func stringValues(values []*string) []string {
	out := []string{}
	for _, v := range values {
		out = append(out, value(v))
	}
	return out
}
func (c *Lighthouse) Rules(ctx context.Context, id string) (RuleSet, error) {
	out := RuleSet{Rules: []Rule{}}
	for offset := int64(0); ; offset += 100 {
		r := lighthouse.NewDescribeFirewallRulesRequest()
		r.InstanceId = common.StringPtr(id)
		r.Offset = common.Int64Ptr(offset)
		r.Limit = common.Int64Ptr(100)
		res, e := c.client.DescribeFirewallRulesWithContext(ctx, r)
		if e != nil {
			return out, cloudError(e, "lighthouse:DescribeFirewallRules")
		}
		if res.Response.FirewallVersion == nil {
			return out, fmt.Errorf("Lighthouse 未返回防火墙版本")
		}
		version := strconv.FormatUint(*res.Response.FirewallVersion, 10)
		if offset > 0 && out.Version != version {
			return out, fmt.Errorf("分页读取期间防火墙已变更，请重试")
		}
		out.Version = version
		for _, r := range res.Response.FirewallRuleSet {
			out.Rules = append(out.Rules, Rule{Protocol: value(r.Protocol), Port: value(r.Port), CIDR: value(r.CidrBlock), Action: value(r.Action), Description: value(r.FirewallRuleDescription)})
		}
		if offset+100 >= value(res.Response.TotalCount) {
			break
		}
	}
	return out, nil
}
func lighthouseRules(rules []Rule) []*lighthouse.FirewallRule {
	out := []*lighthouse.FirewallRule{}
	for _, r := range rules {
		out = append(out, &lighthouse.FirewallRule{Protocol: common.StringPtr(r.Protocol), Port: common.StringPtr(r.Port), CidrBlock: common.StringPtr(r.CIDR), Action: common.StringPtr("ACCEPT"), FirewallRuleDescription: common.StringPtr(r.Description)})
	}
	return out
}
func firewallVersion(version string) (uint64, error) {
	v, e := strconv.ParseUint(version, 10, 64)
	if e != nil || version == "" {
		return 0, fmt.Errorf("防火墙版本无效，已停止变更")
	}
	return v, nil
}
func (c *Lighthouse) Add(ctx context.Context, id, version string, rules []Rule) error {
	v, e := firewallVersion(version)
	if e != nil {
		return e
	}
	r := lighthouse.NewCreateFirewallRulesRequest()
	r.InstanceId = common.StringPtr(id)
	r.FirewallVersion = common.Uint64Ptr(v)
	r.FirewallRules = lighthouseRules(rules)
	_, e = c.client.CreateFirewallRulesWithContext(ctx, r)
	return cloudError(e, "lighthouse:CreateFirewallRules")
}
func (c *Lighthouse) Delete(ctx context.Context, id, version string, rules []Rule) error {
	v, e := firewallVersion(version)
	if e != nil {
		return e
	}
	// Lighthouse 按规则值删除；再次检查版本与唯一性，避免同值的非归属规则被连带删除。
	latest, e := c.Rules(ctx, id)
	if e != nil {
		return e
	}
	if latest.Version != version {
		return fmt.Errorf("防火墙版本已变更，请重试")
	}
	for _, candidate := range rules {
		count := 0
		owned := false
		for _, r := range latest.Rules {
			if ruleKey(r) == ruleKey(candidate) && strings.EqualFold(r.Action, "ACCEPT") {
				count++
				owned = r.Description == candidate.Description
			}
		}
		if count != 1 || !owned {
			return fmt.Errorf("无法唯一确认待撤销防火墙规则，已停止删除")
		}
	}
	r := lighthouse.NewDeleteFirewallRulesRequest()
	r.InstanceId = common.StringPtr(id)
	r.FirewallVersion = common.Uint64Ptr(v)
	r.FirewallRules = lighthouseRules(rules)
	_, e = c.client.DeleteFirewallRulesWithContext(ctx, r)
	return cloudError(e, "lighthouse:DeleteFirewallRules")
}

func (c *Lighthouse) UpdateDescription(ctx context.Context, id, version string, old Rule, description string) error {
	v, e := firewallVersion(version)
	if e != nil {
		return e
	}
	latest, e := c.Rules(ctx, id)
	if e != nil {
		return e
	}
	if latest.Version != version {
		return fmt.Errorf("防火墙版本已变更，请重试")
	}
	count, owned := 0, false
	for _, r := range latest.Rules {
		if ruleKey(r) == ruleKey(old) && strings.EqualFold(r.Action, "ACCEPT") {
			count++
			owned = r.Description == old.Description
		}
	}
	if count != 1 || !owned {
		return fmt.Errorf("无法唯一确认防火墙规则，已停止更新备注")
	}
	updated := old
	updated.Description = description
	r := lighthouse.NewModifyFirewallRuleDescriptionRequest()
	r.InstanceId = common.StringPtr(id)
	r.FirewallVersion = common.Uint64Ptr(v)
	r.FirewallRule = lighthouseRules([]Rule{updated})[0]
	_, e = c.client.ModifyFirewallRuleDescriptionWithContext(ctx, r)
	return cloudError(e, "lighthouse:ModifyFirewallRuleDescription")
}
