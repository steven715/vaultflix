package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/websocket"
)

// importItems lists the files one Import Job processes: every video file the
// Scan finds, so already-registered ones count as skipped.
type importItems struct {
	svc    *ImportService
	source *model.MediaSource
}

func (im importItems) Items(ctx context.Context) ([]JobItem, error) {
	files, err := im.svc.scanVideoFiles(im.source.MountPath)
	if err != nil {
		return nil, fmt.Errorf("scan media source %s: %w", im.source.ID, err)
	}
	items := make([]JobItem, len(files))
	for i, path := range files {
		items[i] = JobItem{ID: path, Name: filepath.Base(path), Run: func(ctx context.Context) error {
			result := im.svc.processOneFile(ctx, im.source, path)
			switch result.Status {
			case "skipped":
				return model.ErrJobItemSkipped
			case "error":
				return errors.New(result.Error)
			}
			return nil
		}}
	}
	return items, nil
}

// importReport sends one Import Job's Import messages.
type importReport struct {
	notifier websocket.Notifier
	userID   string
}

func (im *importReport) ItemStarted(job Job, it JobItem, current int) {
	im.send(websocket.TypeImportProgress, model.ImportProgress{
		JobID: job.ID, FileName: it.Name, Current: current, Total: job.Total, Status: "processing",
	})
}

func (im *importReport) ItemDone(job Job, it JobItem, current int, err error) {
	p := model.ImportProgress{
		JobID: job.ID, FileName: it.Name, Current: current, Total: job.Total, Status: "success",
		Imported: job.Succeeded, Skipped: job.Skipped, Failed: job.Failed,
	}
	switch {
	case errors.Is(err, model.ErrJobItemSkipped):
		p.Status = "skipped"
	case err != nil:
		p.Status, p.Error = "error", err.Error()
	}
	im.send(websocket.TypeImportProgress, p)
}

func (im *importReport) Failed(job Job, err error) {
	im.send(websocket.TypeImportError, map[string]string{"job_id": job.ID, "error": err.Error()})
}

func (im *importReport) Finished(job Job) {
	snap := toImportJob(&job)
	im.send(websocket.TypeImportComplete, snap)
	slog.Info("import completed",
		"source_id", snap.SourceID, "source_label", snap.SourceLabel, "total_scanned", snap.Total,
		"imported", snap.Imported, "skipped", snap.Skipped, "failed", snap.Failed,
	)
}

func (im *importReport) send(msgType string, payload interface{}) {
	im.notifier.SendToUser(im.userID, &websocket.Message{Type: msgType, Payload: payload})
}

// toImportJob shows a Job in the Import API's shape. An Import Job in which
// every file failed counts as failed, and a failed Scan is listed against the
// Media Source's mount path.
func toImportJob(j *Job) *model.ImportJob {
	source := j.Meta.(*model.MediaSource)
	errs := make([]model.ImportError, 0, len(j.Errors)+1)
	if j.ListError != "" {
		errs = append(errs, model.ImportError{FileName: source.MountPath, Error: j.ListError})
	}
	for _, e := range j.Errors {
		errs = append(errs, model.ImportError{FileName: e.ItemName, Error: e.Error})
	}
	status := j.Status
	if status == "completed" && j.Failed > 0 && j.Succeeded == 0 {
		status = "failed"
	}
	return &model.ImportJob{
		ID: j.ID, SourceID: source.ID, SourceLabel: source.Label, Status: status,
		Total: j.Total, Processed: j.Processed, Imported: j.Succeeded, Skipped: j.Skipped, Failed: j.Failed,
		Errors: errs, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
	}
}
