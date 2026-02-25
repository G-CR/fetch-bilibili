package bilibili

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

var mixinKeyEncTab = []int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35,
	27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13,
	37, 48, 7, 16, 24, 55, 40, 61, 26, 17, 0, 1, 60, 51, 30, 4,
	22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11, 36, 20, 34, 44, 52,
}

type wbiKeys struct {
	imgKey    string
	subKey    string
	mixinKey  string
	expiresAt time.Time
}

func calcMixinKey(imgKey, subKey string) string {
	raw := imgKey + subKey
	if len(raw) < 64 {
		return ""
	}

	var sb strings.Builder
	sb.Grow(64)
	for _, idx := range mixinKeyEncTab {
		if idx >= 0 && idx < len(raw) {
			sb.WriteByte(raw[idx])
		}
	}
	mixin := sb.String()
	if len(mixin) > 32 {
		mixin = mixin[:32]
	}
	return mixin
}

func signParams(params map[string]string, mixinKey string, wts int64) string {
	clean := make(map[string]string, len(params)+1)
	for k, v := range params {
		clean[k] = sanitizeValue(v)
	}
	clean["wts"] = strconv.FormatInt(wts, 10)

	keys := make([]string, 0, len(clean))
	for k := range clean {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(encodeComponent(k))
		sb.WriteByte('=')
		sb.WriteString(encodeComponent(clean[k]))
	}

	query := sb.String()
	sum := md5.Sum([]byte(query + mixinKey))
	wRid := hex.EncodeToString(sum[:])
	return query + "&w_rid=" + wRid
}

func sanitizeValue(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '!', '\'', '(', ')', '*':
			return -1
		default:
			return r
		}
	}, s)
}

func encodeComponent(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		b := s[i]
		if isUnreserved(b) {
			sb.WriteByte(b)
			continue
		}
		const hex = "0123456789ABCDEF"
		sb.WriteByte('%')
		sb.WriteByte(hex[b>>4])
		sb.WriteByte(hex[b&15])
	}
	return sb.String()
}

func isUnreserved(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '-' || b == '_' || b == '.' || b == '~':
		return true
	default:
		return false
	}
}
