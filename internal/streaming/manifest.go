package streaming

import (
	"fmt"
	"math"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// SegmentName 回傳第 i 段的檔名(與 handler 的 segment regex、快取檔名一致)。
func SegmentName(i int) string {
	return fmt.Sprintf("seg%05d.ts", i)
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
