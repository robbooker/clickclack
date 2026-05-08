package sqlite

import (
	"context"
	"database/sql"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func (s *Store) AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error {
	if role == "" {
		role = "member"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)`, workspaceID, userID, role, now())
	return err
}

func (s *Store) EnsureDefaultWorkspaceMember(ctx context.Context, userID string) (store.Workspace, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Workspace{}, err
	}
	defer tx.Rollback()

	var workspace store.Workspace
	err = tx.QueryRowContext(ctx, `SELECT id, name, slug, created_at FROM workspaces ORDER BY created_at LIMIT 1`).Scan(
		&workspace.ID,
		&workspace.Name,
		&workspace.Slug,
		&workspace.CreatedAt,
	)
	if err != nil && err != sql.ErrNoRows {
		return store.Workspace{}, err
	}
	if err == sql.ErrNoRows {
		workspace = store.Workspace{ID: newID("wsp"), Name: "ClickClack", Slug: "clickclack", CreatedAt: now()}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, name, slug, created_at) VALUES (?, ?, ?, ?)`, workspace.ID, workspace.Name, workspace.Slug, workspace.CreatedAt); err != nil {
			return store.Workspace{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO channels (id, workspace_id, name, kind, created_at) VALUES (?, ?, 'general', 'public', ?)`, newID("chn"), workspace.ID, workspace.CreatedAt); err != nil {
			return store.Workspace{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, 'member', ?)`, workspace.ID, userID, now()); err != nil {
		return store.Workspace{}, err
	}
	return workspace, tx.Commit()
}
