package app

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

type Account struct {
	ResourceType string `json:"resourceType"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	Region       string `json:"region"`
	MaskedKey    string `json:"maskedKey"`
	Status       string `json:"status"`
	TestedAt     string `json:"testedAt"`
	TargetCount  int    `json:"targetCount"`
}
type Credentials struct {
	SecretID  string `json:"secretId"`
	SecretKey string `json:"secretKey"`
}
type Port struct {
	Protocol string `json:"protocol"`
	Value    string `json:"value"`
}
type FixedIP struct {
	CIDR string `json:"cidr"`
	Name string `json:"name,omitempty"`
}
type Target struct {
	Name         string    `json:"name"`
	ResourceType string    `json:"resourceType"`
	ID           string    `json:"id"`
	AccountID    string    `json:"accountId"`
	Region       string    `json:"region"`
	GroupID      string    `json:"groupId"`
	GroupName    string    `json:"groupName"`
	Ports        []Port    `json:"ports"`
	IPMode       string    `json:"ipMode"`
	FixedCIDR    string    `json:"fixedCidr"`
	FixedIPs     []FixedIP `json:"fixedIps"`
	AutoSync     bool      `json:"autoSync"`
	Note         string    `json:"note"`
	CurrentCIDR  string    `json:"currentCidr"`
	CurrentCIDRs []string  `json:"currentCidrs"`
	Status       string    `json:"status"`
	LastSync     string    `json:"lastSync"`
	Error        string    `json:"error"`
}

func (t Target) fixedIPs() []FixedIP {
	if t.FixedIPs == nil && t.IPMode == "fixed" && t.FixedCIDR != "" {
		return []FixedIP{{CIDR: t.FixedCIDR}}
	}
	return t.FixedIPs
}

func sameCIDRs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, cidr := range a {
		seen[cidr] = true
	}
	for _, cidr := range b {
		if !seen[cidr] {
			return false
		}
	}
	return true
}

func (t Target) desiredCIDRs(ip string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(cidr string) {
		if cidr != "" && !seen[cidr] {
			seen[cidr] = true
			out = append(out, cidr)
		}
	}
	if t.IPMode == "dynamic" && ip != "" {
		add(ip + "/32")
	}
	for _, source := range t.fixedIPs() {
		add(source.CIDR)
	}
	return out
}

type Settings struct {
	DetectOnManualSync *bool    `json:"detectOnManualSync"`
	DetectOnSchedule   *bool    `json:"detectOnSchedule"`
	Timezone           string   `json:"timezone"`
	Enabled            bool     `json:"enabled"`
	Interval           int      `json:"interval"`
	Cron               string   `json:"cron"`
	Sources            []string `json:"sources"`
}
type IPState struct {
	IP        string `json:"ip"`
	Source    string `json:"source"`
	CheckedAt string `json:"checkedAt"`
	Error     string `json:"error"`
}
type LogEntry struct {
	ID      int64    `json:"id"`
	Time    string   `json:"time"`
	Kind    string   `json:"kind"`
	Trigger string   `json:"trigger"`
	Object  string   `json:"object"`
	Detail  string   `json:"detail"`
	Success bool     `json:"success"`
	Changes []string `json:"changes"`
}
type Group struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var groupPattern = regexp.MustCompile(`^sg-[a-zA-Z0-9]+$`)
var instancePattern = regexp.MustCompile(`^lhins-[a-zA-Z0-9]+$`)
var regionPattern = regexp.MustCompile(`^[a-z]+-[a-z]+(?:-[a-z]+)?$`)

func now() string { return time.Now().UTC().Format(time.RFC3339) }
func normalizeCIDR(s string) (string, error) {
	if a, e := netip.ParseAddr(strings.TrimSpace(s)); e == nil && a.Is4() {
		if a.IsUnspecified() {
			return "", fmt.Errorf("不能使用未指定的 IPv4 地址")
		}
		return a.String() + "/32", nil
	}
	p, e := netip.ParsePrefix(strings.TrimSpace(s))
	if e != nil || !p.Addr().Is4() {
		return "", fmt.Errorf("请输入有效的 IPv4 地址或 CIDR 网段")
	}
	p = p.Masked()
	// 腾讯云会将 0.0.0.0/n 映射为全网开放，拒绝容易被意外扩大的网段。
	if p.Addr().IsUnspecified() && p.Bits() != 0 {
		return "", fmt.Errorf("腾讯云会将此网段扩大为全网，不能使用 0.0.0.0/n")
	}
	return p.String(), nil
}
func validateTarget(t *Target) error {
	t.Name = strings.TrimSpace(t.Name)
	if len([]rune(t.Name)) > 60 {
		return fmt.Errorf("目标名称最多 60 个字符")
	}
	if t.ResourceType == "" {
		t.ResourceType = "security-group"
	}
	validResource := t.ResourceType == "security-group" && groupPattern.MatchString(t.GroupID) || t.ResourceType == "lighthouse" && instancePattern.MatchString(t.GroupID)
	if t.AccountID == "" || !regionPattern.MatchString(t.Region) || !validResource {
		return fmt.Errorf("请选择云账号、有效地域与安全组或 Lighthouse 实例")
	}
	if len(t.Note) > 300 || len(t.GroupName) > 200 {
		return fmt.Errorf("备注或安全组名称过长")
	}
	if t.IPMode != "dynamic" && t.IPMode != "fixed" {
		return fmt.Errorf("请选择公网 IP 或固定网段")
	}
	inputs := t.fixedIPs()
	if len(inputs) > 20 {
		return fmt.Errorf("自定义 IP 最多 20 项")
	}
	t.FixedIPs = []FixedIP{}
	seenIPs := map[string]bool{}
	for _, input := range inputs {
		cidr, e := normalizeCIDR(input.CIDR)
		if e != nil {
			return e
		}
		input.Name = strings.TrimSpace(input.Name)
		if len(input.Name) > 90 {
			return fmt.Errorf("IP 名称过长")
		}
		if !seenIPs[cidr] {
			seenIPs[cidr] = true
			t.FixedIPs = append(t.FixedIPs, FixedIP{CIDR: cidr, Name: input.Name})
		}
	}
	if t.IPMode == "fixed" && len(t.FixedIPs) == 0 {
		return fmt.Errorf("请至少选择一个 IP 来源")
	}
	t.FixedCIDR = ""
	if t.IPMode == "fixed" {
		t.FixedCIDR = t.FixedIPs[0].CIDR
	}
	if len(t.Ports) == 0 || len(t.Ports) > 50 {
		return fmt.Errorf("请配置 1 至 50 个端口或端口范围")
	}
	count := len(t.FixedIPs)
	if t.IPMode == "dynamic" {
		count++
	}
	if count*len(t.Ports) > 100 {
		return fmt.Errorf("IP 来源与端口组合最多 100 条，请减少 IP 或端口")
	}
	seen := map[string]bool{}
	for i, p := range t.Ports {
		if p.Protocol != "TCP" && p.Protocol != "UDP" {
			return fmt.Errorf("仅支持 TCP 和 UDP")
		}
		parts := strings.Split(strings.TrimSpace(p.Value), "-")
		if len(parts) > 2 {
			return fmt.Errorf("无效端口：%s", p.Value)
		}
		values := []int{}
		for _, v := range parts {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 65535 {
				return fmt.Errorf("端口范围应为 1–65535")
			}
			values = append(values, n)
		}
		if len(values) == 2 && values[0] > values[1] {
			return fmt.Errorf("端口范围起点不能大于终点")
		}
		p.Value = strconv.Itoa(values[0])
		if len(values) == 2 && values[0] != values[1] {
			p.Value += "-" + strconv.Itoa(values[1])
		}
		t.Ports[i] = p
		k := p.Protocol + ":" + p.Value
		if seen[k] {
			return fmt.Errorf("存在重复端口")
		}
		seen[k] = true
	}
	return nil
}
func (s Settings) schedule() (cron.Schedule, error) {
	zone := s.Timezone
	if zone == "" {
		zone = "Asia/Shanghai"
	}
	if _, e := time.LoadLocation(zone); e != nil {
		return nil, fmt.Errorf("请输入有效的 IANA 时区")
	}
	if s.Cron != "" {
		if len(strings.Fields(s.Cron)) != 5 {
			return nil, fmt.Errorf("Cron 必须为五段表达式")
		}
		return cron.ParseStandard("CRON_TZ=" + zone + " " + s.Cron)
	}
	if s.Interval != 1 && s.Interval != 5 && s.Interval != 15 {
		return nil, fmt.Errorf("检查间隔仅支持 1、5、15 分钟")
	}
	return cron.Every(time.Duration(s.Interval) * time.Minute), nil
}
