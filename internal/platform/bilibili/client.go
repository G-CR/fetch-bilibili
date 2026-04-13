package bilibili

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"fetch-bilibili/internal/config"
)

var ErrInvalidID = errors.New("无效的 ID")

const (
	defaultBaseURL  = "https://api.bilibili.com"
	defaultReferer  = "https://www.bilibili.com"
	defaultPageSize = 30
)

// Client is a Bilibili API client.
type Client struct {
	httpClient *http.Client
	userAgent  string
	referer    string
	baseURL    string
	logger     *log.Logger
	now        func() time.Time

	cookieMu     sync.RWMutex
	cookie       string
	cookieFile   string
	sessdataFile string

	mu  sync.Mutex
	wbi wbiKeys

	nameCacheMu  sync.RWMutex
	nameCache    map[string]nameCacheEntry
	nameCacheTTL time.Duration

	riskMu      sync.Mutex
	riskUntil   time.Time
	riskBackoff time.Duration
	riskBase    time.Duration
	riskMax     time.Duration
	riskJitter  float64
	riskRand    *rand.Rand
	statusMu    sync.RWMutex
	status      RuntimeStatus
}

type Option func(*Client)

type RuntimeStatus struct {
	CookieConfigured bool
	CookieSource     string
	LastReloadAt     time.Time
	LastReloadResult string
	LastCheckAt      time.Time
	LastCheckResult  string
	LastError        string
	RiskUntil        time.Time
	RiskBackoff      time.Duration
	LastRiskAt       time.Time
	LastRiskReason   string
}

func WithBaseURL(base string) Option {
	return func(c *Client) {
		if base != "" {
			c.baseURL = strings.TrimRight(base, "/")
		}
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(c *Client) {
		if now != nil {
			c.now = now
		}
	}
}

func New(cfg config.BilibiliConfig, logger *log.Logger, opts ...Option) *Client {
	if logger == nil {
		logger = log.Default()
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	cacheTTL := cfg.ResolveNameCacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 24 * time.Hour
	}
	riskBase := cfg.RiskBackoffBase
	if riskBase <= 0 {
		riskBase = 2 * time.Second
	}
	riskMax := cfg.RiskBackoffMax
	if riskMax <= 0 {
		riskMax = 30 * time.Second
	}
	riskJitter := cfg.RiskBackoffJitter
	if riskJitter < 0 {
		riskJitter = 0
	}
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = "fetch-bilibili/1.0"
	}

	cookie := buildCookie(cfg.Cookie, cfg.SESSDATA)
	cookieConfigured := cookie != "" || strings.TrimSpace(cfg.CookieFile) != "" || strings.TrimSpace(cfg.SESSDATAFile) != ""

	c := &Client{
		httpClient:   &http.Client{Timeout: timeout},
		userAgent:    userAgent,
		referer:      defaultReferer,
		baseURL:      defaultBaseURL,
		cookie:       cookie,
		cookieFile:   strings.TrimSpace(cfg.CookieFile),
		sessdataFile: strings.TrimSpace(cfg.SESSDATAFile),
		logger:       logger,
		now:          time.Now,
		nameCache:    make(map[string]nameCacheEntry),
		nameCacheTTL: cacheTTL,
		riskBase:     riskBase,
		riskMax:      riskMax,
		riskJitter:   riskJitter,
		riskRand:     rand.New(rand.NewSource(time.Now().UnixNano())),
		status: RuntimeStatus{
			CookieConfigured: cookieConfigured,
			CookieSource:     detectCookieSource(cfg),
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) ListVideos(ctx context.Context, uid string) ([]VideoMeta, error) {
	if uid == "" {
		return nil, ErrInvalidID
	}

	params := map[string]string{
		"mid":   uid,
		"pn":    "1",
		"ps":    strconv.Itoa(defaultPageSize),
		"order": "pubdate",
	}

	query, err := c.signWbiParams(ctx, params)
	if err != nil {
		return nil, err
	}

	var resp arcSearchResp
	if err := c.doGetJSON(ctx, "/x/space/wbi/arc/search", query, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		if resp.Code == -403 || resp.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/space/wbi/arc/search 返回风控码 %d", resp.Code))
		}
		return nil, fmt.Errorf("拉取视频列表失败: %s(%d)", resp.Message, resp.Code)
	}

	metas := make([]VideoMeta, 0, len(resp.Data.List.VList))
	for _, item := range resp.Data.List.VList {
		videoID := item.BVID
		if videoID == "" && item.AID != 0 {
			videoID = "av" + strconv.FormatInt(item.AID, 10)
		}
		if videoID == "" {
			continue
		}

		meta := VideoMeta{
			VideoID:       videoID,
			Title:         item.Title,
			Description:   item.Description,
			PublishTime:   time.Unix(item.Created, 0),
			Duration:      parseDuration(item.Length),
			CoverURL:      item.Pic,
			ViewCount:     item.Play,
			FavoriteCount: pickFavorite(item.Favorite, item.Favorites),
		}
		metas = append(metas, meta)
	}

	return metas, nil
}

func (c *Client) CheckAvailable(ctx context.Context, videoID string) (bool, error) {
	if videoID == "" {
		return false, ErrInvalidID
	}

	values := url.Values{}
	if bvid, ok := normalizeBVID(videoID); ok {
		values.Set("bvid", bvid)
	} else if aid, ok := normalizeAID(videoID); ok {
		values.Set("aid", aid)
	} else {
		return false, ErrInvalidID
	}

	var resp viewResp
	if err := c.doGetJSON(ctx, "/x/web-interface/view", values.Encode(), &resp); err != nil {
		return false, err
	}
	if resp.Code == 0 {
		return true, nil
	}
	if resp.Code == -404 || resp.Code == 62002 {
		return false, nil
	}
	if resp.Code == -403 {
		c.markRiskReason(fmt.Sprintf("/x/web-interface/view 返回风控码 %d", resp.Code))
		return false, fmt.Errorf("访问受限: %s(%d)", resp.Message, resp.Code)
	}
	if resp.Code == -412 {
		c.markRiskReason(fmt.Sprintf("/x/web-interface/view 返回风控码 %d", resp.Code))
		return false, fmt.Errorf("触发风控: %s(%d)", resp.Message, resp.Code)
	}
	return false, fmt.Errorf("检查失败: %s(%d)", resp.Message, resp.Code)
}

func (c *Client) CheckAuth(ctx context.Context) (AuthInfo, error) {
	checkedAt := c.now()
	var resp navResp
	if err := c.doGetJSON(ctx, "/x/web-interface/nav", "", &resp); err != nil {
		c.recordCheck(checkedAt, "error", err.Error())
		return AuthInfo{}, err
	}
	if resp.Code != 0 {
		if resp.Code == -403 || resp.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/web-interface/nav 返回风控码 %d", resp.Code))
		}
		err := fmt.Errorf("认证检查失败: %s(%d)", resp.Message, resp.Code)
		c.recordCheck(checkedAt, "error", err.Error())
		return AuthInfo{}, err
	}
	if !resp.Data.IsLogin {
		c.recordCheck(checkedAt, "invalid", "")
		return AuthInfo{IsLogin: false}, nil
	}
	c.recordCheck(checkedAt, "valid", "")
	return AuthInfo{IsLogin: true, Mid: resp.Data.Mid, Uname: resp.Data.Uname}, nil
}

