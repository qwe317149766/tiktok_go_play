package main

import (
	"regexp"
	"strings"
)

var (
	reVideoID = regexp.MustCompile(`/video/(\d+)`)
	reAwemeID = regexp.MustCompile(`(?i)(?:aweme_id=|item_id=)(\d+)`)
	reDigits  = regexp.MustCompile(`\d+`)
)

func parseAwemeID(link string) (string, bool) {
	s := strings.TrimSpace(link)
	if s == "" {
		return "", false
	}
	if m := reVideoID.FindStringSubmatch(s); len(m) == 2 {
		return m[1], true
	}
	if m := reAwemeID.FindStringSubmatch(s); len(m) == 2 {
		return m[1], true
	}
	// 最后兜底：取最后一段数字
	all := reDigits.FindAllString(s, -1)
	if len(all) == 0 {
		return "", false
	}
	return all[len(all)-1], true
}

// fetchStartCount: TODO
// 这里按 API.md 需要抓一次 start_count。
// 目前先返回 0，后续可以接入你现有的抓取/播放统计逻辑。
func fetchStartCount(_awemeID string, _link string) int64 {
	return 0
}


