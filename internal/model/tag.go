package model

type Tag struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

type TagWithCount struct {
	Tag
	VideoCount int `json:"video_count"`
}
