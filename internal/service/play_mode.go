package service

import (
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// 瀏覽器 <video> 原生可解的編碼與容器集合（Phase 1 保守取交集）。
var (
	browserVideoCodecs = map[string]bool{"h264": true}
	browserAudioCodecs = map[string]bool{"aac": true, "mp3": true}
	browserContainers  = map[string]bool{"mp4": true, "mov": true}
)

// ClassifyPlayMode 依容器副檔名與編碼判定播放策略。
// direct：容器+編碼皆相容；remux：編碼相容但容器不相容；
// transcode：視訊或音訊編碼不相容。
func ClassifyPlayMode(container, videoCodec, audioCodec string) model.PlayMode {
	c := strings.ToLower(strings.TrimPrefix(container, "."))
	v := strings.ToLower(videoCodec)
	a := strings.ToLower(audioCodec)

	if !browserVideoCodecs[v] || !browserAudioCodecs[a] {
		return model.PlayModeTranscode
	}
	if !browserContainers[c] {
		return model.PlayModeRemux
	}
	return model.PlayModeDirect
}
