package bilibili

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/live"
)

var ErrInvalidID = errors.New("无效的 ID")

// PermanentError 表示不可恢复的 API 错误，重试无意义。
type PermanentError struct {
	Code    int
	Message string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("%s(%d)", e.Message, e.Code)
}

// permanentPlayURLCodes 是播放地址接口中明确不可恢复的错误码。
// 87008: 视频无法播放（通常为版权限制或地区限制）
// -404:  视频不存在
// 62002: 视频不可见
// -10403: 大会员专享
var permanentPlayURLCodes = map[int]bool{
	87008:  true,
	-404:   true,
	62002:  true,
	-10403: true,
}

const (
	defaultBaseURL  = "https://api.bilibili.com"
	defaultReferer  = "https://www.bilibili.com"
	defaultPageSize = 5
)

// Client is a Bilibili API client.
type Client struct {
	httpClient *http.Client
	userAgent  string
	referer    string
	baseURL    string
	logger     *log.Logger
	now        func() time.Time

	cookieMu sync.RWMutex
	cookie   string

	mu  sync.Mutex
	wbi wbiKeys

	nameCacheMu  sync.RWMutex
	nameCache    map[string]nameCacheEntry
	nameCacheTTL time.Duration
	pageSize     int

	riskMu      sync.Mutex
	riskUntil   time.Time
	riskBackoff time.Duration
	riskBase    time.Duration
	riskMax     time.Duration
	riskJitter  float64
	riskRand    *rand.Rand
	statusMu    sync.RWMutex
	status      RuntimeStatus
	publisher   systemEventPublisher
}

type Option func(*Client)

type RuntimeStatus struct {
	CookieConfigured bool
	CookieSource     string
	LastReloadAt     time.Time
	LastReloadResult string
	LastCheckAt      time.Time
	LastCheckResult  string
	LastCheckMid     int64
	LastCheckUname   string
	LastError        string
	RiskUntil        time.Time
	RiskBackoff      time.Duration
	LastRiskAt       time.Time
	LastRiskReason   string
}

type systemEventPublisher interface {
	Publish(evt live.Event)
}

type systemCookieState struct {
	Configured       bool
	Source           string
	Status           string
	IsLogin          bool
	Mid              int64
	Uname            string
	LastReloadResult string
	LastCheckResult  string
	LastError        string
}

