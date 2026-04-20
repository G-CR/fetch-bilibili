package bilibili

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
)

func TestSearchCreatorsBuildsRequestAndParsesResponse(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/search/type" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if got := q.Get("search_type"); got != "bili_user" {
			t.Fatalf("expected search_type=bili_user, got %q", got)
		}
		if got := q.Get("keyword"); got != "补档" {
			t.Fatalf("expected keyword 补档, got %q", got)
		}
		if got := q.Get("page"); got != "2" {
			t.Fatalf("expected page=2, got %q", got)
		}
		if got := q.Get("page_size"); got != "20" {
			t.Fatalf("expected page_size=20, got %q", got)
		}

		resp := map[string]any{
			"code": 0,
			"data": map[string]any{
				"result": []map[string]any{
					{
						"mid":   "12345",
						"uname": `<em class="keyword">补档</em>频道`,
						"upic":  "https://img.test/avatar.jpg",
						"fans":  "4321",
						"usign": "简介",
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{RequestTimeout: 2 * time.Second}, nil, WithBaseURL(server.URL))
	hits, err := client.SearchCreators(context.Background(), "补档", 2, 20)
	if err != nil {
		t.Fatalf("SearchCreators error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].UID != "12345" {
		t.Fatalf("expected uid 12345, got %+v", hits[0])
	}
	if hits[0].Name != "补档频道" {
		t.Fatalf("expected cleaned name, got %q", hits[0].Name)
	}
	if hits[0].FollowerCount != 4321 {
		t.Fatalf("expected follower_count 4321, got %d", hits[0].FollowerCount)
	}
	if hits[0].AvatarURL != "https://img.test/avatar.jpg" {
		t.Fatalf("unexpected avatar url: %q", hits[0].AvatarURL)
	}
	if hits[0].ProfileURL != "https://space.bilibili.com/12345" {
		t.Fatalf("unexpected profile url: %q", hits[0].ProfileURL)
	}
}

func TestSearchVideosBuildsRequestAndParsesResponse(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/search/type" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if got := q.Get("search_type"); got != "video" {
			t.Fatalf("expected search_type=video, got %q", got)
		}
		if got := q.Get("keyword"); got != "重传" {
			t.Fatalf("expected keyword 重传, got %q", got)
		}
		if got := q.Get("page"); got != "1" {
			t.Fatalf("expected page=1, got %q", got)
		}
		if got := q.Get("page_size"); got != "10" {
			t.Fatalf("expected page_size=10, got %q", got)
		}

		resp := map[string]any{
			"code": 0,
			"data": map[string]any{
				"result": []map[string]any{
					{
						"mid":         "98765",
						"author":      "阿婆主",
						"bvid":        "BV1xx411c7mD",
						"title":       `<em class="keyword">重传</em>作品`,
						"description": "作品简介",
						"pubdate":     1710000000,
						"play":        "123",
						"favorites":   "45",
						"pic":         "https://img.test/cover.jpg",
						"duration":    "02:10",
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{RequestTimeout: 2 * time.Second}, nil, WithBaseURL(server.URL))
	hits, err := client.SearchVideos(context.Background(), "重传", 1, 10)
	if err != nil {
		t.Fatalf("SearchVideos error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].UID != "98765" || hits[0].CreatorName != "阿婆主" {
		t.Fatalf("unexpected creator fields: %+v", hits[0])
	}
	if hits[0].VideoID != "BV1xx411c7mD" {
		t.Fatalf("unexpected video id: %+v", hits[0])
	}
	if hits[0].Title != "重传作品" {
		t.Fatalf("expected cleaned title, got %q", hits[0].Title)
	}
	if hits[0].ViewCount != 123 || hits[0].FavoriteCount != 45 {
		t.Fatalf("unexpected counters: %+v", hits[0])
	}
	if hits[0].Duration != 130 {
		t.Fatalf("expected duration 130, got %d", hits[0].Duration)
	}
	if !hits[0].PublishTime.Equal(time.Unix(1710000000, 0)) {
		t.Fatalf("unexpected publish time: %s", hits[0].PublishTime)
	}
}

func TestSearchCreatorsReturnsReadableAuthError(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    -101,
			"message": "账号未登录",
		})
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	_, err := client.SearchCreators(context.Background(), "补档", 1, 20)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "搜索作者失败: 账号未登录(-101)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchVideosMarksRiskOnWafError(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    -412,
			"message": "请求被拦截",
		})
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	_, err := client.SearchVideos(context.Background(), "补档", 1, 20)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "搜索视频失败: 请求被拦截(-412)") {
		t.Fatalf("unexpected error: %v", err)
	}
	status := client.RuntimeStatus()
	if status.LastRiskReason == "" {
		t.Fatalf("expected risk reason to be recorded")
	}
}

func TestSearchRelatedVideosDelegatesToVideoSearch(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/search/type" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if got := q.Get("search_type"); got != "video" {
			t.Fatalf("expected search_type=video, got %q", got)
		}
		if got := q.Get("keyword"); got != "演唱会" {
			t.Fatalf("expected keyword 演唱会, got %q", got)
		}
		if got := q.Get("page"); got != "1" {
			t.Fatalf("expected page=1, got %q", got)
		}
		if got := q.Get("page_size"); got != "8" {
			t.Fatalf("expected page_size=8, got %q", got)
		}
		resp := map[string]any{
			"code": 0,
			"data": map[string]any{
				"result": []map[string]any{
					{
						"mid":         "7788",
						"author":      "相似作者",
						"bvid":        "BV1related",
						"title":       "演唱会全场录制",
						"description": "测试",
						"pubdate":     1710000010,
						"play":        "456",
						"favorite":    "78",
						"pic":         "https://img.test/rel.jpg",
						"duration":    "300",
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{RequestTimeout: 2 * time.Second}, nil, WithBaseURL(server.URL))
	hits, err := client.SearchRelatedVideos(context.Background(), "演唱会", 1, 8)
	if err != nil {
		t.Fatalf("SearchRelatedVideos error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].UID != "7788" || hits[0].VideoID != "BV1related" {
		t.Fatalf("unexpected hit: %+v", hits[0])
	}
}