func (c *Client) ResolveUID(ctx context.Context, keyword string) (string, string, error) {
	key := strings.TrimSpace(keyword)
	if key == "" {
		return "", "", errors.New("名称不能为空")
	}
	if isDigits(key) {
		return key, "", nil
	}

	if uid, name, ok := c.getCachedName(key); ok {
		return uid, name, nil
	}

	values := url.Values{}
	values.Set("search_type", "bili_user")
	values.Set("keyword", key)
	values.Set("page", "1")
	values.Set("page_size", "5")

	var resp userSearchResp
	if err := c.doGetJSON(ctx, "/x/web-interface/search/type", values.Encode(), &resp); err != nil {
		return "", "", err
	}
	if resp.Code != 0 {
		if resp.Code == -403 || resp.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/web-interface/search/type 返回风控码 %d", resp.Code))
		}
		return "", "", fmt.Errorf("名称解析失败: %s(%d)", resp.Message, resp.Code)
	}
	if len(resp.Data.Result) == 0 {
		return "", "", fmt.Errorf("未找到匹配博主: %s", key)
	}

	best := resp.Data.Result[0]
	for _, item := range resp.Data.Result {
		if strings.EqualFold(item.Uname, key) {
			best = item
			break
		}
	}
	uid := strconv.FormatInt(best.Mid, 10)
	name := best.Uname
	if uid == "" {
		return "", "", fmt.Errorf("名称解析失败: 结果缺少 UID")
	}

	c.setCachedName(key, uid, name)
	return uid, name, nil
}

