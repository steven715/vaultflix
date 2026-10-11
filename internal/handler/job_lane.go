package handler

import (
	"errors"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

// jobLaneBusy is the 409 body for a Job refused by a busy lane: the code is
// the same whoever holds the lane, and the message names that Job
// (GLOSSARY.md「Job Lane」, ADR-0014).
func jobLaneBusy(err error) model.ErrorResponse {
	return model.ErrorResponse{Error: "job_lane_busy", Message: jobLaneBusyMessage(err)}
}

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
