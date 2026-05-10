package sqlite

import (
	"context"
	"errors"
	"strings"
	"time"
)

// PruneEvents deletes old durable events for a workspace while preserving the
// newest keepLatest events. At least one retention bound is required so callers
// cannot accidentally wipe the whole event log with default zero values.
func (s *Store) PruneEvents(ctx context.Context, workspaceID string, keepLatest int, before string) (int64, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	before = strings.TrimSpace(before)
	if workspaceID == "" {
		return 0, errors.New("workspace is required")
	}
	if keepLatest < 0 {
		return 0, errors.New("keep_latest must be non-negative")
	}
	if keepLatest == 0 && before == "" {
		return 0, errors.New("keep_latest or before is required")
	}
	if before != "" {
		parsed, err := time.Parse(time.RFC3339Nano, before)
		if err != nil {
			return 0, errors.New("before must be RFC3339")
		}
		before = parsed.UTC().Format(time.RFC3339Nano)
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM events
		WHERE workspace_id = ?
		  AND (? = '' OR julianday(created_at) < julianday(?))
		  AND id NOT IN (
		    SELECT id
		    FROM events
		    WHERE workspace_id = ?
		    ORDER BY cursor DESC
		    LIMIT ?
		  )`, workspaceID, before, before, workspaceID, keepLatest)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