func (c *Client) Download(ctx context.Context, videoID, dst string) (int64, error) {
	urls, size, err := c.getPlayURLs(ctx, videoID)
	if err != nil {
		return 0, err
	}
	if len(urls) == 0 {
		return 0, errors.New("获取下载地址失败")
	}

	tmp := dst + ".tmp"
	var lastErr error
	for _, u := range urls {
		n, err := c.downloadFile(ctx, u, tmp)
		if err == nil {
			if err := os.Rename(tmp, dst); err != nil {
				_ = os.Remove(tmp)
				return 0, err
			}
			if n == 0 && size > 0 {
				return size, nil
			}
			return n, nil
		}
		lastErr = err
		_ = os.Remove(tmp)
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, errors.New("下载失败")
}

func (c *Client) ReloadAuth() (bool, error) {
	reloadedAt := c.now()
	cookie, source, err := c.loadCookieFromFiles()
	if err != nil {
		c.recordReload(reloadedAt, "error", err.Error(), "")
		return false, err
	}
	if cookie == "" {
		c.recordReload(reloadedAt, "no_change", "", "")
		return false, nil
	}
	c.setCookie(cookie)
	c.recordReload(reloadedAt, "success", "", source)
	return true, nil
}

func (c *Client) RuntimeStatus() RuntimeStatus {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status
}

func (c *Client) doGetJSON(ctx context.Context, apiPath, query string, out any) error {
	if err := c.waitRisk(ctx); err != nil {
		return err
	}
	full := c.baseURL + apiPath
	if query != "" {
		full += "?" + query
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Referer", c.referer)
	if cookie := c.getCookie(); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusPreconditionFailed {
			c.markRiskReason(fmt.Sprintf("%s 返回 HTTP %d", apiPath, resp.StatusCode))
		}
		return fmt.Errorf("请求失败: %s", resp.Status)
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil {
		return err
	}
	c.resetRisk()
	return nil
}

func (c *Client) getPlayURLs(ctx context.Context, videoID string) ([]string, int64, error) {
	values := url.Values{}
	if bvid, ok := normalizeBVID(videoID); ok {
		values.Set("bvid", bvid)
	} else if aid, ok := normalizeAID(videoID); ok {
		values.Set("aid", aid)
	} else {
		return nil, 0, ErrInvalidID
	}

	var view viewResp
	if err := c.doGetJSON(ctx, "/x/web-interface/view", values.Encode(), &view); err != nil {
		return nil, 0, err
	}
	if view.Code != 0 {
		if view.Code == -403 || view.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/web-interface/view 返回风控码 %d", view.Code))
		}
		return nil, 0, fmt.Errorf("获取视频信息失败: %s(%d)", view.Message, view.Code)
	}
	if view.Data.CID == 0 {
		return nil, 0, errors.New("获取 cid 失败")
	}

	playQuery := url.Values{}
	for k, v := range values {
		playQuery[k] = v
	}
	playQuery.Set("cid", strconv.FormatInt(view.Data.CID, 10))
	playQuery.Set("qn", "80")

	var play playURLResp
	if err := c.doGetJSON(ctx, "/x/player/playurl", playQuery.Encode(), &play); err != nil {
		return nil, 0, err
	}
	if play.Code != 0 {
		if play.Code == -403 || play.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/player/playurl 返回风控码 %d", play.Code))
		}
		return nil, 0, fmt.Errorf("获取播放地址失败: %s(%d)", play.Message, play.Code)
	}
	if len(play.Data.Durl) == 0 {
		return nil, 0, errors.New("播放地址为空")
	}
	first := play.Data.Durl[0]
	urls := make([]string, 0, 1+len(first.BackupURL))
	if first.URL != "" {
		urls = append(urls, first.URL)
	}
	for _, backup := range first.BackupURL {
		if backup != "" {
			urls = append(urls, backup)
		}
	}
	return urls, first.Size, nil
}

func (c *Client) downloadFile(ctx context.Context, rawURL, dst string) (int64, error) {
	if err := c.waitRisk(ctx); err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Referer", c.referer)
	if cookie := c.getCookie(); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusPreconditionFailed {
			c.markRiskReason(fmt.Sprintf("下载地址返回 HTTP %d", resp.StatusCode))
		}
		return 0, fmt.Errorf("下载失败: %s", resp.Status)
	}

	file, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	n, err := io.Copy(file, resp.Body)
	if err != nil {
		return 0, err
	}
	c.resetRisk()
	return n, nil
}

