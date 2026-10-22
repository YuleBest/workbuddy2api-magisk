package tasks

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

var defaultScenes = []catalogItem{
	{ID: "0", Meta: "幻灯片"},
	{ID: "2", Meta: "视频生成"},
	{ID: "4", Meta: "深度研究"},
	{ID: "6", Meta: "文档处理"},
	{ID: "8", Meta: "数据分析"},
	{ID: "10", Meta: "可视化"},
	{ID: "12", Meta: "金融服务"},
	{ID: "14", Meta: "产品管理"},
}

var defaultExperts = []catalogItem{
	{ID: "ContentCreator", Meta: map[string]any{"name": "内容创作专家", "category": "", "expertType": "agent", "version": ""}},
	{ID: "UiDesigner", Meta: map[string]any{"name": "UI设计师", "category": "", "expertType": "agent", "version": ""}},
	{ID: "DataAnalyticsReporter", Meta: map[string]any{"name": "数据分析报告师", "category": "", "expertType": "agent", "version": ""}},
	{ID: "ChinaEcommerceOperationsExpert", Meta: map[string]any{"name": "中国电商运营专家", "category": "", "expertType": "agent", "version": ""}},
	{ID: "DouyinStrategist", Meta: map[string]any{"name": "抖音策略师", "category": "", "expertType": "agent", "version": ""}},
	{ID: "SalesCoach", Meta: map[string]any{"name": "销售教练", "category": "", "expertType": "agent", "version": ""}},
	{ID: "BrandGuardian", Meta: map[string]any{"name": "品牌策略师", "category": "", "expertType": "agent", "version": ""}},
	{ID: "XiaohongshuOperationsExpert", Meta: map[string]any{"name": "小红书运营专家", "category": "", "expertType": "agent", "version": ""}},
}

var defaultTeams = []catalogItem{
	{ID: "CloudOpsTeam", Meta: map[string]any{"name": "", "category": "", "expertType": "team", "version": ""}},
	{ID: "SoftwareCompany", Meta: map[string]any{"name": "", "category": "", "expertType": "team", "version": ""}},
	{ID: "TradingAgentTeam", Meta: map[string]any{"name": "", "category": "", "expertType": "team", "version": ""}},
	{ID: "GPTResearcherTeam", Meta: map[string]any{"name": "", "category": "", "expertType": "team", "version": ""}},
	{ID: "MarketingCampaignTeam", Meta: map[string]any{"name": "", "category": "", "expertType": "team", "version": ""}},
}

var defaultSkills = []catalogItem{
	{ID: "skill_2096525080079265792", Meta: "pptx"},
	{ID: "skill_2096528888507297792", Meta: "xlsx"},
	{ID: "skill_2070033533400236032", Meta: "qqmusic"},
	{ID: "skill_2095322904487550976", Meta: "pdf"},
}

var defaultCases = []catalogItem{
	{ID: "worker-ledger-freedom-dashboard", Meta: map[string]any{
		"title": "打工人小账本", "artifact_type": "other", "categories": []any{""},
		"skills": []any{}, "experts": []any{}, "prompt": "",
	}},
}

var defaultLighthouse = []catalogItem{
	{ID: "ex_2cvvUZQhDyeJ", Meta: map[string]any{
		"name": "腾讯轻量云专家", "category": "02-Engineering",
		"expertType": "agent", "version": "1.0.2",
	}},
}

var defaultTheme = []catalogItem{
	{ID: "theme-tkmw7j", Meta: map[string]any{
		"name": "和平精英激战金秋", "vipLevel": "free",
		"series": "craft", "appearance": "light",
	}},
}

func (s *Service) fetchScenes(ctx context.Context, a Account, out io.Writer) []catalogItem {
	outItems := []catalogItem{}
	st, body := s.doGrowth(ctx, a, "GET", s.ChatBase,
		"/console/as/support/scenes?locale=zh-CN", nil, nil)
	if st == 200 && codeZeroOrNil(codeOf(body)) {
		for _, raw := range array(object(body["data"])["scenes"]) {
			scene := object(raw)
			id := scene["id"]
			if id == nil {
				continue
			}
			outItems = append(outItems, catalogItem{
				ID: stringValue(id), Meta: stringValue(scene["name"]),
			})
		}
	} else if st != 200 {
		fmt.Fprintf(out, "  [warn] 场景清单拉取失败(http=%d)，回落内置表\n", st)
	}
	if len(outItems) == 0 {
		outItems = append(outItems, defaultScenes...)
	}
	return outItems
}

