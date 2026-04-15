package bilibili

import (
	"context"
	"encoding/json"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/live"
)

type stubSystemEventPublisher struct {
	events []live.Event
}

func (s *stubSystemEventPublisher) Publish(evt live.Event) {
	s.events = append(s.events, evt)
}

func mustFindLastSystemChangedEvent(t *testing.T, events []live.Event) live.Event {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == "system.changed" {
			return events[i]
		}
	}
	t.Fatalf("expected system.changed event, got %+v", events)
	return live.Event{}
}

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func TestListVideosAndCheckAvailable(t *testing.T) {
	imgKey := "7cd084941338484aae1ad9425b84077c"
	subKey := "4932caff0ff746eab6f01bf08b70ac45"
	mixinKey := calcMixinKey(imgKey, subKey)

	navResp := navResp{
		Code: 0,
		Data: struct {
			IsLogin bool   `json:"isLogin"`
			Mid     int64  `json:"mid"`
			Uname   string `json:"uname"`
			WbiImg  struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			} `json:"wbi_img"`
		}{
			WbiImg: struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			}{
				ImgURL: "https://i0.hdslb.com/bfs/wbi/" + imgKey + ".png",
				SubURL: "https://i0.hdslb.com/bfs/wbi/" + subKey + ".png",
			},
		},
	}

	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/nav":
			_ = json.NewEncoder(w).Encode(navResp)
		case "/x/space/wbi/arc/search":
			q := r.URL.Query()
			if q.Get("w_rid") == "" || q.Get("wts") == "" {
				t.Fatalf("missing w_rid or wts")
			}
			params := map[string]string{
				"mid":   q.Get("mid"),
				"pn":    q.Get("pn"),
				"ps":    q.Get("ps"),
				"order": q.Get("order"),
			}
			wts, _ := strconv.ParseInt(q.Get("wts"), 10, 64)
			want := signParams(params, mixinKey, wts)
			if want != r.URL.RawQuery {
				t.Fatalf("signed query mismatch")
			}

			resp := arcSearchResp{Code: 0}
			resp.Data.List.VList = []struct {
				AID         int64  `json:"aid"`
				BVID        string `json:"bvid"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Created     int64  `json:"created"`
				Length      string `json:"length"`
				Pic         string `json:"pic"`
				Play        int64  `json:"play"`
				Favorite    int64  `json:"favorite"`
				Favorites   int64  `json:"favorites"`
			}{
				{
					AID:         1,
					BVID:        "BV1xx",
					Title:       "t",
					Description: "d",
					Created:     1700000000,
					Length:      "01:02",
					Pic:         "pic",
					Play:        100,
					Favorite:    3,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := config.BilibiliConfig{UserAgent: "test", RequestTimeout: 2 * time.Second}
	client := New(cfg, nil, WithBaseURL(server.URL))

	list, err := client.ListVideos(context.Background(), "123")
	if err != nil {
		t.Fatalf("ListVideos error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 video")
	}

	ok, err := client.CheckAvailable(context.Background(), "BV1xx")
	if err != nil {
		t.Fatalf("CheckAvailable error: %v", err)
	}
	if !ok {
		t.Fatalf("expected available")
	}
}

func TestListVideosUsesConfiguredFetchPageSize(t *testing.T) {
	imgKey := "7cd084941338484aae1ad9425b84077c"
	subKey := "4932caff0ff746eab6f01bf08b70ac45"

	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/nav":
			_ = json.NewEncoder(w).Encode(navResp{
				Code: 0,
				Data: struct {
					IsLogin bool   `json:"isLogin"`
					Mid     int64  `json:"mid"`
					Uname   string `json:"uname"`
					WbiImg  struct {
						ImgURL string `json:"img_url"`
						SubURL string `json:"sub_url"`
					} `json:"wbi_img"`
				}{
					WbiImg: struct {
						ImgURL string `json:"img_url"`
						SubURL string `json:"sub_url"`
					}{
						ImgURL: "https://i0.hdslb.com/bfs/wbi/" + imgKey + ".png",
						SubURL: "https://i0.hdslb.com/bfs/wbi/" + subKey + ".png",
					},
				},
			})
		case "/x/space/wbi/arc/search":
			if got := r.URL.Query().Get("ps"); got != "7" {
				t.Fatalf("expected ps=7, got %s", got)
			}
			_ = json.NewEncoder(w).Encode(arcSearchResp{Code: 0})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{FetchPageSize: 7}, nil, WithBaseURL(server.URL))
	if _, err := client.ListVideos(context.Background(), "123"); err != nil {
		t.Fatalf("ListVideos error: %v", err)
	}
}

func TestCheckAvailableUnavailable(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(viewResp{Code: -404, Message: "not found"})
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	ok, err := client.CheckAvailable(context.Background(), "BV1xx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected unavailable")
	}
}

func TestCheckAvailableForbidden(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(viewResp{Code: -403, Message: "forbidden"})
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.CheckAvailable(context.Background(), "BV1xx"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestListVideosInvalidID(t *testing.T) {
	client := New(config.BilibiliConfig{}, nil)
	if _, err := client.ListVideos(context.Background(), ""); err != ErrInvalidID {
		t.Fatalf("expected ErrInvalidID")
	}
}

func TestCheckAvailableInvalidID(t *testing.T) {
	client := New(config.BilibiliConfig{}, nil)
	if _, err := client.CheckAvailable(context.Background(), ""); err != ErrInvalidID {
		t.Fatalf("expected ErrInvalidID")
	}
}

func TestNormalizeAID(t *testing.T) {
	if _, ok := normalizeAID("av123"); !ok {
		t.Fatalf("expected aid ok")
	}
	if _, ok := normalizeAID("123"); !ok {
		t.Fatalf("expected aid ok")
	}
	if _, ok := normalizeAID("avx"); ok {
		t.Fatalf("expected invalid aid")
	}
}

func TestCheckAvailableUsesAID(t *testing.T) {
	called := false
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		if q.Get("aid") != "123" {
			t.Fatalf("expected aid query")
		}
		called = true
		_ = json.NewEncoder(w).Encode(viewResp{Code: 0})
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	ok, err := client.CheckAvailable(context.Background(), "av123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || !called {
		t.Fatalf("expected available")
	}
}

func TestParseDuration(t *testing.T) {
	if got := parseDuration("01:02"); got != 62 {
		t.Fatalf("expected 62, got %d", got)
	}
	if got := parseDuration("1:02:03"); got != 3723 {
		t.Fatalf("expected 3723, got %d", got)
	}
	if got := parseDuration("bad"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestPickFavorite(t *testing.T) {
	if got := pickFavorite(5, 9); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
	if got := pickFavorite(0, 9); got != 9 {
		t.Fatalf("expected 9, got %d", got)
	}
}

func TestNormalizeBVID(t *testing.T) {
	if _, ok := normalizeBVID("BV1xx"); !ok {
		t.Fatalf("expected bvid ok")
	}
	if _, ok := normalizeBVID("av123"); ok {
		t.Fatalf("expected bvid false")
	}
}

func TestTrimFileKey(t *testing.T) {
	if got := trimFileKey("https://i0.hdslb.com/bfs/wbi/abc.png"); got != "abc" {
		t.Fatalf("unexpected key: %s", got)
	}
	if got := trimFileKey("abc.png"); got != "abc" {
		t.Fatalf("unexpected key: %s", got)
	}
	if got := trimFileKey("abc"); got != "abc" {
		t.Fatalf("unexpected key: %s", got)
	}
}

func TestCheckAvailableRiskAndUnknown(t *testing.T) {
	var calls int32
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			_ = json.NewEncoder(w).Encode(viewResp{Code: -412, Message: "risk"})
			return
		}
		_ = json.NewEncoder(w).Encode(viewResp{Code: -1, Message: "unknown"})
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.CheckAvailable(context.Background(), "BV1xx"); err == nil {
		t.Fatalf("expected error for risk code")
	}
	if _, err := client.CheckAvailable(context.Background(), "BV1xx"); err == nil {
		t.Fatalf("expected error for unknown code")
	}
}

func TestWbiKeyCache(t *testing.T) {
	var navCalls int32
	imgKey := "7cd084941338484aae1ad9425b84077c"
	subKey := "4932caff0ff746eab6f01bf08b70ac45"
	mixinKey := calcMixinKey(imgKey, subKey)

	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/nav":
			atomic.AddInt32(&navCalls, 1)
			_ = json.NewEncoder(w).Encode(navResp{
				Code: 0,
				Data: struct {
					IsLogin bool   `json:"isLogin"`
					Mid     int64  `json:"mid"`
					Uname   string `json:"uname"`
					WbiImg  struct {
						ImgURL string `json:"img_url"`
						SubURL string `json:"sub_url"`
					} `json:"wbi_img"`
				}{
					WbiImg: struct {
						ImgURL string `json:"img_url"`
						SubURL string `json:"sub_url"`
					}{
						ImgURL: "https://i0.hdslb.com/bfs/wbi/" + imgKey + ".png",
						SubURL: "https://i0.hdslb.com/bfs/wbi/" + subKey + ".png",
					},
				},
			})
		case "/x/space/wbi/arc/search":
			q := r.URL.Query()
			if q.Get("w_rid") == "" || q.Get("wts") == "" {
				t.Fatalf("missing w_rid or wts")
			}
			params := map[string]string{
				"mid":   q.Get("mid"),
				"pn":    q.Get("pn"),
				"ps":    q.Get("ps"),
				"order": q.Get("order"),
			}
			wts, _ := strconv.ParseInt(q.Get("wts"), 10, 64)
			want := signParams(params, mixinKey, wts)
			if want != r.URL.RawQuery {
				t.Fatalf("signed query mismatch")
			}
			_ = json.NewEncoder(w).Encode(arcSearchResp{Code: 0})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fixedNow := time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)
	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL), WithNow(func() time.Time { return fixedNow }))

	if _, err := client.ListVideos(context.Background(), "1"); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if _, err := client.ListVideos(context.Background(), "1"); err != nil {
		t.Fatalf("list error: %v", err)
	}
	if atomic.LoadInt32(&navCalls) != 1 {
		t.Fatalf("expected nav to be called once")
	}
}

func TestCookieHeader(t *testing.T) {
	cookie := "SESSDATA=abc"
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != cookie {
			t.Fatalf("expected cookie header")
		}
		_ = json.NewEncoder(w).Encode(viewResp{Code: 0})
	}))
	defer server.Close()

	cfg := config.BilibiliConfig{Cookie: cookie}
	client := New(cfg, nil, WithBaseURL(server.URL))
	if _, err := client.CheckAvailable(context.Background(), "BV1xx"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSESSDATAHeader(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "SESSDATA=token" {
			t.Fatalf("expected sessdata cookie header")
		}
		_ = json.NewEncoder(w).Encode(viewResp{Code: 0})
	}))
	defer server.Close()

	cfg := config.BilibiliConfig{SESSDATA: "token"}
	client := New(cfg, nil, WithBaseURL(server.URL))
	if _, err := client.CheckAvailable(context.Background(), "BV1xx"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReloadAuthWithInlineCookieReturnsNoChange(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "SESSDATA=inline-token" {
			t.Fatalf("expected inline cookie header")
		}
		_ = json.NewEncoder(w).Encode(viewResp{Code: 0})
	}))
	defer server.Close()

	cfg := config.BilibiliConfig{Cookie: "SESSDATA=inline-token"}
	client := New(cfg, nil, WithBaseURL(server.URL))

	updated, err := client.ReloadAuth()
	if err != nil {
		t.Fatalf("expected reload no-op without error: %v", err)
	}
	if updated {
		t.Fatalf("expected no config reload for inline cookie")
	}

	if _, err := client.CheckAvailable(context.Background(), "BV1xx"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckAuth(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := navResp{Code: 0}
		resp.Data.IsLogin = true
		resp.Data.Mid = 123
		resp.Data.Uname = "user"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	info, err := client.CheckAuth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.IsLogin || info.Mid != 123 {
		t.Fatalf("unexpected auth info")
	}
}

func TestResolveUIDByNameWithCache(t *testing.T) {
	var calls int32
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/search/type" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt32(&calls, 1)
		resp := userSearchResp{Code: 0}
		resp.Data.Result = []struct {
			Mid   int64  `json:"mid"`
			Uname string `json:"uname"`
		}{
			{Mid: 100, Uname: "Alice"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.BilibiliConfig{ResolveNameCacheTTL: time.Hour}
	client := New(cfg, nil, WithBaseURL(server.URL))

	uid, name, err := client.ResolveUID(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if uid != "100" || name != "Alice" {
		t.Fatalf("unexpected resolve result")
	}

	uid, name, err = client.ResolveUID(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if uid != "100" || name != "Alice" {
		t.Fatalf("unexpected cached result")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestResolveUIDNumeric(t *testing.T) {
	var calls int32
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	uid, name, err := client.ResolveUID(context.Background(), "123")
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if uid != "123" || name != "" {
		t.Fatalf("unexpected resolve result")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("expected no search call")
	}
}

func TestResolveNameByUID(t *testing.T) {
	imgKey := "7cd084941338484aae1ad9425b84077c"
	subKey := "4932caff0ff746eab6f01bf08b70ac45"
	mixinKey := calcMixinKey(imgKey, subKey)

	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/nav":
			_ = json.NewEncoder(w).Encode(navResp{
				Code: 0,
				Data: struct {
					IsLogin bool   `json:"isLogin"`
					Mid     int64  `json:"mid"`
					Uname   string `json:"uname"`
					WbiImg  struct {
						ImgURL string `json:"img_url"`
						SubURL string `json:"sub_url"`
					} `json:"wbi_img"`
				}{
					WbiImg: struct {
						ImgURL string `json:"img_url"`
						SubURL string `json:"sub_url"`
					}{
						ImgURL: "https://i0.hdslb.com/bfs/wbi/" + imgKey + ".png",
						SubURL: "https://i0.hdslb.com/bfs/wbi/" + subKey + ".png",
					},
				},
			})
		case "/x/space/wbi/acc/info":
			q := r.URL.Query()
			params := map[string]string{
				"mid": q.Get("mid"),
			}
			wts, _ := strconv.ParseInt(q.Get("wts"), 10, 64)
			want := signParams(params, mixinKey, wts)
			if want != r.URL.RawQuery {
				t.Fatalf("signed query mismatch")
			}
			resp := struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Data    struct {
					Name string `json:"name"`
				} `json:"data"`
			}{Code: 0}
			resp.Data.Name = "Alice"
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))

	type nameResolver interface {
		ResolveName(context.Context, string) (string, error)
	}
	resolver, ok := any(client).(nameResolver)
	if !ok {
		t.Fatalf("client should implement ResolveName")
	}

	name, err := resolver.ResolveName(context.Background(), "123")
	if err != nil {
		t.Fatalf("resolve name error: %v", err)
	}
	if name != "Alice" {
		t.Fatalf("unexpected name: %s", name)
	}
}

func TestResolveUIDNoResult(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := userSearchResp{Code: 0}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, _, err := client.ResolveUID(context.Background(), "missing"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownload(t *testing.T) {
	content := []byte("video")
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			resp.Data.CID = 123
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/player/playurl":
			resp := playURLResp{Code: 0}
			resp.Data.Durl = []struct {
				URL       string   `json:"url"`
				BackupURL []string `json:"backup_url"`
				Size      int64    `json:"size"`
			}{
				{URL: server.URL + "/video.mp4", Size: int64(len(content))},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/video.mp4":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "v1.mp4")

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	n, err := client.Download(context.Background(), "BV1xx", dst)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	if n != int64(len(content)) {
		t.Fatalf("unexpected size")
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != string(content) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadDashMergesAudioAndVideo(t *testing.T) {
	ffmpegDir := t.TempDir()
	ffmpegPath := filepath.Join(ffmpegDir, "ffmpeg")
	ffmpegScript := `#!/bin/sh
video=""
audio=""
expect_input=0
out=""
for arg in "$@"; do
  if [ "$expect_input" = "1" ]; then
    if [ -z "$video" ]; then
      video="$arg"
    else
      audio="$arg"
    fi
    expect_input=0
    continue
  fi
  if [ "$arg" = "-i" ]; then
    expect_input=1
    continue
  fi
  out="$arg"
done
cat "$video" "$audio" > "$out"
`
	if err := os.WriteFile(ffmpegPath, []byte(ffmpegScript), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	t.Setenv("PATH", ffmpegDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	videoContent := []byte("video-")
	audioContent := []byte("audio")
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			resp.Data.CID = 123
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/player/playurl":
			resp := struct {
				Code int `json:"code"`
				Data struct {
					Dash struct {
						Video []struct {
							BaseURL string `json:"baseUrl"`
						} `json:"video"`
						Audio []struct {
							BaseURL string `json:"baseUrl"`
						} `json:"audio"`
					} `json:"dash"`
				} `json:"data"`
			}{Code: 0}
			resp.Data.Dash.Video = []struct {
				BaseURL string `json:"baseUrl"`
			}{
				{BaseURL: server.URL + "/video.m4s"},
			}
			resp.Data.Dash.Audio = []struct {
				BaseURL string `json:"baseUrl"`
			}{
				{BaseURL: server.URL + "/audio.m4s"},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/video.m4s":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(videoContent)
		case "/audio.m4s":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(audioContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	dst := filepath.Join(t.TempDir(), "dash.mp4")
	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))

	n, err := client.Download(context.Background(), "BV1xx", dst)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	if n != int64(len(videoContent)+len(audioContent)) {
		t.Fatalf("unexpected size: %d", n)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read merged file: %v", err)
	}
	if string(data) != "video-audio" {
		t.Fatalf("unexpected merged content: %q", string(data))
	}
}

func TestDownloadPlayURLFailure(t *testing.T) {
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			resp.Data.CID = 123
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/player/playurl":
			resp := playURLResp{Code: -1, Message: "bad"}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.Download(context.Background(), "BV1xx", filepath.Join(t.TempDir(), "v1.mp4")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadNoDurl(t *testing.T) {
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			resp.Data.CID = 123
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/player/playurl":
			resp := playURLResp{Code: 0}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.Download(context.Background(), "BV1xx", filepath.Join(t.TempDir(), "v1.mp4")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadInvalidID(t *testing.T) {
	client := New(config.BilibiliConfig{}, nil)
	if _, err := client.Download(context.Background(), "bad", filepath.Join(t.TempDir(), "v1.mp4")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadBackupURL(t *testing.T) {
	content := []byte("backup")
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			resp.Data.CID = 1
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/player/playurl":
			resp := playURLResp{Code: 0}
			resp.Data.Durl = []struct {
				URL       string   `json:"url"`
				BackupURL []string `json:"backup_url"`
				Size      int64    `json:"size"`
			}{
				{URL: server.URL + "/primary.mp4", BackupURL: []string{server.URL + "/backup.mp4"}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/primary.mp4":
			w.WriteHeader(http.StatusInternalServerError)
		case "/backup.mp4":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	dst := filepath.Join(t.TempDir(), "v1.mp4")
	if _, err := client.Download(context.Background(), "BV1xx", dst); err != nil {
		t.Fatalf("download error: %v", err)
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != string(content) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadEmptyContentReturnsSize(t *testing.T) {
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			resp.Data.CID = 1
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/player/playurl":
			resp := playURLResp{Code: 0}
			resp.Data.Durl = []struct {
				URL       string   `json:"url"`
				BackupURL []string `json:"backup_url"`
				Size      int64    `json:"size"`
			}{
				{URL: server.URL + "/empty.mp4", Size: 10},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/empty.mp4":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	dst := filepath.Join(t.TempDir(), "empty.mp4")
	n, err := client.Download(context.Background(), "BV1xx", dst)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	if n != 10 {
		t.Fatalf("expected size 10, got %d", n)
	}
}

func TestDownloadMissingCID(t *testing.T) {
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x/web-interface/view" {
			resp := viewResp{Code: 0}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.Download(context.Background(), "BV1xx", filepath.Join(t.TempDir(), "v1.mp4")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadPrimaryFailNoBackup(t *testing.T) {
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			resp.Data.CID = 1
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/player/playurl":
			resp := playURLResp{Code: 0}
			resp.Data.Durl = []struct {
				URL       string   `json:"url"`
				BackupURL []string `json:"backup_url"`
				Size      int64    `json:"size"`
			}{
				{URL: server.URL + "/fail.mp4"},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/fail.mp4":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.Download(context.Background(), "BV1xx", filepath.Join(t.TempDir(), "v1.mp4")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRiskBackoffReset(t *testing.T) {
	cfg := config.BilibiliConfig{
		RiskBackoffBase:   10 * time.Millisecond,
		RiskBackoffMax:    20 * time.Millisecond,
		RiskBackoffJitter: 0,
	}
	client := New(cfg, nil)
	client.markRisk()
	if client.riskBackoff != 10*time.Millisecond {
		t.Fatalf("unexpected backoff")
	}
	client.markRisk()
	if client.riskBackoff != 20*time.Millisecond {
		t.Fatalf("unexpected backoff")
	}
	client.resetRisk()
	if client.riskBackoff != 0 || !client.riskUntil.IsZero() {
		t.Fatalf("expected reset")
	}
}

func TestWaitRiskCanceled(t *testing.T) {
	client := New(config.BilibiliConfig{}, nil)
	client.riskUntil = time.Now().Add(50 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.waitRisk(ctx); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCheckAuthHTTPRiskStatus(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.CheckAuth(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWaitRiskExpired(t *testing.T) {
	client := New(config.BilibiliConfig{}, nil)
	client.riskUntil = time.Now().Add(-time.Second)
	if err := client.waitRisk(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRiskBackoffJitter(t *testing.T) {
	cfg := config.BilibiliConfig{
		RiskBackoffBase:   10 * time.Millisecond,
		RiskBackoffMax:    10 * time.Millisecond,
		RiskBackoffJitter: 0.5,
	}
	client := New(cfg, nil)
	client.riskRand = rand.New(rand.NewSource(1))
	client.markRisk()
	if client.riskBackoff == 0 {
		t.Fatalf("expected backoff")
	}
}

func TestListVideosRiskCode(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x/web-interface/nav" {
			resp := navResp{Code: -412, Message: "risk"}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.ListVideos(context.Background(), "1"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadForbidden(t *testing.T) {
	var server *httptest.Server
	server = newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/view":
			resp := viewResp{Code: 0}
			resp.Data.CID = 1
			_ = json.NewEncoder(w).Encode(resp)
		case "/x/player/playurl":
			resp := playURLResp{Code: 0}
			resp.Data.Durl = []struct {
				URL       string   `json:"url"`
				BackupURL []string `json:"backup_url"`
				Size      int64    `json:"size"`
			}{
				{URL: server.URL + "/forbid.mp4"},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/forbid.mp4":
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{}, nil, WithBaseURL(server.URL))
	if _, err := client.Download(context.Background(), "BV1xx", filepath.Join(t.TempDir(), "v1.mp4")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRuntimeStatusTracksCookieSourceAndReload(t *testing.T) {
	client := New(config.BilibiliConfig{Cookie: "SESSDATA=inline-token"}, nil)
	status := client.RuntimeStatus()
	if !status.CookieConfigured {
		t.Fatalf("expected cookie configured")
	}
	if status.CookieSource != "config" {
		t.Fatalf("unexpected cookie source: %s", status.CookieSource)
	}

	updated, err := client.ReloadAuth()
	if err != nil {
		t.Fatalf("ReloadAuth error: %v", err)
	}
	if updated {
		t.Fatalf("expected no inline reload")
	}

	status = client.RuntimeStatus()
	if status.LastReloadResult != "no_change" {
		t.Fatalf("unexpected reload result: %s", status.LastReloadResult)
	}
	if status.LastReloadAt.IsZero() {
		t.Fatalf("expected last reload at")
	}
}

func TestReloadAuthPublishesSystemChangedWhenStateChanges(t *testing.T) {
	client := New(config.BilibiliConfig{Cookie: "SESSDATA=inline-token"}, nil)
	publisher := &stubSystemEventPublisher{}
	client.SetPublisher(publisher)

	if _, err := client.ReloadAuth(); err != nil {
		t.Fatalf("ReloadAuth error: %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(publisher.events))
	}

	evt := mustFindLastSystemChangedEvent(t, publisher.events)
	payload, ok := evt.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", evt.Payload)
	}
	cookie, ok := payload["cookie"].(map[string]any)
	if !ok {
		t.Fatalf("expected cookie payload, got %T", payload["cookie"])
	}
	if got := cookie["last_reload_result"]; got != "no_change" {
		t.Fatalf("expected last_reload_result=no_change, got %v", got)
	}

	if _, err := client.ReloadAuth(); err != nil {
		t.Fatalf("ReloadAuth second error: %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("expected no duplicate event for same reload state, got %d", len(publisher.events))
	}
}

func TestRuntimeStatusTracksCheckAndRisk(t *testing.T) {
	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := navResp{Code: -412, Message: "risk"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(config.BilibiliConfig{RiskBackoffBase: 10 * time.Millisecond, RiskBackoffJitter: 0}, nil, WithBaseURL(server.URL))
	if _, err := client.CheckAuth(context.Background()); err == nil {
		t.Fatalf("expected error")
	}

	status := client.RuntimeStatus()
	if status.LastCheckResult != "error" {
		t.Fatalf("unexpected check result: %s", status.LastCheckResult)
	}
	if status.LastCheckAt.IsZero() {
		t.Fatalf("expected last check at")
	}
	if status.LastError == "" {
		t.Fatalf("expected last error")
	}
	if status.RiskUntil.IsZero() {
		t.Fatalf("expected risk until")
	}
	if status.LastRiskAt.IsZero() {
		t.Fatalf("expected last risk at")
	}
	if status.LastRiskReason == "" {
		t.Fatalf("expected last risk reason")
	}
}

func TestCheckAuthPublishesSystemChangedForValidInvalidAndError(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.Handler
		wantStatus  string
		wantIsLogin bool
		wantMid     int64
	}{
		{
			name: "valid",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := navResp{Code: 0}
				resp.Data.IsLogin = true
				resp.Data.Mid = 12
				resp.Data.Uname = "tester"
				_ = json.NewEncoder(w).Encode(resp)
			}),
			wantStatus:  "valid",
			wantIsLogin: true,
			wantMid:     12,
		},
		{
			name: "invalid",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := navResp{Code: 0}
				resp.Data.IsLogin = false
				_ = json.NewEncoder(w).Encode(resp)
			}),
			wantStatus:  "invalid",
			wantIsLogin: false,
			wantMid:     0,
		},
		{
			name: "error",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := navResp{Code: -101, Message: "not login"}
				_ = json.NewEncoder(w).Encode(resp)
			}),
			wantStatus:  "error",
			wantIsLogin: false,
			wantMid:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newIPv4TestServer(t, tt.handler)
			defer server.Close()

			client := New(config.BilibiliConfig{Cookie: "SESSDATA=inline-token", RiskBackoffBase: 10 * time.Millisecond, RiskBackoffJitter: 0}, nil, WithBaseURL(server.URL))
			publisher := &stubSystemEventPublisher{}
			client.SetPublisher(publisher)

			_, _ = client.CheckAuth(context.Background())
			if len(publisher.events) == 0 {
				t.Fatalf("expected system.changed event")
			}

			evt := mustFindLastSystemChangedEvent(t, publisher.events)
			payload, ok := evt.Payload.(map[string]any)
			if !ok {
				t.Fatalf("expected map payload, got %T", evt.Payload)
			}
			cookie, ok := payload["cookie"].(map[string]any)
			if !ok {
				t.Fatalf("expected cookie payload, got %T", payload["cookie"])
			}
			if got := cookie["status"]; got != tt.wantStatus {
				t.Fatalf("expected cookie status=%s, got %v", tt.wantStatus, got)
			}
			if got := cookie["is_login"]; got != tt.wantIsLogin {
				t.Fatalf("expected is_login=%v, got %v", tt.wantIsLogin, got)
			}
			if got := cookie["mid"]; got != tt.wantMid {
				t.Fatalf("expected mid=%d, got %v", tt.wantMid, got)
			}
		})
	}
}

func TestRiskStatePublishesSystemChangedOnHitAndRecovery(t *testing.T) {
	client := New(config.BilibiliConfig{RiskBackoffBase: 10 * time.Millisecond, RiskBackoffJitter: 0}, nil)
	publisher := &stubSystemEventPublisher{}
	client.SetPublisher(publisher)

	client.markRiskReason("hit")
	if len(publisher.events) != 1 {
		t.Fatalf("expected 1 event after risk hit, got %d", len(publisher.events))
	}

	hit := mustFindLastSystemChangedEvent(t, publisher.events)
	hitPayload, ok := hit.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", hit.Payload)
	}
	risk, ok := hitPayload["risk"].(map[string]any)
	if !ok {
		t.Fatalf("expected risk payload, got %T", hitPayload["risk"])
	}
	if got := risk["active"]; got != true {
		t.Fatalf("expected risk active=true, got %v", got)
	}

	client.resetRisk()
	if len(publisher.events) != 2 {
		t.Fatalf("expected 2 events after recovery, got %d", len(publisher.events))
	}

	reset := mustFindLastSystemChangedEvent(t, publisher.events)
	resetPayload, ok := reset.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", reset.Payload)
	}
	risk, ok = resetPayload["risk"].(map[string]any)
	if !ok {
		t.Fatalf("expected risk payload, got %T", resetPayload["risk"])
	}
	if got := risk["active"]; got != false {
		t.Fatalf("expected risk active=false, got %v", got)
	}
}