func (c *Client) signWbiParams(ctx context.Context, params map[string]string) (string, error) {
	keys, err := c.getWbiKeys(ctx)
	if err != nil {
		return "", err
	}
	wts := c.now().Unix()
	return signParams(params, keys.mixinKey, wts), nil
}

func (c *Client) getWbiKeys(ctx context.Context) (wbiKeys, error) {
	c.mu.Lock()
	if c.wbi.mixinKey != "" && c.now().Before(c.wbi.expiresAt) {
		keys := c.wbi
		c.mu.Unlock()
		return keys, nil
	}
	c.mu.Unlock()

	var resp navResp
	if err := c.doGetJSON(ctx, "/x/web-interface/nav", "", &resp); err != nil {
		return wbiKeys{}, err
	}
	if resp.Code != 0 {
		if resp.Code == -403 || resp.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/web-interface/nav 返回风控码 %d", resp.Code))
		}
		return wbiKeys{}, fmt.Errorf("获取 wbi key 失败: %s(%d)", resp.Message, resp.Code)
	}
	imgURL := resp.Data.WbiImg.ImgURL
	subURL := resp.Data.WbiImg.SubURL
	if imgURL == "" || subURL == "" {
		return wbiKeys{}, errors.New("获取 wbi key 失败")
	}

	imgKey := trimFileKey(imgURL)
	subKey := trimFileKey(subURL)
	mixinKey := calcMixinKey(imgKey, subKey)
	if mixinKey == "" {
		return wbiKeys{}, errors.New("生成 wbi key 失败")
	}

	keys := wbiKeys{
		imgKey:    imgKey,
		subKey:    subKey,
		mixinKey:  mixinKey,
		expiresAt: c.now().Add(12 * time.Hour),
	}

	c.mu.Lock()
	c.wbi = keys
	c.mu.Unlock()
	return keys, nil
}

func (c *Client) loadCookieFromFiles() (string, string, error) {
	if c.cookieFile != "" {
		cookie, err := readCookieFile(c.cookieFile)
		if err != nil {
			return "", "", err
		}
		if cookie != "" {
			return cookie, "cookie_file", nil
		}
	}
	if c.sessdataFile != "" {
		content, err := os.ReadFile(c.sessdataFile)
		if err != nil {
			return "", "", err
		}
		return buildCookie("", string(content)), "sessdata_file", nil
	}
	return "", "", nil
}

func readCookieFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		return "", nil
	}
	if strings.Contains(value, "=") {
		return value, nil
	}
	return buildCookie("", value), nil
}

func buildCookie(cookie, sessdata string) string {
	cookie = strings.TrimSpace(cookie)
	if cookie != "" {
		return cookie
	}
	sess := strings.TrimSpace(sessdata)
	if sess == "" {
		return ""
	}
	if strings.HasPrefix(sess, "SESSDATA=") {
		return sess
	}
	return "SESSDATA=" + sess
}

func (c *Client) setCookie(cookie string) {
	c.cookieMu.Lock()
	c.cookie = strings.TrimSpace(cookie)
	c.cookieMu.Unlock()
}

func (c *Client) getCookie() string {
	c.cookieMu.RLock()
	val := c.cookie
	c.cookieMu.RUnlock()
	return val
}

func (c *Client) waitRisk(ctx context.Context) error {
	c.riskMu.Lock()
	until := c.riskUntil
	c.riskMu.Unlock()
	if until.IsZero() {
		return nil
	}
	now := c.now()
	if !until.After(now) {
		return nil
	}
	wait := until.Sub(now)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) markRisk() {
	c.markRiskReason("触发风控退避")
}

func (c *Client) markRiskReason(reason string) {
	c.riskMu.Lock()
	defer c.riskMu.Unlock()

	backoff := c.riskBackoff
	if backoff <= 0 {
		backoff = c.riskBase
	} else {
		backoff *= 2
	}
	if c.riskMax > 0 && backoff > c.riskMax {
		backoff = c.riskMax
	}
	if c.riskJitter > 0 {
		factor := 1 + (c.riskRand.Float64()*2-1)*c.riskJitter
		if factor < 0.1 {
			factor = 0.1
		}
		backoff = time.Duration(float64(backoff) * factor)
	}
	c.riskBackoff = backoff
	c.riskUntil = c.now().Add(backoff)
	c.statusMu.Lock()
	c.status.RiskBackoff = backoff
	c.status.RiskUntil = c.riskUntil
	c.status.LastRiskAt = c.now()
	c.status.LastRiskReason = strings.TrimSpace(reason)
	c.statusMu.Unlock()
}