type systemRiskState struct {
	Active       bool
	BackoffUntil time.Time
	LastReason   string
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
	pageSize := cfg.FetchPageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}

	cookie := buildCookie(cfg.Cookie, cfg.SESSDATA)
	cookieConfigured := cookie != ""

	c := &Client{
		httpClient:   &http.Client{Timeout: timeout},
		userAgent:    userAgent,
		referer:      defaultReferer,
		baseURL:      defaultBaseURL,
		cookie:       cookie,
		logger:       logger,
		now:          time.Now,
		nameCache:    make(map[string]nameCacheEntry),
		nameCacheTTL: cacheTTL,
		pageSize:     pageSize,
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

func (c *Client) SetPublisher(publisher systemEventPublisher) {
	c.statusMu.Lock()
	c.publisher = publisher
	c.statusMu.Unlock()
}

func (c *Client) ListVideos(ctx context.Context, uid string) ([]VideoMeta, error) {
	if uid == "" {
		return nil, ErrInvalidID
	}

	params := map[string]string{
		"mid":   uid,
		"pn":    "1",
		"ps":    strconv.Itoa(c.pageSize),
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
	before := c.RuntimeStatus()
	var resp navResp
	if err := c.doGetJSON(ctx, "/x/web-interface/nav", "", &resp); err != nil {
		c.recordCheck(checkedAt, "error", err.Error(), 0, "")
		c.publishSystemChangedIfNeeded(before, c.RuntimeStatus())
		return AuthInfo{}, err
	}
	if resp.Code != 0 {
		if resp.Code == -403 || resp.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/web-interface/nav 返回风控码 %d", resp.Code))
		}
		err := fmt.Errorf("认证检查失败: %s(%d)", resp.Message, resp.Code)
		c.recordCheck(checkedAt, "error", err.Error(), 0, "")
		c.publishSystemChangedIfNeeded(before, c.RuntimeStatus())
		return AuthInfo{}, err
	}
	if !resp.Data.IsLogin {
		c.recordCheck(checkedAt, "invalid", "", 0, "")
		c.publishSystemChangedIfNeeded(before, c.RuntimeStatus())
		return AuthInfo{IsLogin: false}, nil
	}
	c.recordCheck(checkedAt, "valid", "", resp.Data.Mid, resp.Data.Uname)
	c.publishSystemChangedIfNeeded(before, c.RuntimeStatus())
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
	c.setCachedName(uidCacheKey(uid), uid, name)
	return uid, name, nil
}

func (c *Client) ResolveName(ctx context.Context, uid string) (string, error) {
	key := strings.TrimSpace(uid)
	if key == "" {
		return "", errors.New("uid 不能为空")
	}

	if _, name, ok := c.getCachedName(uidCacheKey(key)); ok && strings.TrimSpace(name) != "" {
		return name, nil
	}

	query, err := c.signWbiParams(ctx, map[string]string{"mid": key})
	if err != nil {
		return "", err
	}

	var resp userProfileResp
	if err := c.doGetJSON(ctx, "/x/space/wbi/acc/info", query, &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 {
		if resp.Code == -403 || resp.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/space/wbi/acc/info 返回风控码 %d", resp.Code))
		}
		return "", fmt.Errorf("查询博主信息失败: %s(%d)", resp.Message, resp.Code)
	}

	name := strings.TrimSpace(resp.Data.Name)
	if name == "" {
		return "", errors.New("查询博主信息失败: 名称为空")
	}
	c.setCachedName(uidCacheKey(key), key, name)
	return name, nil
}

func (c *Client) Download(ctx context.Context, videoID, dst string) (int64, error) {
	plan, err := c.getPlayPlan(ctx, videoID)
	if err != nil {
		return 0, err
	}
	if len(plan.directURLs) == 0 && (len(plan.videoURLs) == 0 || len(plan.audioURLs) == 0) {
		return 0, errors.New("获取下载地址失败")
	}

	if len(plan.videoURLs) > 0 && len(plan.audioURLs) > 0 {
		size, err := c.downloadAndMergeDash(ctx, plan, dst)
		if err == nil {
			return size, nil
		}
		return 0, err
	}

	tmp := dst + ".tmp"
	n, err := c.downloadFirstAvailable(ctx, plan.directURLs, tmp)
	if err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	if n == 0 && plan.expectedSize > 0 {
		return plan.expectedSize, nil
	}
	return n, nil
}

func (c *Client) ReloadAuth() (bool, error) {
	before := c.RuntimeStatus()
	c.recordReload(c.now(), "no_change", "", "")
	c.publishSystemChangedIfNeeded(before, c.RuntimeStatus())
	return false, nil
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

type playPlan struct {
	directURLs   []string
	videoURLs    []string
	audioURLs    []string
	expectedSize int64
}

func (c *Client) getPlayPlan(ctx context.Context, videoID string) (playPlan, error) {
	values := url.Values{}
	if bvid, ok := normalizeBVID(videoID); ok {
		values.Set("bvid", bvid)
	} else if aid, ok := normalizeAID(videoID); ok {
		values.Set("aid", aid)
	} else {
		return playPlan{}, ErrInvalidID
	}

	var view viewResp
	if err := c.doGetJSON(ctx, "/x/web-interface/view", values.Encode(), &view); err != nil {
		return playPlan{}, err
	}
	if view.Code != 0 {
		if view.Code == -403 || view.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/web-interface/view 返回风控码 %d", view.Code))
		}
		return playPlan{}, fmt.Errorf("获取视频信息失败: %s(%d)", view.Message, view.Code)
	}
	if view.Data.CID == 0 {
		return playPlan{}, errors.New("获取 cid 失败")
	}

	playQuery := url.Values{}
	for k, v := range values {
		playQuery[k] = v
	}
	playQuery.Set("cid", strconv.FormatInt(view.Data.CID, 10))
	playQuery.Set("qn", "80")
	playQuery.Set("fnver", "0")
	playQuery.Set("fnval", "4048")
	playQuery.Set("fourk", "1")

	var play playURLResp
	if err := c.doGetJSON(ctx, "/x/player/playurl", playQuery.Encode(), &play); err != nil {
		return playPlan{}, err
	}
	if play.Code != 0 {
		if play.Code == -403 || play.Code == -412 {
			c.markRiskReason(fmt.Sprintf("/x/player/playurl 返回风控码 %d", play.Code))
		}
		if permanentPlayURLCodes[play.Code] {
			return playPlan{}, &PermanentError{Code: play.Code, Message: play.Message}
		}
		return playPlan{}, fmt.Errorf("获取播放地址失败: %s(%d)", play.Message, play.Code)
	}

	plan := playPlan{
		videoURLs:    pickDashURLs(play.Data.Dash.Video),
		audioURLs:    pickDashURLs(play.Data.Dash.Audio),
		expectedSize: play.Data.Dash.bestVideoSize() + play.Data.Dash.bestAudioSize(),
	}
	if len(plan.videoURLs) > 0 && len(plan.audioURLs) > 0 {
		return plan, nil
	}

	if len(play.Data.Durl) == 0 {
		return playPlan{}, errors.New("播放地址为空")
	}
	first := play.Data.Durl[0]
	plan.expectedSize = first.Size
	plan.directURLs = make([]string, 0, 1+len(first.BackupURL))
	if first.URL != "" {
		plan.directURLs = append(plan.directURLs, first.URL)
	}
	for _, backup := range first.BackupURL {
		if backup != "" {
			plan.directURLs = append(plan.directURLs, backup)
		}
	}
	return plan, nil
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

func (c *Client) downloadFirstAvailable(ctx context.Context, urls []string, dst string) (int64, error) {
	if len(urls) == 0 {
		return 0, errors.New("下载地址为空")
	}
	var lastErr error
	for _, rawURL := range urls {
		n, err := c.downloadFile(ctx, rawURL, dst)
		if err == nil {
			return n, nil
		}
		lastErr = err
		_ = os.Remove(dst)
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, errors.New("下载失败")
}

func (c *Client) downloadAndMergeDash(ctx context.Context, plan playPlan, dst string) (int64, error) {
	tmpDir, err := os.MkdirTemp(filepath.Dir(dst), "dash-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmpDir)

	videoPath := filepath.Join(tmpDir, "video.m4s")
	audioPath := filepath.Join(tmpDir, "audio.m4s")
	outPath := filepath.Join(tmpDir, "merged.mp4")

	if _, err := c.downloadFirstAvailable(ctx, plan.videoURLs, videoPath); err != nil {
		return 0, err
	}
	if _, err := c.downloadFirstAvailable(ctx, plan.audioURLs, audioPath); err != nil {
		return 0, err
	}
	if err := c.mergeAV(ctx, videoPath, audioPath, outPath); err != nil {
		return 0, err
	}
	if err := os.Rename(outPath, dst); err != nil {
		return 0, err
	}
	info, err := os.Stat(dst)
	if err != nil {
		return 0, err
	}
	if info.Size() == 0 && plan.expectedSize > 0 {
		return plan.expectedSize, nil
	}
	return info.Size(), nil
}

func (c *Client) mergeAV(ctx context.Context, videoPath, audioPath, dst string) error {
	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-loglevel", "error",
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		"-movflags", "+faststart",
		dst,
	)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("未找到 ffmpeg，无法合并音视频")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("合并音视频失败: %w", err)
		}
		return fmt.Errorf("合并音视频失败: %s", msg)
	}
	return nil
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
	before := c.RuntimeStatus()
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
	c.publishSystemChangedIfNeeded(before, c.RuntimeStatus())
}

func (c *Client) resetRisk() {
	before := c.RuntimeStatus()
	c.riskMu.Lock()
	c.riskBackoff = 0
	c.riskUntil = time.Time{}
	c.riskMu.Unlock()
	c.statusMu.Lock()
	c.status.RiskBackoff = 0
	c.status.RiskUntil = time.Time{}
	c.statusMu.Unlock()
	c.publishSystemChangedIfNeeded(before, c.RuntimeStatus())
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

func uidCacheKey(uid string) string {
	return "uid:" + strings.TrimSpace(uid)
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

func (c *Client) recordCheck(at time.Time, result, errMsg string, mid int64, uname string) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.status.LastCheckAt = at
	c.status.LastCheckResult = result
	c.status.LastCheckMid = mid
	c.status.LastCheckUname = strings.TrimSpace(uname)
	if strings.TrimSpace(errMsg) != "" {
		c.status.LastError = strings.TrimSpace(errMsg)
	}
}

func (c *Client) publishSystemChangedIfNeeded(before, after RuntimeStatus) {
	if c.cookieState(before) != c.cookieState(after) || c.riskState(before) != c.riskState(after) {
		c.publishSystemChanged(after)
	}
}

func (c *Client) publishSystemChanged(status RuntimeStatus) {
	c.statusMu.RLock()
	publisher := c.publisher
	c.statusMu.RUnlock()
	if publisher == nil {
		return
	}
	at := c.now()
	publisher.Publish(live.Event{
		ID:   fmt.Sprintf("system-%d", at.UnixNano()),
		Type: "system.changed",
		At:   at,
		Payload: map[string]any{
			"cookie": c.cookiePayload(status),
			"risk":   c.riskPayload(status),
		},
	})
}

func (c *Client) cookieState(status RuntimeStatus) systemCookieState {
	cookieStatus, isLogin := deriveCookieStatus(status)
	return systemCookieState{
		Configured:       status.CookieConfigured,
		Source:           status.CookieSource,
		Status:           cookieStatus,
		IsLogin:          isLogin,
		Mid:              status.LastCheckMid,
		Uname:            strings.TrimSpace(status.LastCheckUname),
		LastReloadResult: status.LastReloadResult,
		LastCheckResult:  status.LastCheckResult,
		LastError:        strings.TrimSpace(status.LastError),
	}
}

func (c *Client) riskState(status RuntimeStatus) systemRiskState {
	return systemRiskState{
		Active:       !status.RiskUntil.IsZero() && status.RiskUntil.After(c.now()),
		BackoffUntil: status.RiskUntil,
		LastReason:   strings.TrimSpace(status.LastRiskReason),
	}
}

func (c *Client) cookiePayload(status RuntimeStatus) map[string]any {
	cookieStatus, isLogin := deriveCookieStatus(status)
	payload := map[string]any{
		"configured":         status.CookieConfigured,
		"is_login":           isLogin,
		"mid":                status.LastCheckMid,
		"uname":              strings.TrimSpace(status.LastCheckUname),
		"status":             cookieStatus,
		"source":             strings.TrimSpace(status.CookieSource),
		"last_check_at":      formatSystemEventTime(status.LastCheckAt),
		"last_check_result":  status.LastCheckResult,
		"last_reload_at":     formatSystemEventTime(status.LastReloadAt),
		"last_reload_result": status.LastReloadResult,
		"last_error":         strings.TrimSpace(status.LastError),
	}
	if cookieStatus == "error" {
		payload["error"] = strings.TrimSpace(status.LastError)
	}
	return payload
}

func (c *Client) riskPayload(status RuntimeStatus) map[string]any {
	now := c.now()
	active := !status.RiskUntil.IsZero() && status.RiskUntil.After(now)
	payload := map[string]any{
		"level":           "低",
		"active":          active,
		"backoff_until":   "",
		"backoff_seconds": int64(0),
		"last_hit_at":     formatSystemEventTime(status.LastRiskAt),
		"last_reason":     strings.TrimSpace(status.LastRiskReason),
	}
	if active {
		payload["level"] = "高"
		payload["backoff_until"] = formatSystemEventTime(status.RiskUntil)
		payload["backoff_seconds"] = int64(math.Ceil(status.RiskUntil.Sub(now).Seconds()))
		if payload["backoff_seconds"].(int64) < 0 {
			payload["backoff_seconds"] = int64(0)
		}
	}
	return payload
}

func deriveCookieStatus(status RuntimeStatus) (string, bool) {
	if !status.CookieConfigured {
		return "not_configured", false
	}
	switch status.LastCheckResult {
	case "valid":
		return "valid", true
	case "invalid":
		return "invalid", false
	case "error":
		return "error", false
	default:
		return "unknown", false
	}
}

func formatSystemEventTime(v time.Time) string {
	if v.IsZero() {
		return ""
	}
	return v.Format(time.RFC3339)
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
		Dash playDash `json:"dash"`
	} `json:"data"`
}

type playDash struct {
	Video []playDashItem `json:"video"`
	Audio []playDashItem `json:"audio"`
}

func (d playDash) bestVideoSize() int64 {
	if len(d.Video) == 0 {
		return 0
	}
	best := d.Video[0]
	for _, item := range d.Video[1:] {
		if item.Bandwidth > best.Bandwidth {
			best = item
		}
	}
	return best.Bandwidth
}

func (d playDash) bestAudioSize() int64 {
	if len(d.Audio) == 0 {
		return 0
	}
	best := d.Audio[0]
	for _, item := range d.Audio[1:] {
		if item.Bandwidth > best.Bandwidth {
			best = item
		}
	}
	return best.Bandwidth
}

type playDashItem struct {
	BaseURL      string   `json:"baseUrl"`
	BaseURLAlt   string   `json:"base_url"`
	BackupURL    []string `json:"backupUrl"`
	BackupURLAlt []string `json:"backup_url"`
	Bandwidth    int64    `json:"bandwidth"`
}

func (i playDashItem) urls() []string {
	base := strings.TrimSpace(i.BaseURL)
	if base == "" {
		base = strings.TrimSpace(i.BaseURLAlt)
	}
	urls := make([]string, 0, 1+len(i.BackupURL)+len(i.BackupURLAlt))
	if base != "" {
		urls = append(urls, base)
	}
	for _, item := range i.BackupURL {
		if strings.TrimSpace(item) != "" {
			urls = append(urls, item)
		}
	}
	for _, item := range i.BackupURLAlt {
		if strings.TrimSpace(item) != "" {
			urls = append(urls, item)
		}
	}
	return urls
}

func pickDashURLs(items []playDashItem) []string {
	if len(items) == 0 {
		return nil
	}
	best := items[0]
	for _, item := range items[1:] {
		if item.Bandwidth > best.Bandwidth {
			best = item
		}
	}
	return best.urls()
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

type userProfileResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Name string `json:"name"`
	} `json:"data"`
}
