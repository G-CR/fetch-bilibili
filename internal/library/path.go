package library

import (
	"path/filepath"
	"strings"
	"unicode"
)

func StoreRootPath(root string) string {
	return filepath.Join(root, "store")
}

func StoreVideoPath(root, platform, videoID string) string {
	return filepath.Join(StoreRootPath(root), normalizePlatform(platform), videoID+".mp4")
}

func CreatorDirectoryPath(root string, creator CreatorSnapshot) string {
	return filepath.Join(root, "library", normalizePlatform(creator.Platform), "creators", CreatorDirectoryName(creator.UID, creator.Name))
}

func CreatorDirectoryName(uid, name string) string {
	return uid + "_" + sanitizeName(name)
}

func normalizePlatform(platform string) string {
	if platform == "" {
		return "bilibili"
	}
	return platform
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unknown"
	}

	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		if isAllowedNameRune(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}

	cleaned := strings.Trim(b.String(), "_-")
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}

func isAllowedNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}
