package logic

import (
	"sort"
	"strings"

	"short-url/models"
	"short-url/mq"
)

// RecordClick 异步记录一次点击（通过 RabbitMQ 投递）
func RecordClick(shortCode, ip, referer, userAgent string) {
	mq.RecordClick(shortCode, ip, referer, userAgent)
}

// extractBrowser 从 User-Agent 中提取浏览器名称
func extractBrowser(ua string) string {
	switch {
	case strings.Contains(ua, "Edg/"):
		return "Edge"
	case strings.Contains(ua, "Chrome/"):
		return "Chrome"
	case strings.Contains(ua, "Firefox/"):
		return "Firefox"
	case strings.Contains(ua, "Safari/") && !strings.Contains(ua, "Chrome/"):
		return "Safari"
	default:
		return "Other"
	}
}

// mergeBrowsers 将原始 UA 计数合并为浏览器分类并返回 TopN
func mergeBrowsers(raw map[string]int64, n int) []models.TopSource {
	browserCount := make(map[string]int64)
	for ua, count := range raw {
		browserCount[extractBrowser(ua)] += count
	}

	type kv struct {
		k string
		v int64
	}
	list := make([]kv, 0, len(browserCount))
	for k, v := range browserCount {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })

	result := make([]models.TopSource, 0, n)
	for i := 0; i < n && i < len(list); i++ {
		result = append(result, models.TopSource{Source: list[i].k, Count: list[i].v})
	}
	return result
}
