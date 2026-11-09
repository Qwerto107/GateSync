package app

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

type Rule struct {
	Index                                     int64
	Protocol, Port, CIDR, Action, Description string
}
type RuleSet struct {
	Version string
	Rules   []Rule
}

// 云厂商差异集中在此接口，业务层不依赖腾讯云的数据结构。
type Cloud interface {
	Groups(context.Context) ([]Group, error)
	Rules(context.Context, string) (RuleSet, error)
	Add(context.Context, string, string, []Rule) error
	Delete(context.Context, string, string, []Rule) error
	UpdateDescription(context.Context, string, string, Rule, string) error
}
type Tencent struct{ client *vpc.Client }

func newTencent(c Credentials, region string) (Cloud, error) {
	p := profile.NewClientProfile()
	p.HttpProfile.ReqTimeout = 20
	client, e := vpc.NewClient(common.NewCredential(c.SecretID, c.SecretKey), region, p)
	return &Tencent{client}, e
}
func value[T any](p *T) T {
	if p == nil {
		var z T
		return z
	}
	return *p
}
func (c *Tencent) Groups(ctx context.Context) ([]Group, error) {
	groups := []Group{}
	for offset := uint64(0); ; offset += 100 {
		r := vpc.NewDescribeSecurityGroupsRequest()
		r.Offset = common.StringPtr(strconv.FormatUint(offset, 10))
		r.Limit = common.StringPtr("100")
		res, e := c.client.DescribeSecurityGroupsWithContext(ctx, r)
		if e != nil {
			return nil, cloudError(e, "vpc:DescribeSecurityGroups")
		}
		for _, g := range res.Response.SecurityGroupSet {
			groups = append(groups, Group{value(g.SecurityGroupId), value(g.SecurityGroupName), value(g.SecurityGroupDesc)})
		}
		if offset+100 >= value(res.Response.TotalCount) {
			break
		}
	}
	return groups, nil
}
func (c *Tencent) Rules(ctx context.Context, id string) (RuleSet, error) {
	r := vpc.NewDescribeSecurityGroupPoliciesRequest()
	r.SecurityGroupId = common.StringPtr(id)
	res, e := c.client.DescribeSecurityGroupPoliciesWithContext(ctx, r)
	if e != nil {
		return RuleSet{}, cloudError(e, "vpc:DescribeSecurityGroupPolicies")
	}
	p := res.Response.SecurityGroupPolicySet
	if p == nil {
		return RuleSet{}, fmt.Errorf("云端未返回安全组规则")
	}
	out := RuleSet{Version: value(p.Version)}
	for _, p := range p.Ingress {
		out.Rules = append(out.Rules, Rule{value(p.PolicyIndex), value(p.Protocol), value(p.Port), value(p.CidrBlock), value(p.Action), value(p.PolicyDescription)})
	}
	return out, nil
}
func (c *Tencent) Add(ctx context.Context, id, version string, rules []Rule) error {
	r := vpc.NewCreateSecurityGroupPoliciesRequest()
	r.SecurityGroupId = common.StringPtr(id)
	p := &vpc.SecurityGroupPolicySet{Version: common.StringPtr(version)}
	for _, v := range rules {
		p.Ingress = append(p.Ingress, &vpc.SecurityGroupPolicy{Protocol: common.StringPtr(v.Protocol), Port: common.StringPtr(v.Port), CidrBlock: common.StringPtr(v.CIDR), Action: common.StringPtr("ACCEPT"), PolicyDescription: common.StringPtr(v.Description)})
	}
	r.SecurityGroupPolicySet = p
	_, e := c.client.CreateSecurityGroupPoliciesWithContext(ctx, r)
	return cloudError(e, "vpc:CreateSecurityGroupPolicies")
}
func (c *Tencent) Delete(ctx context.Context, id, version string, rules []Rule) error {
	if version == "" {
		return fmt.Errorf("缺少规则版本号，已停止删除")
	}
	r := vpc.NewDeleteSecurityGroupPoliciesRequest()
	r.SecurityGroupId = common.StringPtr(id)
	p := &vpc.SecurityGroupPolicySet{Version: common.StringPtr(version)}
	// 使用刚读取的版本和索引删除；并发修改会导致失败，不会误删索引移位后的其他规则。
	for _, v := range rules {
		p.Ingress = append(p.Ingress, &vpc.SecurityGroupPolicy{PolicyIndex: common.Int64Ptr(v.Index)})
	}
	r.SecurityGroupPolicySet = p
	_, e := c.client.DeleteSecurityGroupPoliciesWithContext(ctx, r)
	return cloudError(e, "vpc:DeleteSecurityGroupPolicies")
}
func (c *Tencent) UpdateDescription(ctx context.Context, id, version string, old Rule, description string) error {
	if version == "" {
		return fmt.Errorf("缺少规则版本，已停止更新备注")
	}
	r := vpc.NewReplaceSecurityGroupPolicyRequest()
	r.SecurityGroupId = common.StringPtr(id)
	// 使用原有匹配字段及最新索引、版本，仅替换已确认归属规则的描述。
	r.SecurityGroupPolicySet = &vpc.SecurityGroupPolicySet{Version: common.StringPtr(version), Ingress: []*vpc.SecurityGroupPolicy{{PolicyIndex: common.Int64Ptr(old.Index), Protocol: common.StringPtr(old.Protocol), Port: common.StringPtr(old.Port), CidrBlock: common.StringPtr(old.CIDR), Action: common.StringPtr(old.Action), PolicyDescription: common.StringPtr(description)}}}
	_, e := c.client.ReplaceSecurityGroupPolicyWithContext(ctx, r)
	return cloudError(e, "vpc:ReplaceSecurityGroupPolicy")
}
