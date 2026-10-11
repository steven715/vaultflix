package repository

import (
	"reflect"
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestBuildWhereClause_NumbersPlaceholdersInArgOrder(t *testing.T) {
	tests := []struct {
		name       string
		filter     model.VideoFilter
		wantClause string
		wantArgs   []interface{}
	}{
		{
			name:       "no filter",
			filter:     model.VideoFilter{},
			wantClause: "",
			wantArgs:   nil,
		},
		{
			name:       "query only",
			filter:     model.VideoFilter{Query: "cat"},
			wantClause: " WHERE to_tsvector('simple', v.title) @@ to_tsquery('simple', $1)",
			wantArgs:   []interface{}{"cat"},
		},
		{
			name:   "tags only",
			filter: model.VideoFilter{TagIDs: []int{3, 5}},
			wantClause: " WHERE v.id IN (SELECT vt.video_id FROM video_tags vt WHERE vt.tag_id = ANY($1) " +
				"GROUP BY vt.video_id HAVING COUNT(DISTINCT vt.tag_id) = $2)",
			wantArgs: []interface{}{[]int{3, 5}, 2},
		},
		{
			name:   "query and tags",
			filter: model.VideoFilter{Query: "cat", TagIDs: []int{3}},
			wantClause: " WHERE to_tsvector('simple', v.title) @@ to_tsquery('simple', $1) AND " +
				"v.id IN (SELECT vt.video_id FROM video_tags vt WHERE vt.tag_id = ANY($2) " +
				"GROUP BY vt.video_id HAVING COUNT(DISTINCT vt.tag_id) = $3)",
			wantArgs: []interface{}{"cat", []int{3}, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args := buildWhereClause(tt.filter)
			if clause != tt.wantClause {
				t.Errorf("clause =\n%q\nwant\n%q", clause, tt.wantClause)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tt.wantArgs)
			}
		})
	}
}
