package app

import "fmt"

func providerID(provider string) string {
	if provider == "" {
		return "tencent"
	}
	return provider
}

func validProviderResource(provider, kind string) bool {
	switch providerID(provider) {
	case "tencent":
		return resourceType(kind) == "security-group" || kind == "lighthouse"
	case "aliyun":
		return resourceType(kind) == "security-group" || kind == "swas"
	}
	return false
}

func resourceLabel(kind string) string {
	switch resourceType(kind) {
	case "lighthouse":
		return "Lighthouse 实例"
	case "swas":
		return "轻量应用服务器实例"
	default:
		return "安全组"
	}
}

func (a *App) accountCloud(account Account, c Credentials, region, kind string) (Cloud, error) {
	if !validProviderResource(account.Provider, kind) {
		return nil, fmt.Errorf("云厂商不支持此目标类型")
	}
	if providerID(account.Provider) == "aliyun" {
		return a.aliyun(c, region, resourceType(kind))
	}
	return a.cloudFor(c, region, kind)
}
