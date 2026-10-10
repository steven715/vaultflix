package model

// MediaInfo 是從影片檔讀出的技術屬性（見 CONTEXT.md「Media Info」）。
type MediaInfo struct {
	DurationSeconds int
	Resolution      string
	MimeType        string
	VideoCodec      string
	AudioCodec      string
}

// MediaFile 指出要從哪個影片檔推導資料：所屬 Video、磁碟路徑與時長。
type MediaFile struct {
	VideoID         string
	Path            string
	DurationSeconds int
}
