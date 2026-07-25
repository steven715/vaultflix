package streaming

import (
	"math"
	"testing"
)

func TestParseKeyframeProbe_MixedOutput(t *testing.T) {
	out := "packet,0.000000,K__\n" +
		"packet,0.033000,___\n" +
		"packet,N/A,K__\n" + // 無 pts 的 keyframe packet 跳過
		"packet,8.341000,K__\n" +
		"format,120.500000\n"
	kf, total, err := parseKeyframeProbe(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kf) != 2 || kf[0] != 0 || kf[1] != 8.341 {
		t.Errorf("kf = %v, want [0 8.341]", kf)
	}
	if total != 120.5 {
		t.Errorf("total = %v, want 120.5", total)
	}
}

func TestParseKeyframeProbe_NoKeyframes(t *testing.T) {
	if _, _, err := parseKeyframeProbe("format,10.0\n"); err == nil {
		t.Error("expected error for output without keyframes")
	}
}

func TestParseKeyframeProbe_NoDuration(t *testing.T) {
	if _, _, err := parseKeyframeProbe("packet,0.000000,K__\nformat,N/A\n"); err == nil {
		t.Error("expected error for output without duration")
	}
}

func TestGroupSegments_Boundaries(t *testing.T) {
	dense := make([]float64, 0, 100) // 每 1.2s 一個 keyframe
	for i := 0; i < 100; i++ {
		dense = append(dense, float64(i)*1.2)
	}
	tests := []struct {
		name      string
		kf        []float64
		total     float64
		wantCount int
		wantLast  float64 // 末段結尾 = Start+Duration 應等於 total
	}{
		{name: "sparse keyframes every 8s", kf: []float64{0, 8, 16, 24}, total: 30, wantCount: 4, wantLast: 30},
		{name: "dense keyframes group to ~6s", kf: dense, total: 118.8, wantCount: 20, wantLast: 118.8},
		{name: "tail shorter than 1s merges into previous", kf: []float64{0, 7, 14}, total: 14.5, wantCount: 2, wantLast: 14.5},
		{name: "single keyframe whole file", kf: []float64{0}, total: 42, wantCount: 1, wantLast: 42},
		{name: "first keyframe not at zero still starts at 0", kf: []float64{1.5, 9.0}, total: 20, wantCount: 2, wantLast: 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			segs := GroupSegments(tc.kf, tc.total, DefaultSegmentTarget)
			if len(segs) != tc.wantCount {
				t.Fatalf("count = %d, want %d (segs=%v)", len(segs), tc.wantCount, segs)
			}
			if segs[0].Start != 0 {
				t.Errorf("first start = %v, want 0", segs[0].Start)
			}
			last := segs[len(segs)-1]
			if math.Abs(last.Start+last.Duration-tc.wantLast) > 1e-9 {
				t.Errorf("last end = %v, want %v", last.Start+last.Duration, tc.wantLast)
			}
			for i := 1; i < len(segs); i++ {
				if math.Abs(segs[i].Start-(segs[i-1].Start+segs[i-1].Duration)) > 1e-9 {
					t.Errorf("gap between segment %d and %d", i-1, i)
				}
			}
		})
	}
}

func TestGroupSegments_ZeroTotal(t *testing.T) {
	if segs := GroupSegments([]float64{0}, 0, DefaultSegmentTarget); segs != nil {
		t.Errorf("expected nil for zero total, got %v", segs)
	}
}
