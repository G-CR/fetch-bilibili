package bilibili

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const defaultDiscoverySearchPageSize = 20

var searchHighlightTagPattern = regexp.MustCompile(`<[^>]+>`)

func (c *Client) SearchCreators(ctx context.Context, keyword string, page, pageSize int) ([]CreatorHit, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, errors.New("关键词不能为空")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultDiscoverySearchPageSize
	}

	values := url.Values{}
	values.Set("search_type", "bili_user")
	values.Set("keyword", keyword)
	values.Set("page", strconv.Itoa(page))
	values.Set("page_size", strconv.Itoa(pageSize))

	var resp creatorSearchResp
	if err := c.doGetJSON(ctx, "/x/web-interface/search/type", values.Encode(), &resp); err != nil {
		return nil, c.wrapDiscoveryRequestError("搜索作者", keyword, page, err)
	}
	if resp.Code != 0 {
		return nil, c.wrapDiscoveryAPIError("搜索作者", "/x/web-interface/search/type", keyword, page, resp.Code, resp.Message)
	}

	hits := make([]CreatorHit, 0, len(resp.Data.Result))
	for _, item := range resp.Data.Result {
		uid := item.Mid.String()
		if uid == "" {
			continue
		}
		hits = append(hits, CreatorHit{
			UID:           uid,
			Name:          cleanSearchText(item.Uname.String()),
			AvatarURL:     normalizeSearchURL(item.UPic.String()),
			ProfileURL:    creatorProfileURL(uid),
			FollowerCount: item.Fans.Int64(),
			Signature:     cleanSearchText(item.USign.String()),
		})
	}
	return hits, nil
}

func (c *Client) SearchVideos(ctx context.Context, keyword string, page, pageSize int) ([]VideoHit, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, errors.New("关键词不能为空")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultDiscoverySearchPageSize
	}

	values := url.Values{}
	values.Set("search_type", "video")
	values.Set("keyword", keyword)
	values.Set("page", strconv.Itoa(page))
	values.Set("page_size", strconv.Itoa(pageSize))

	var resp videoSearchResp
	if err := c.doGetJSON(ctx, "/x/web-interface/search/type", values.Encode(), &resp); err != nil {
		return nil, c.wrapDiscoveryRequestError("搜索视频", keyword, page, err)
	}
	if resp.Code != 0 {
		return nil, c.wrapDiscoveryAPIError("搜索视频", "/x/web-interface/search/type", keyword, page, resp.Code, resp.Message)
	}

	hits := make([]VideoHit, 0, len(resp.Data.Result))
	for _, item := range resp.Data.Result {
		uid := item.Mid.String()
		videoID := strings.TrimSpace(item.BVID.String())
		if uid == "" || videoID == "" {
			continue
		}
		hits = append(hits, VideoHit{
			UID:           uid,
			CreatorName:   cleanSearchText(item.Author.String()),
			VideoID:       videoID,
			Title:         cleanSearchText(item.Title.String()),
			Description:   cleanSearchText(item.Description.String()),
			PublishTime:   item.Pubdate.Time(),
			Duration:      parseFlexibleDuration(item.Duration.String()),
			CoverURL:      normalizeSearchURL(item.Pic.String()),
			ViewCount:     item.Play.Int64(),
			FavoriteCount: pickFavorite(item.Favorite.Int64(), item.Favorites.Int64()),
		})
	}
	return hits, nil
}

func (c *Client) wrapDiscoveryRequestError(action, keyword string, page int, err error) error {
	wrapped := fmt.Errorf("%s失败: %w", action, err)
	c.logDiscoveryError(action+"请求失败", keyword, page, wrapped)
	return wrapped
}

func (c *Client) wrapDiscoveryAPIError(action, apiPath, keyword string, page, code int, message string) error {
	if code == -403 || code == -412 {
		c.markRiskReason(fmt.Sprintf("%s 返回风控码 %d", apiPath, code))
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "未知错误"
	}
	err := fmt.Errorf("%s失败: %s(%d)", action, msg, code)
	c.logDiscoveryError(action+"接口返回异常", keyword, page, err)
	return err
}

func (c *Client) logDiscoveryError(action, keyword string, page int, err error) {
	if c == nil || c.logger == nil || err == nil {
		return
	}
	c.logger.Printf("%s，关键词=%s，页码=%d: %v", action, keyword, page, err)
}

func creatorProfileURL(uid string) string {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return ""
	}
	return "https://space.bilibili.com/" + uid
}

func cleanSearchText(raw string) string {
	raw = html.UnescapeString(raw)
	raw = searchHighlightTagPattern.ReplaceAllString(raw, "")
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func normalizeSearchURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

func parseFlexibleDuration(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return seconds
	}
	return parseDuration(raw)
}

type creatorSearchResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Result []creatorSearchItem `json:"result"`
	} `json:"data"`
}

type creatorSearchItem struct {
	Mid   flexibleInt64  `json:"mid"`
	Uname flexibleString `json:"uname"`
	UPic  flexibleString `json:"upic"`
	Fans  flexibleInt64  `json:"fans"`
	USign flexibleString `json:"usign"`
}

type videoSearchResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Result []videoSearchItem `json:"result"`
	} `json:"data"`
}

type videoSearchItem struct {
	Mid         flexibleInt64  `json:"mid"`
	Author      flexibleString `json:"author"`
	BVID        flexibleString `json:"bvid"`
	Title       flexibleString `json:"title"`
	Description flexibleString `json:"description"`
	Pubdate     flexibleTime   `json:"pubdate"`
	Play        flexibleInt64  `json:"play"`
	Favorite    flexibleInt64  `json:"favorite"`
	Favorites   flexibleInt64  `json:"favorites"`
	Pic         flexibleString `json:"pic"`
	Duration    flexibleString `json:"duration"`
}

type flexibleString string

func (v *flexibleString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*v = ""
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*v = flexibleString(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*v = flexibleString(number.String())
		return nil
	}
	*v = flexibleString(strings.Trim(string(data), `"`))
	return nil
}

func (v flexibleString) String() string {
	return strings.TrimSpace(string(v))
}

type flexibleInt64 int64

func (v *flexibleInt64) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*v = 0
		return nil
	}
	var number int64
	if err := json.Unmarshal(data, &number); err == nil {
		*v = flexibleInt64(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" || text == "--" {
			*v = 0
			return nil
		}
		number, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			*v = 0
			return nil
		}
		*v = flexibleInt64(number)
		return nil
	}
	*v = 0
	return nil
}

func (v flexibleInt64) Int64() int64 {
	return int64(v)
}

func (v flexibleInt64) String() string {
	if v == 0 {
		return ""
	}
	return strconv.FormatInt(int64(v), 10)
}

type flexibleTime time.Time

func (v *flexibleTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*v = flexibleTime(time.Time{})
		return nil
	}
	var number int64
	if err := json.Unmarshal(data, &number); err == nil {
		*v = flexibleTime(time.Unix(number, 0))
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			*v = flexibleTime(time.Time{})
			return nil
		}
		number, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			*v = flexibleTime(time.Time{})
			return nil
		}
		*v = flexibleTime(time.Unix(number, 0))
		return nil
	}
	*v = flexibleTime(time.Time{})
	return nil
}

func (v flexibleTime) Time() time.Time {
	return time.Time(v)
}
