package repository

import (
	"strconv"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

func buildWhereClause(filter model.VideoFilter) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if filter.Query != "" {
		conditions = append(conditions, "to_tsvector('simple', v.title) @@ to_tsquery('simple', $"+strconv.Itoa(argIdx)+")")
		args = append(args, filter.Query)
		argIdx++
	}

	if len(filter.TagIDs) > 0 {
		conditions = append(conditions,
			"v.id IN (SELECT vt.video_id FROM video_tags vt WHERE vt.tag_id = ANY($"+strconv.Itoa(argIdx)+") "+
				"GROUP BY vt.video_id HAVING COUNT(DISTINCT vt.tag_id) = $"+strconv.Itoa(argIdx+1)+")")
		args = append(args, filter.TagIDs, len(filter.TagIDs))
		argIdx += 2
	}

	clause := ""
	if len(conditions) > 0 {
		clause = " WHERE " + strings.Join(conditions, " AND ")
	}

	return clause, args
}
