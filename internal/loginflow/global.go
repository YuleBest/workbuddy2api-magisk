package loginflow

import (
	"context"
	"fmt"
	"io"

	"workbuddy2api/internal/region"
)

func (r *Runner) runGlobal(ctx context.Context, token, uid string, in io.Reader) {
	client := &region.Client{HTTPClient: r.HTTPClient, BaseURL: r.GlobalBaseURL}
	err := client.ActivateRegion(ctx, token, uid)
	needsRegion := region.IsRegionRequired(err)
	switch {
	case err == nil:
		fmt.Fprintln(r.Out, "注册激活: 成功")
	case needsRegion:
		fmt.Fprintln(r.Out, "注册激活: 需完善注册地区")
	default:
		fmt.Fprintf(r.Out, "注册激活: %v\n", err)
	}

	if needsRegion {
		countries, fetchErr := client.FetchCountries(ctx, true)
		if fetchErr != nil {
			fmt.Fprintf(r.Out, "  拉取地区列表失败: %v\n", fetchErr)
		} else {
			detected, _ := client.DetectRegion(ctx, token)
			var picked *region.Country
			for i := range countries {
				if detected.IOS2 != "" && countries[i].IOS2 == detected.IOS2 {
					picked = &countries[i]
					break
				}
			}
			if picked != nil {
				fmt.Fprintf(r.Out, "  检测到当前地区: %s %s，跳过选项单\n", picked.IOS2, picked.EnName)
			} else {
				picked = region.InteractiveMenu(
					countries,
					detected.IOS2,
					"国际版账号需完善注册地区",
					in,
					r.Out,
				)
			}
			if picked == nil {
				fmt.Fprintln(r.Out, "  未选择地区，跳过完善（后续可重跑登录完善）")
			} else if completeErr := client.CompleteFlow(ctx, token, uid, picked, false); completeErr != nil {
				fmt.Fprintf(r.Out, "  地区完善失败: %v\n", completeErr)
			} else {
				fmt.Fprintf(r.Out, "  地区已完善: %s %s（register 重新验证通过）\n", picked.IOS2, picked.EnName)
			}
		}
	}

	already, trialErr := client.Trial(ctx, token)
	switch {
	case trialErr != nil:
		fmt.Fprintf(r.Out, "国际版 trial: %v（不阻断登录）\n", trialErr)
	case already:
		fmt.Fprintln(r.Out, "国际版 trial: 已激活，可以开始对话（已领取过）")
	default:
		fmt.Fprintln(r.Out, "国际版 trial: 已激活，可以开始对话")
	}
}