func (s *Service) fetchMarketExperts(
	ctx context.Context,
	a Account,
	teamOnly bool,
	keyword string,
	out io.Writer,
) []catalogItem {
	outItems := []catalogItem{}
	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		body := map[string]any{"page": page, "page_size": 50}
		if keyword != "" {
			body["keyword"] = keyword
		}
		st, response := s.doGrowth(ctx, a, "POST", s.ChatBase,
			"/v2/operation-platform/market/expert/list", body, nil)
		if st != 200 {
			break
		}
		experts := array(object(response["data"])["experts"])
		if len(experts) == 0 {
			break
		}
		for _, raw := range experts {
			expert := object(raw)
			id := stringValue(expert["expert_id"])
			if id == "" {
				id = stringValue(expert["source_id"])
			}
			if id == "" || seen[id] {
				continue
			}
			expertType := fallback(stringValue(expert["expert_type"]), "agent")
			if teamOnly && expertType != "team" {
				continue
			}
			seen[id] = true
			category := ""
			if categories := array(expert["categories"]); len(categories) > 0 {
				category = stringValue(categories[0])
			}
			name := stringValue(expert["display_name_zh"])
			if name == "" {
				name = stringValue(expert["profession_zh"])
			}
			outItems = append(outItems, catalogItem{ID: id, Meta: map[string]any{
				"name": name, "category": category, "expertType": expertType,
				"version":    stringValue(expert["version"]),
				"agent_name": stringValue(expert["agent_name"]),
			}})
		}
		if len(experts) < 50 {
			break
		}
	}

	res, err := s.request(ctx, "GET", "", s.COSExpert, nil, map[string]string{
		"User-Agent": "Mozilla/5.0",
	}, 20*time.Second)
	if err != nil {
		fmt.Fprintf(out, "  [warn] COS 专家清单失败(%v)\n", err)
	} else {
		for _, raw := range array(res.Body["experts"]) {
			expert := object(raw)
			id := stringValue(expert["id"])
			expertType := fallback(stringValue(expert["expertType"]), "agent")
			if id == "" || seen[id] || (teamOnly && expertType != "team") {
				continue
			}
			seen[id] = true
			profession := object(expert["profession"])
			outItems = append(outItems, catalogItem{ID: id, Meta: map[string]any{
				"name": stringValue(profession["zh"]), "category": stringValue(expert["categoryId"]),
				"expertType": expertType, "version": "",
			}})
		}
	}
	if len(outItems) == 0 {
		if teamOnly {
			outItems = append(outItems, defaultTeams...)
		} else {
			outItems = append(outItems, defaultExperts...)
		}
	}
	return outItems
}

func (s *Service) fetchSkills(ctx context.Context, a Account, out io.Writer) []catalogItem {
	outItems := []catalogItem{}
	st, body := s.doGrowth(ctx, a, "POST", s.ChatBase,
		"/v2/operation-platform/market/skill/list",
		map[string]any{"page": 1, "page_size": 8}, nil)
	if st == 200 {
		for _, raw := range array(object(body["data"])["skills"]) {
			skill := object(raw)
			id := stringValue(skill["skill_id"])
			if id != "" {
				outItems = append(outItems, catalogItem{ID: id, Meta: stringValue(skill["name"])})
			}
		}
	} else {
		fmt.Fprintf(out, "  [warn] 技能市场拉取失败(http=%d)，回落内置表\n", st)
	}
	if len(outItems) == 0 {
		outItems = append(outItems, defaultSkills...)
	}
	return outItems
}

func (s *Service) fetchPlaybookCases(ctx context.Context, out io.Writer) []catalogItem {
	outItems := []catalogItem{}
	res, err := s.request(ctx, "GET", "", s.Playbook+"/registry.json", nil, map[string]string{
		"User-Agent": "Mozilla/5.0",
	}, 20*time.Second)
	if err != nil {
		fmt.Fprintf(out, "  [warn] 案例注册表拉取失败(%v)，回落内置表\n", err)
	} else {
		for _, raw := range array(res.Body["cases"]) {
			item := object(raw)
			id := stringValue(item["id"])
			if id == "" {
				continue
			}
			outItems = append(outItems, catalogItem{ID: id, Meta: map[string]any{
				"title": stringValue(item["title"]), "artifact_type": fallback(stringValue(item["artifact_type"]), "other"),
				"categories": item["categories"], "skills": item["skills"], "experts": item["experts"],
				"prompt": stringValue(item["prompt"]), "source": "discover",
			}})
		}
	}
	if len(outItems) == 0 {
		outItems = append(outItems, defaultCases...)
	}
	return outItems
}

