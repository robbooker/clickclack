UPDATE workspace_members wm
SET role = 'owner'
WHERE wm.role <> 'bot'
  AND wm.workspace_id = (
    SELECT id
    FROM workspaces
    WHERE slug = 'clickclack'
  )
  AND NOT EXISTS (
    SELECT 1
    FROM workspace_members owners
    WHERE owners.workspace_id = wm.workspace_id
      AND owners.role = 'owner'
  )
  AND wm.user_id = (
    SELECT first.user_id
    FROM workspace_members first
    WHERE first.workspace_id = wm.workspace_id
      AND first.role <> 'bot'
    ORDER BY first.created_at, first.user_id
    LIMIT 1
  );
