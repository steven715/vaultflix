package service

import (
	"errors"
	"strconv"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// Library query limits.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// ParseVideoFilter builds a Library query from its parameters; get returns ""
// for an absent one. Unset parameters take their defaults (page 1, 20 per
// page, newest first). Returns an error naming the parameter when one is
// invalid.
func ParseVideoFilter(get func(key string) string) (model.VideoFilter, error) {
	filter := model.VideoFilter{
		Page: 1, PageSize: defaultPageSize, SortBy: model.SortByCreatedAt, SortOrder: "desc", Query: get("q"),
	}
	var err error
	if filter.Page, err = intParam(get("page"), filter.Page, 1, 0); err != nil {
		return filter, errors.New("page must be a positive integer")
	}
	if filter.PageSize, err = intParam(get("page_size"), filter.PageSize, 1, maxPageSize); err != nil {
		return filter, errors.New("page_size must be between 1 and 100")
	}
	if raw := get("sort_by"); raw != "" {
		if !isVideoSort(model.VideoSort(raw)) {
			return filter, errors.New("sort_by must be one of: created_at, title, duration_seconds, file_size_bytes, random")
		}
		filter.SortBy = model.VideoSort(raw)
	}
	if raw := strings.ToLower(get("sort_order")); raw != "" {
		if raw != "asc" && raw != "desc" {
			return filter, errors.New("sort_order must be asc or desc")
		}
		filter.SortOrder = raw
	}
	if filter.TagIDs, err = tagIDsParam(get("tag_ids")); err != nil {
		return filter, errors.New("tag_ids must be comma-separated positive integers")
	}
	return filter, nil
}

func isVideoSort(s model.VideoSort) bool {
	switch s {
	case model.SortByCreatedAt, model.SortByTitle, model.SortByDuration, model.SortByFileSize, model.SortRandom:
		return true
	}
	return false
}

// intParam parses raw as an integer in [lo, hi] (hi 0: no upper bound), or
// returns def when raw is empty.
func intParam(raw string, def, lo, hi int) (int, error) {
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || (hi > 0 && n > hi) {
		return def, errors.New("out of range")
	}
	return n, nil
}

func tagIDsParam(raw string) ([]int, error) {
	if raw == "" {
		return nil, nil
	}
	var ids []int
	for _, p := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || id < 1 {
			return nil, errors.New("not a positive integer")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
