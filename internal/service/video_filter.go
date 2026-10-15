package service

import (
	"fmt"
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
// page, newest first). Returns a *model.InvalidParamError (matching
// model.ErrInvalidInput) naming the first invalid parameter.
func ParseVideoFilter(get func(key string) string) (model.VideoFilter, error) {
	filter := model.VideoFilter{
		Page: 1, PageSize: defaultPageSize, SortBy: model.SortByCreatedAt, SortOrder: "desc", Query: get("q"),
	}
	if raw := get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return filter, invalidParam("page must be a positive integer")
		}
		filter.Page = n
	}
	if raw := get("page_size"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxPageSize {
			return filter, invalidParam(fmt.Sprintf("page_size must be between 1 and %d", maxPageSize))
		}
		filter.PageSize = n
	}
	if raw := model.VideoSort(get("sort_by")); raw != "" {
		if !raw.Valid() {
			return filter, invalidParam("sort_by must be one of: created_at, title, duration_seconds, file_size_bytes, random")
		}
		filter.SortBy = raw
	}
	if raw := strings.ToLower(get("sort_order")); raw != "" {
		if raw != "asc" && raw != "desc" {
			return filter, invalidParam("sort_order must be asc or desc")
		}
		filter.SortOrder = raw
	}
	ids, ok := tagIDsParam(get("tag_ids"))
	if !ok {
		return filter, invalidParam("tag_ids must be comma-separated positive integers")
	}
	filter.TagIDs = ids
	return filter, nil
}

func invalidParam(message string) error {
	return &model.InvalidParamError{Message: message}
}

// tagIDsParam parses comma-separated positive Tag IDs; ok is false when one
// is not.
func tagIDsParam(raw string) (ids []int, ok bool) {
	if raw == "" {
		return nil, true
	}
	for _, p := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || id < 1 {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}
