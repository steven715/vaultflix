package avid

import (
	"regexp"
	"strings"
)

var (
	// 去除網站浮水印前綴：domain@ 或 [domain] 或 (domain)
	reWatermark = regexp.MustCompile(`(?i)(^[a-z0-9.\-]+\.(?:com|net|org|cc|app|me|tv|xyz)@)|(\[[^\]]*\])|(\([^)]*\.(?:com|net|org|cc|app|me|tv|xyz)[^)]*\))`)
	// 去除解析度 / 畫質
	reResolution = regexp.MustCompile(`(?i)[-_ ]?(\d{3,4}p|2160p|4k|fhd|hd|\d{3,4}x\d{3,4})`)
	// 去除分片 cdN / partN
	reDisc = regexp.MustCompile(`(?i)[-_ ]?(cd\d+|part\d+|disc\d+)`)
	// 去除中字 / 無碼破解等後綴
	reSuffixTag = regexp.MustCompile(`(?i)[-_ ](c|ch|u|uc|uncensored|leak|hack)$`)

	// 抽取階梯（順序重要）
	reFC2      = regexp.MustCompile(`(?i)FC2[-_ ]*(?:PPV[-_ ]*)?(\d{5,7})`)
	reAmateur  = regexp.MustCompile(`(?i)(\d{3}[A-Z]{2,6})[-_ ]?(\d{2,5})`)
	reStandard = regexp.MustCompile(`(?i)([A-Z]{2,10})[-_ ]?(\d{2,5})`)
)

// Clean 去掉副檔名與常見雜訊（浮水印、解析度、分片、中字後綴），回傳清洗後字串。
func Clean(filename string) string {
	s := filename
	if i := strings.LastIndex(s, "."); i > 0 {
		s = s[:i]
	}
	s = reWatermark.ReplaceAllString(s, "")
	s = reResolution.ReplaceAllString(s, "")
	s = reDisc.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = reSuffixTag.ReplaceAllString(s, "")
	return strings.Trim(s, " -_")
}

// ExtractCode 從檔名抽出正規化番號。抽不到回 ("", false)。
// 階梯：FC2 → 素人數字前綴 → 標準 [A-Z]{2,10}-\d{2,5}。
func ExtractCode(filename string) (string, bool) {
	s := Clean(filename)
	if m := reFC2.FindStringSubmatch(s); m != nil {
		return "FC2-PPV-" + m[1], true
	}
	if m := reAmateur.FindStringSubmatch(s); m != nil {
		return strings.ToUpper(m[1]) + "-" + m[2], true
	}
	if m := reStandard.FindStringSubmatch(s); m != nil {
		return strings.ToUpper(m[1]) + "-" + m[2], true
	}
	return "", false
}
