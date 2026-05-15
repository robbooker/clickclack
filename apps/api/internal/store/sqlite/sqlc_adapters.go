package sqlite

import (
	"database/sql"

	"github.com/openclaw/clickclack/apps/api/internal/store"
	"github.com/openclaw/clickclack/apps/api/internal/store/sqlite/storedb"
)

func sqlText(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func sqlOptionalText(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func stringFromNull(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func ptrFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullFromPtr(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sqlText(*value)
}

func storeUserFromDB(id, kind string, ownerUserID sql.NullString, displayName, handle, avatarURL, createdAt string) store.User {
	return store.User{
		ID:          id,
		Kind:        kind,
		OwnerUserID: stringFromNull(ownerUserID),
		DisplayName: displayName,
		Handle:      handle,
		AvatarURL:   avatarURL,
		CreatedAt:   createdAt,
	}
}

func storeUserFromFirstUser(row storedb.FirstUserRow) store.User {
	return storeUserFromDB(row.ID, row.Kind, row.OwnerUserID, row.DisplayName, row.Handle, row.AvatarUrl, row.CreatedAt)
}

func storeUserFromGetUser(row storedb.GetUserRow) store.User {
	return storeUserFromDB(row.ID, row.Kind, row.OwnerUserID, row.DisplayName, row.Handle, row.AvatarUrl, row.CreatedAt)
}

func storeUserFromGetSessionUser(row storedb.GetSessionUserRow) store.User {
	return storeUserFromDB(row.ID, row.Kind, row.OwnerUserID, row.DisplayName, row.Handle, row.AvatarUrl, row.CreatedAt)
}

func storeUserFromIdentityEmail(row storedb.GetUserByIdentityEmailRow) store.User {
	return storeUserFromDB(row.ID, row.Kind, row.OwnerUserID, row.DisplayName, row.Handle, row.AvatarUrl, row.CreatedAt)
}

func storeUserFromIdentityProviderSubject(row storedb.GetUserByIdentityProviderSubjectRow) store.User {
	return storeUserFromDB(row.ID, row.Kind, row.OwnerUserID, row.DisplayName, row.Handle, row.AvatarUrl, row.CreatedAt)
}

func storeMagicLinkFromDB(link storedb.AuthMagicLink) store.MagicLink {
	return store.MagicLink{
		ID:          link.ID,
		Token:       link.Token,
		Email:       link.Email,
		DisplayName: link.DisplayName,
		CreatedAt:   link.CreatedAt,
		ExpiresAt:   link.ExpiresAt,
		UsedAt:      ptrFromNull(link.UsedAt),
	}
}

func storeWorkspaceFromFirstWorkspace(row storedb.FirstWorkspaceRow) store.Workspace {
	return store.Workspace{
		ID:        row.ID,
		RouteID:   row.RouteID,
		Name:      row.Name,
		Slug:      row.Slug,
		CreatedAt: row.CreatedAt,
	}
}

func storeUploadFromGetUpload(row storedb.GetUploadRow) store.Upload {
	return store.Upload{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		OwnerID:     row.OwnerID,
		Filename:    row.Filename,
		ContentType: row.ContentType,
		ByteSize:    row.ByteSize,
		Width:       int(row.Width),
		Height:      int(row.Height),
		DurationMS:  int(row.DurationMs),
		StoragePath: row.StoragePath,
		CreatedAt:   row.CreatedAt,
	}
}

func storeChannelFromGetChannel(row storedb.GetChannelRow) store.Channel {
	return store.Channel{
		ID:          row.ID,
		RouteID:     row.RouteID,
		WorkspaceID: row.WorkspaceID,
		Name:        row.Name,
		Kind:        row.Kind,
		CreatedAt:   row.CreatedAt,
		ArchivedAt:  ptrFromNull(row.ArchivedAt),
	}
}

func storeNotificationSettingsFromDB(row storedb.GetNotificationSettingsRow) store.NotificationSettings {
	return store.NotificationSettings{
		PushoverEnabled: row.PushoverEnabled == 1,
		PushoverUserKey: row.PushoverUserKey,
	}
}

func storePushRecipient(userID, displayName, userKey string) store.PushNotificationRecipient {
	return store.PushNotificationRecipient{
		UserID:          userID,
		DisplayName:     displayName,
		PushoverUserKey: userKey,
	}
}

func storeBotTokenAuthFromDB(row storedb.GetBotTokenAuthRow) store.BotTokenAuth {
	return store.BotTokenAuth{
		User:        storeUserFromDB(row.ID, row.Kind, row.OwnerUserID, row.DisplayName, row.Handle, row.AvatarUrl, row.CreatedAt),
		TokenID:     row.TokenID,
		WorkspaceID: row.WorkspaceID,
	}
}
