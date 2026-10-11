package repository

import (
	"strconv"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

func buildWhereClause(filter model.VideoFilter) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	// next is the placeholder for the next arg, so each condition numbers its
	// own and none can drift.
	next := func() string { return "$" + strconv.Itoa(len(args)+1) }

	if filter.Query != "" {
		conditions = append(conditions, "to_tsvector('simple', v.title) @@ to_tsquery('simple', "+next()+")")
		args = append(args, filter.Query)
	}

	if len(filter.TagIDs) > 0 {
		tagIDs := next()
		args = append(args, filter.TagIDs)
		count := next()
		args = append(args, len(filter.TagIDs))
		conditions = append(conditions,
			"v.id IN (SELECT vt.video_id FROM video_tags vt WHERE vt.tag_id = ANY("+tagIDs+") "+
				"GROUP BY vt.video_id HAVING COUNT(DISTINCT vt.tag_id) = "+count+")")
	}

	clause := ""
	if len(conditions) > 0 {
		clause = " WHERE " + strings.Join(conditions, " AND ")
	}

	return clause, args
}
