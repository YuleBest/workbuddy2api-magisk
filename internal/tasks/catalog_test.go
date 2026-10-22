package tasks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestCatalogSources(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/console/as/support/scenes":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"scenes": []any{map[string]any{"id": 2, "name": "视频生成"}}},
			})
		case "/chat/v2/operation-platform/market/expert/list":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["keyword"] == "lighthouse" {
				writeJSONResponse(w, 200, map[string]any{
					"code": 0,
					"data": map[string]any{"experts": []any{map[string]any{
						"expert_id": "ex_lighthouse", "display_name_zh": "腾讯轻量云专家",
						"expert_type": "agent",
					}}},
				})
				return
			}
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"experts": []any{
					map[string]any{
						"expert_id": "ex_agent", "display_name_zh": "专家",
						"categories": []any{"cat"}, "expert_type": "agent",
					},
					map[string]any{
						"expert_id": "ex_team", "display_name_zh": "团队",
						"expert_type": "team",
					},
				}},
			})
		case "/cos/expert.json":
			writeJSONResponse(w, 200, map[string]any{
				"experts": []any{map[string]any{
					"id": "ex_cos_team", "expertType": "team",
					"profession": map[string]any{"zh": "COS 团队"}, "categoryId": "cat",
				}},
			})
		case "/chat/v2/operation-platform/market/skill/list":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"skills": []any{map[string]any{
					"skill_id": "skill_1", "name": "PPT",
				}}},
			})
		case "/playbook/registry.json":
			writeJSONResponse(w, 200, map[string]any{
				"cases": []any{map[string]any{
					"id": "case_1", "title": "案例", "artifact_type": "other",
					"categories": []any{"cat"}, "skills": []any{}, "experts": []any{},
					"prompt": "提示",
				}},
			})
		case "/billing/v2/operation-platform/appearance/resources":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"resources": []any{map[string]any{
					"id": "theme_pubg", "name": "和平精英激战金秋",
					"vip_level": "free", "series": "craft", "appearance": "light",
				}}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, nil)
	account := testAccount()
	ctx := context.Background()

	if items := service.fetchScenes(ctx, account, io.Discard); len(items) != 1 || items[0].ID != "2" {
		t.Errorf("scenes=%+v", items)
	}
	experts := service.fetchMarketExperts(ctx, account, false, "", io.Discard)
	if !hasCatalogID(experts, "ex_agent") || !hasCatalogID(experts, "ex_cos_team") {
		t.Errorf("experts=%+v", experts)
	}
	teams := service.fetchMarketExperts(ctx, account, true, "", io.Discard)
	if hasCatalogID(teams, "ex_agent") || !hasCatalogID(teams, "ex_team") ||
		!hasCatalogID(teams, "ex_cos_team") {
		t.Errorf("teams=%+v", teams)
	}
	if items := service.fetchSkills(ctx, account, io.Discard); len(items) != 1 || items[0].ID != "skill_1" {
		t.Errorf("skills=%+v", items)
	}
	if items := service.fetchPlaybookCases(ctx, io.Discard); len(items) != 1 || items[0].ID != "case_1" {
		t.Errorf("cases=%+v", items)
	}
	if items := service.fetchPETheme(ctx, account, io.Discard); len(items) != 1 || items[0].ID != "theme_pubg" {
		t.Errorf("themes=%+v", items)
	}
	if items := service.fetchLighthouse(ctx, account, io.Discard); !hasCatalogID(items, "ex_lighthouse") {
		t.Errorf("lighthouse=%+v", items)
	}
}

func hasCatalogID(items []catalogItem, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
