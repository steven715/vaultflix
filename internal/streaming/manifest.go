package streaming

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// segmentNameRe 是 SegmentName 的反向文法;兩者必須一起改。
var segmentNameRe = regexp.MustCompile(`^seg(\d{5})\.ts$`)

// SegmentName 回傳第 i 段的 HLS Segment 檔名(manifest URI 與快取檔名共用)。
func SegmentName(i int) string {
	return fmt.Sprintf("seg%05d.ts", i)
}

// ParseSegmentName 是 SegmentName 的反函式;name 不符文法時 ok 為 false。
func ParseSegmentName(name string) (idx int, ok bool) {
	m := segmentNameRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	idx, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return idx, true
}

// BuildVODManifest 由分段邊界組出完整 VOD m3u8(含 ENDLIST)。
// segment URI 為相對檔名,token 由 handler 的 rewritePlaylistTokens 事後附加。
func BuildVODManifest(segs []model.SegmentBoundary) []byte {
	maxDur := 0.0
	for _, s := range segs {
		if s.Duration > maxDur {
			maxDur = s.Duration
		}
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-PLAYLIST-TYPE:VOD\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(maxDur)))
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")
	for i, s := range segs {
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n%s\n", s.Duration, SegmentName(i))
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return []byte(b.String())
}
