package app

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	lighthouse "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/lighthouse/v20200324"
)

type Region struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func cloudRegions(ctx context.Context, c Credentials, kind string) ([]Region, error) {
	if !validResourceType(kind) {
		return nil, fmt.Errorf("不支持的目标类型")
	}
	p := profile.NewClientProfile()
	p.HttpProfile.ReqTimeout = 20
	credential := common.NewCredential(c.SecretID, c.SecretKey)
	out := []Region{}
	if resourceType(kind) == "lighthouse" {
		client, e := lighthouse.NewClient(credential, "ap-guangzhou", p)
		if e != nil {
			return nil, e
		}
		res, e := client.DescribeRegionsWithContext(ctx, lighthouse.NewDescribeRegionsRequest())
		if e != nil {
			return nil, cloudError(e, "lighthouse:DescribeRegions")
		}
		for _, r := range res.Response.RegionSet {
			if value(r.RegionState) == "AVAILABLE" {
				out = append(out, Region{value(r.Region), value(r.RegionName)})
			}
		}
	} else {
		client, e := cvm.NewClient(credential, "ap-guangzhou", p)
		if e != nil {
			return nil, e
		}
		res, e := client.DescribeRegionsWithContext(ctx, cvm.NewDescribeRegionsRequest())
		if e != nil {
			return nil, cloudError(e, "cvm:DescribeRegions")
		}
		for _, r := range res.Response.RegionSet {
			if value(r.RegionState) == "AVAILABLE" {
				out = append(out, Region{value(r.Region), value(r.RegionName)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (a *App) regions(w http.ResponseWriter, r *http.Request) {
	account, e := a.store.account(r.PathValue("id"))
	if e != nil {
		fail(w, 404, "云账号不存在")
		return
	}
	c, e := a.store.credentials(account.ID)
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	kind := r.URL.Query().Get("resourceType")
	if kind == "" {
		kind = account.ResourceType
	}
	out, e := cloudRegions(r.Context(), c, kind)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	jsonResponse(w, 200, out)
}
func (a *App) formRegions(w http.ResponseWriter, r *http.Request) {
	var input struct {
		accountInput
		ID string `json:"id"`
	}
	if !decode(w, r, &input) {
		return
	}
	c, e := a.resolveCredentials(input.ID, input.accountInput)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	out, e := cloudRegions(r.Context(), c, input.ResourceType)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	jsonResponse(w, 200, out)
}