func (s *Service) fetchLighthouse(ctx context.Context, a Account, out io.Writer) []catalogItem {
	outItems := []catalogItem{}
	for _, keyword := range []string{"lighthouse", "轻量云"} {
		for _, item := range s.fetchMarketExperts(ctx, a, false, keyword, out) {
			meta := object(item.Meta)
			blob := strings.ToLower(item.ID + stringValue(meta["name"]) + stringValue(meta["agent_name"]))
			if strings.Contains(blob, "lighthouse") || strings.Contains(blob, "轻量") ||
				strings.Contains(blob, "light") || strings.Contains(blob, "yun") {
				outItems = append(outItems, item)
			}
		}
	}
	if len(outItems) == 0 {
		outItems = append(outItems, defaultLighthouse...)
	}
	return outItems
}

func (s *Service) fetchPETheme(ctx context.Context, a Account, out io.Writer) []catalogItem {
	outItems := []catalogItem{}
	st, body := s.doGrowth(ctx, a, "POST", s.BillingBase,
		"/v2/operation-platform/appearance/resources",
		map[string]any{
			"platform": "client", "kind": "theme",
			"version": "2.63.2", "lang": "zh-CN",
		}, nil)
	if st == 200 {
		for _, raw := range array(object(body["data"])["resources"]) {
			resource := object(raw)
			name := stringValue(resource["name"])
			if strings.Contains(name, "和平精英") || strings.Contains(strings.ToLower(name), "pubg") {
				outItems = append(outItems, catalogItem{ID: stringValue(resource["id"]), Meta: map[string]any{
					"name": name, "vipLevel": stringValue(resource["vip_level"]),
					"series": stringValue(resource["series"]), "appearance": stringValue(resource["appearance"]),
				}})
			}
		}
	} else {
		fmt.Fprintf(out, "  [warn] 主题目录拉取失败(http=%d)，回落内置表\n", st)
	}
	if len(outItems) == 0 {
		outItems = append(outItems, defaultTheme...)
	}
	return outItems
}

func dedupSlice(src []catalogItem, offset, need int) []catalogItem {
	seen := map[string]bool{}
	final := []catalogItem{}
	for _, item := range src {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		final = append(final, item)
	}
	if offset >= len(final) {
		return nil
	}
	end := offset + need
	if end > len(final) {
		end = len(final)
	}
	return final[offset:end]
}

func (s *Service) idsFor(
	ctx context.Context,
	a Account,
	kind string,
	need, offset int,
	out io.Writer,
) []catalogItem {
	if need <= 0 {
		return nil
	}
	switch kind {
	case "canvas":
		base := s.Runtime.millis()
		items := make([]catalogItem, 0, need)
		for i := 0; i < need; i++ {
			items = append(items, catalogItem{ID: fmt.Sprintf("wbx-canvas-%d", base+int64(i)), Meta: map[string]any{}})
		}
		return items
	case "template":
		return dedupSlice(s.fetchScenes(ctx, a, out), offset, need)
	case "expert":
		return dedupSlice(s.fetchMarketExperts(ctx, a, false, "", out), offset, need)
	case "team":
		return dedupSlice(s.fetchMarketExperts(ctx, a, true, "", out), offset, need)
	case "skill":
		return dedupSlice(s.fetchSkills(ctx, a, out), offset, need)
	case "playbook":
		return dedupSlice(s.fetchPlaybookCases(ctx, out), offset, need)
	case "lighthouse":
		return dedupSlice(s.fetchLighthouse(ctx, a, out), offset, need)
	case "skin":
		return dedupSlice(s.fetchPETheme(ctx, a, out), offset, need)
	case "chat", "glmchat", "automation", "cat", "richmeow", "buddy5", "library", "buddyfirst",
		"minichat", "schoolseason", "school":
		items := make([]catalogItem, need)
		return items
	}
	return nil
}
