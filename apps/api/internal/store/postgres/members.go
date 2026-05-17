package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openclaw/clickclack/apps/api/internal/store"
	"github.com/openclaw/clickclack/apps/api/internal/store/postgres/storedb"
)

func (s *Store) AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error {
	if role == "" {
		role = store.WorkspaceRoleMember
	}
	return s.q.InsertWorkspaceMember(ctx, storedb.InsertWorkspaceMemberParams{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Role:        normalizeWorkspaceRole(role),
		CreatedAt:   now(),
	})
}

func (s *Store) EnsureDefaultWorkspaceMember(ctx context.Context, userID string) (store.Workspace, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Workspace{}, err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)

	workspace, err := postgresWorkspaceBySlugTx(ctx, tx, "clickclack")
	memberRole := "member"
	if err != nil && err != sql.ErrNoRows {
		return store.Workspace{}, err
	}
	if err == sql.ErrNoRows {
		workspace = store.Workspace{ID: newID("wsp"), Name: "ClickClack", Slug: "clickclack", CreatedAt: now()}
		insertedWorkspace := false
		createdWorkspace := false
		for attempt := 0; attempt < routeIDInsertAttempts; attempt++ {
			workspaceRouteID, err := newRouteID('T')
			if err != nil {
				return store.Workspace{}, err
			}
			workspace.RouteID = workspaceRouteID
			result, err := tx.ExecContext(ctx, `
				INSERT INTO workspaces (id, route_id, name, slug, created_at)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT DO NOTHING`,
				workspace.ID, sqlText(workspace.RouteID), workspace.Name, workspace.Slug, workspace.CreatedAt,
			)
			if err != nil {
				return store.Workspace{}, err
			}
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return store.Workspace{}, err
			}
			existingWorkspace, err := postgresWorkspaceBySlugTx(ctx, tx, "clickclack")
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return store.Workspace{}, err
			}
			workspace = existingWorkspace
			createdWorkspace = rowsAffected == 1
			if createdWorkspace {
				memberRole = "owner"
			}
			insertedWorkspace = true
			break
		}
		if !insertedWorkspace {
			return store.Workspace{}, errors.New("could not create workspace route_id after collision retries")
		}
		if createdWorkspace {
			channelID := newID("chn")
			insertedChannel := false
			for attempt := 0; attempt < routeIDInsertAttempts; attempt++ {
				channelRouteID, err := newRouteID('C')
				if err != nil {
					return store.Workspace{}, err
				}
				if err := qtx.InsertDefaultChannel(ctx, storedb.InsertDefaultChannelParams{
					ID:          channelID,
					RouteID:     sqlText(channelRouteID),
					WorkspaceID: workspace.ID,
					CreatedAt:   workspace.CreatedAt,
				}); err != nil {
					if isRouteIDConflict(err) {
						continue
					}
					return store.Workspace{}, err
				}
				insertedChannel = true
				break
			}
			if !insertedChannel {
				return store.Workspace{}, errors.New("could not create channel route_id after collision retries")
			}
		}
	}
	if err := qtx.InsertDefaultWorkspaceMember(ctx, storedb.InsertDefaultWorkspaceMemberParams{
		WorkspaceID: workspace.ID,
		UserID:      userID,
		Role:        memberRole,
		CreatedAt:   now(),
	}); err != nil {
		return store.Workspace{}, err
	}
	return workspace, tx.Commit()
}

func (s *Store) EnsureDefaultGuestWorkspaceMember(ctx context.Context, userID, role string) (store.Workspace, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Workspace{}, err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)
	role = normalizeWorkspaceRole(role)

	workspace, err := postgresWorkspaceBySlugTx(ctx, tx, "guests")
	if err != nil && err != sql.ErrNoRows {
		return store.Workspace{}, err
	}
	if err == sql.ErrNoRows {
		workspace = store.Workspace{ID: newID("wsp"), Name: "Guests", Slug: "guests", CreatedAt: now()}
		insertedWorkspace := false
		for attempt := 0; attempt < routeIDInsertAttempts; attempt++ {
			workspaceRouteID, err := newRouteID('T')
			if err != nil {
				return store.Workspace{}, err
			}
			workspace.RouteID = workspaceRouteID
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO workspaces (id, route_id, name, slug, created_at)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT DO NOTHING`,
				workspace.ID, sqlText(workspace.RouteID), workspace.Name, workspace.Slug, workspace.CreatedAt,
			); err != nil {
				return store.Workspace{}, err
			}
			workspace, err = postgresWorkspaceBySlugTx(ctx, tx, "guests")
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return store.Workspace{}, err
			}
			insertedWorkspace = true
			break
		}
		if !insertedWorkspace {
			return store.Workspace{}, errors.New("could not create guest workspace route_id after collision retries")
		}
	}
	if err := postgresEnsureNamedChannelTx(ctx, tx, workspace.ID, "guest", "public"); err != nil {
		return store.Workspace{}, err
	}
	if err := postgresEnsureNamedChannelTx(ctx, tx, workspace.ID, "general", "public"); err != nil {
		return store.Workspace{}, err
	}
	if err := qtx.UpsertGuestWorkspaceMemberRole(ctx, storedb.UpsertGuestWorkspaceMemberRoleParams{
		WorkspaceID: workspace.ID,
		UserID:      userID,
		Role:        role,
		CreatedAt:   now(),
	}); err != nil {
		return store.Workspace{}, err
	}
	workspace.Role, _ = memberRoleTx(ctx, tx, workspace.ID, userID)
	return workspace, tx.Commit()
}

func postgresWorkspaceBySlugTx(ctx context.Context, tx *sql.Tx, slug string) (store.Workspace, error) {
	var workspace store.Workspace
	err := tx.QueryRowContext(ctx, `SELECT id, COALESCE(route_id, ''), name, slug, created_at FROM workspaces WHERE slug = $1`, slug).Scan(
		&workspace.ID, &workspace.RouteID, &workspace.Name, &workspace.Slug, &workspace.CreatedAt,
	)
	return workspace, err
}

func postgresEnsureNamedChannelTx(ctx context.Context, tx *sql.Tx, workspaceID, name, kind string) error {
	var existingID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM channels WHERE workspace_id = $1 AND name = $2`, workspaceID, name).Scan(&existingID)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	for attempt := 0; attempt < routeIDInsertAttempts; attempt++ {
		routeID, err := newRouteID('C')
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO channels (id, route_id, workspace_id, name, kind, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT DO NOTHING`,
			newID("chn"), sqlText(routeID), workspaceID, name, kind, now(),
		); err != nil {
			if isRouteIDConflict(err) {
				continue
			}
			return err
		}
		err = tx.QueryRowContext(ctx, `SELECT id FROM channels WHERE workspace_id = $1 AND name = $2`, workspaceID, name).Scan(&existingID)
		if err == nil {
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
	}
	return errors.New("could not create guest channel route_id after collision retries")
}
