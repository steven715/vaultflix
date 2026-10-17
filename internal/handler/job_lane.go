package handler

import (
	"errors"

	"github.com/steven/vaultflix/internal/service"
)

// jobLaneBusyMessage names the Job holding a busy lane, for the admin
// (GLOSSARY.md「Job Lane」, ADR-0014); writeError answers it 409 job_lane_busy.
func jobLaneBusyMessage(err error) string {
	var busy *service.JobLaneBusyError
	if errors.As(err, &busy) {
		switch busy.RunningKind {
		case service.JobKindImport:
			return "匯入正在執行，完成或取消後再試"
		case service.JobKindBackfill:
			return "補齊正在執行，完成或取消後再試"
		case service.JobKindEnrichment:
			return "資料補全正在執行，完成或取消後再試"
		}
	}
	return "已有作業正在執行，完成後再試"
}
