-- name: RolesListRoles :many
SELECT id, name, description, created_at, updated_at 
FROM roles 
ORDER BY 
  CASE WHEN @sort_by::text = 'name' AND @sort_dir::text = 'desc' THEN name END DESC,
  CASE WHEN @sort_by::text = 'name' AND @sort_dir::text != 'desc' THEN name END ASC,
  CASE WHEN @sort_by::text = 'created_at' AND @sort_dir::text = 'desc' THEN created_at END DESC,
  CASE WHEN @sort_by::text = 'created_at' AND @sort_dir::text != 'desc' THEN created_at END ASC,
  CASE WHEN @sort_by::text = 'id' AND @sort_dir::text = 'desc' THEN id END DESC,
  CASE WHEN @sort_by::text = 'id' AND @sort_dir::text != 'desc' THEN id END ASC,
  id ASC;

-- name: RolesCreateRole :one
INSERT INTO roles (
    name, 
    description, 
    created_at, 
    updated_at
) VALUES (
    $1, $2, NOW(), NOW()
) RETURNING id, name, description, created_at, updated_at;
