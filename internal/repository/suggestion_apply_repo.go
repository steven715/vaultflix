package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/steven/vaultflix/internal/model"
)

// queryLinkGenreTag links a genre Tag to a Video; a Tag the Video already has
// is left as is, so accepting a Suggestion never fails on an existing genre.
const queryLinkGenreTag = `
    INSERT INTO video_tags (video_id, tag_id)
    VALUES ($1, $2)
    ON CONFLICT DO NOTHING
`

// Apply writes everything accepting a Metadata Suggestion changes in one
// transaction: it deletes the Suggestion, updates the Video's Metadata,
// upserts and links each Performer, and gets-or-creates and links each genre
// Tag. On any error nothing is written.
func (r *suggestionRepo) Apply(ctx context.Context, app model.SuggestionApplication) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin applying suggestion %s: %w", app.SuggestionID, err)
	}
	if err := applySuggestion(ctx, tx, app); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("failed to apply suggestion %s: %w (rollback failed: %v)", app.SuggestionID, err, rbErr)
		}
		return fmt.Errorf("failed to apply suggestion %s: %w", app.SuggestionID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit suggestion %s: %w", app.SuggestionID, err)
	}
	return nil
}

func applySuggestion(ctx context.Context, tx pgx.Tx, app model.SuggestionApplication) error {
	// Deleting first doubles as the existence check: a missing Suggestion
	// aborts before anything else is written.
	if err := execOne(ctx, tx, queryDeleteSuggestion, app.SuggestionID); err != nil {
		return fmt.Errorf("delete suggestion: %w", err)
	}
	m := app.Metadata
	if err := execOne(ctx, tx, queryUpdateVideoMetadata, app.VideoID,
		m.Code, m.Title, m.ReleaseDate, m.RuntimeMinutes, m.Maker, m.Label, m.Series, m.CoverKey); err != nil {
		return fmt.Errorf("update video %s metadata: %w", app.VideoID, err)
	}
	for _, p := range app.Performers {
		var id string
		if err := tx.QueryRow(ctx, queryUpsertActress, p.NameJa, p.NameRomaji, p.AvatarKey).Scan(&id, new(any)); err != nil {
			return fmt.Errorf("upsert performer %q: %w", p.NameJa, err)
		}
		if _, err := tx.Exec(ctx, queryAddVideoActress, app.VideoID, id); err != nil {
			return fmt.Errorf("link performer %s: %w", id, err)
		}
	}
	for _, name := range app.Genres {
		var tag model.Tag
		if err := tx.QueryRow(ctx, queryGetOrCreateTag, name, "genre").Scan(&tag.ID, &tag.Name, &tag.Category); err != nil {
			return fmt.Errorf("get or create genre %q: %w", name, err)
		}
		if _, err := tx.Exec(ctx, queryLinkGenreTag, app.VideoID, tag.ID); err != nil {
			return fmt.Errorf("link genre %d: %w", tag.ID, err)
		}
	}
	return nil
}

// execOne runs a statement that must touch exactly one row; zero rows is
// model.ErrNotFound.
func execOne(ctx context.Context, tx pgx.Tx, query string, args ...any) error {
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}
