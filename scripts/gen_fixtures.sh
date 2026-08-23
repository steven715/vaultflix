#!/bin/bash
# =============================================================================
# 重生整合測試 fixtures(.ci/fixtures/)
#
# 使用方式: bash scripts/gen_fixtures.sh   (需要 ffmpeg 在 PATH)
#
# sample_h264.mkv 的關鍵屬性 —— 重生時不可丟失:
#   - `-bf 2`:B-frames(video_delay > 0)是 remux seek 錯位 bug 的觸發條件
#     (ffmpeg CLI 對這類輸入的 input seek 會把 -ss 目標自動減 3/23s)。
#     沒有 B-frames 的 fixture 會讓 HLS 整合測試無聲失去對該 bug 的覆蓋。
#   - 30 秒、GOP 2s(-g 48 @24fps):產生多個 6s 分段,讓「跳抓最末段」
#     測試真正走到非零 start 的 on-demand 切段路徑。
#   - h264 + aac + mkv:落在 play_mode == "remux" 的分類。
#
# 配方與 internal/streaming/segment_generator_ffmpeg_test.go 的 makeMKVFixture
# 一致(該測試自產 fixture,不吃這裡的檔案;兩邊同步改)。
# =============================================================================

set -euo pipefail
cd "$(dirname "$0")/.."

ffmpeg -hide_banner -loglevel error \
    -f lavfi -i "testsrc2=size=160x120:rate=24" \
    -f lavfi -i "sine=frequency=440:sample_rate=44100" \
    -t 30 \
    -c:v libx264 -preset veryfast -crf 35 -bf 2 \
    -g 48 -keyint_min 48 -sc_threshold 0 \
    -c:a aac -b:a 32k -ac 1 \
    -y .ci/fixtures/sample_h264.mkv

echo "regenerated .ci/fixtures/sample_h264.mkv:"
ffprobe -v error -select_streams v:0 \
    -show_entries "stream=codec_name,has_b_frames:format=duration" \
    -of default .ci/fixtures/sample_h264.mkv