func (c *Client) resetRisk() {
	c.riskMu.Lock()
	c.riskBackoff = 0
	c.riskUntil = time.Time{}
	c.riskMu.Unlock()
	c.statusMu.Lock()
	c.status.RiskBackoff = 0
	c.status.RiskUntil = time.Time{}
	c.statusMu.Unlock()
}

func (c *Client) getCachedName(key string) (string, string, bool) {
	c.nameCacheMu.RLock()
	entry, ok := c.nameCache[key]
	c.nameCacheMu.RUnlock()
	if !ok {
		return "", "", false
	}
	if c.now().After(entry.expiresAt) {
		c.nameCacheMu.Lock()
		delete(c.nameCache, key)
		c.nameCacheMu.Unlock()
		return "", "", false
	}
	return entry.uid, entry.name, true
}

func (c *Client) setCachedName(key, uid, name string) {
	if c.nameCacheTTL <= 0 {
		return
	}
	c.nameCacheMu.Lock()
	c.nameCache[key] = nameCacheEntry{uid: uid, name: name, expiresAt: c.now().Add(c.nameCacheTTL)}
	c.nameCacheMu.Unlock()
}

func trimFileKey(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Path != "" {
		raw = path.Base(u.Path)
	}
	if idx := strings.IndexByte(raw, '.'); idx > 0 {
		return raw[:idx]
	}
	return raw
}

func parseDuration(raw string) int {
	if raw == "" {
		return 0
	}
	parts := strings.Split(raw, ":")
	if len(parts) == 0 {
		return 0
	}
	seconds := 0
	for _, part := range parts {
		val, err := strconv.Atoi(part)
		if err != nil {
			return 0
		}
		seconds = seconds*60 + val
	}
	return seconds
}

func normalizeBVID(videoID string) (string, bool) {
	if strings.HasPrefix(videoID, "BV") || strings.HasPrefix(videoID, "bv") {
		return videoID, true
	}
	return "", false
}

func normalizeAID(videoID string) (string, bool) {
	id := strings.ToLower(videoID)
	if strings.HasPrefix(id, "av") {
		id = strings.TrimPrefix(id, "av")
	}
	if id == "" {
		return "", false
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return "", false
	}
	return id, true
}

func isDigits(raw string) bool {
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return raw != ""
}

func pickFavorite(fav, favorites int64) int64 {
	if fav != 0 {
		return fav
	}
	return favorites
}

func detectCookieSource(cfg config.BilibiliConfig) string {
	if strings.TrimSpace(cfg.Cookie) != "" || strings.TrimSpace(cfg.SESSDATA) != "" {
		return "config"
	}
	if strings.TrimSpace(cfg.CookieFile) != "" {
		return "cookie_file"
	}
	if strings.TrimSpace(cfg.SESSDATAFile) != "" {
		return "sessdata_file"
	}
	return ""
}

func (c *Client) recordReload(at time.Time, result, errMsg, source string) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.status.LastReloadAt = at
	c.status.LastReloadResult = result
	if strings.TrimSpace(source) != "" {
		c.status.CookieSource = strings.TrimSpace(source)
	}
	if strings.TrimSpace(errMsg) != "" {
		c.status.LastError = strings.TrimSpace(errMsg)
	}
}

func (c *Client) recordCheck(at time.Time, result, errMsg string) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.status.LastCheckAt = at
	c.status.LastCheckResult = result
	if strings.TrimSpace(errMsg) != "" {
		c.status.LastError = strings.TrimSpace(errMsg)
	}
}

// API response types

type navResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		IsLogin bool   `json:"isLogin"`
		Mid     int64  `json:"mid"`
		Uname   string `json:"uname"`
		WbiImg  struct {
			ImgURL string `json:"img_url"`
			SubURL string `json:"sub_url"`
		} `json:"wbi_img"`
	} `json:"data"`
}

type arcSearchResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		List struct {
			VList []struct {
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
			} `json:"vlist"`
		} `json:"list"`
	} `json:"data"`
}

type viewResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		CID int64 `json:"cid"`
	} `json:"data"`
}

type playURLResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Durl []struct {
			URL       string   `json:"url"`
			BackupURL []string `json:"backup_url"`
			Size      int64    `json:"size"`
		} `json:"durl"`
	} `json:"data"`
}

type nameCacheEntry struct {
	uid       string
	name      string
	expiresAt time.Time
}

type userSearchResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Result []struct {
			Mid   int64  `json:"mid"`
			Uname string `json:"uname"`
		} `json:"result"`
	} `json:"data"`
}
