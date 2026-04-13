package httpapi

import (
	"fmt"
	"strconv"
	"strings"
)

func parsePathID(path, prefix string) (int64, string, error) {
	if !strings.HasPrefix(path, prefix) {
		return 0, "", fmt.Errorf("path prefix mismatch")
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return 0, "", fmt.Errorf("missing id")
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", fmt.Errorf("invalid id")
	}
	if len(parts) == 1 {
		return id, "", nil
	}
	return id, parts[1], nil
}
