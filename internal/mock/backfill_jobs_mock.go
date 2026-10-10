package mock

import (
	"github.com/steven/vaultflix/internal/model"
)

// BackfillJobs is a hand-written fake for service.BackfillRunner as seen by
// the Backfill handler.
type BackfillJobs struct {
	StartErr  error
	CancelErr error
	Job       *model.BackfillJob

	StartedKind model.BackfillKind
}

func (m *BackfillJobs) Start(kind model.BackfillKind, userID string) (*model.BackfillJob, error) {
	m.StartedKind = kind
	if m.StartErr != nil {
		return nil, m.StartErr
	}
	return &model.BackfillJob{ID: "job-1", Kind: kind, Status: "running"}, nil
}

func (m *BackfillJobs) Active() *model.BackfillJob { return m.Job }

func (m *BackfillJobs) Cancel(jobID string) error { return m.CancelErr }
