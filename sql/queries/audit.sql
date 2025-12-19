-- name: AuditTimelineWindow :many
SELECT a.occurred_at AS at,
       COALESCE(u.email, a.actor_id::text) AS actor,
       a.action,
       a.entity,
       a.entity_id::text AS entity_id,
       je.number AS journal_no,
       p.code AS period_code
FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_id
LEFT JOIN source_links sl
       ON sl.module = a.entity
      AND sl.ref_id::text = a.entity_id::text
LEFT JOIN journal_entries je
       ON (a.entity = 'journal_entries' AND je.id::text = a.entity_id::text)
       OR (sl.je_id = je.id)
LEFT JOIN periods p ON p.id = je.period_id
WHERE a.occurred_at BETWEEN sqlc.arg(from_at) AND sqlc.arg(to_at)
  AND (sqlc.narg(actor)::text IS NULL OR a.actor_id::text = sqlc.narg(actor)::text)
  AND (sqlc.narg(entity)::text IS NULL OR a.entity = sqlc.narg(entity)::text)
  AND (sqlc.narg(action)::text IS NULL OR a.action = sqlc.narg(action)::text)
ORDER BY a.occurred_at DESC
LIMIT sqlc.arg(limit_rows) OFFSET sqlc.arg(offset_rows);

-- name: AuditTimelineAll :many
SELECT a.occurred_at AS at,
       COALESCE(u.email, a.actor_id::text) AS actor,
       a.action,
       a.entity,
       a.entity_id::text AS entity_id,
       je.number AS journal_no,
       p.code AS period_code
FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_id
LEFT JOIN source_links sl
       ON sl.module = a.entity
      AND sl.ref_id::text = a.entity_id::text
LEFT JOIN journal_entries je
       ON (a.entity = 'journal_entries' AND je.id::text = a.entity_id::text)
       OR (sl.je_id = je.id)
LEFT JOIN periods p ON p.id = je.period_id
WHERE a.occurred_at BETWEEN sqlc.arg(from_at) AND sqlc.arg(to_at)
  AND (sqlc.narg(actor)::text IS NULL OR a.actor_id::text = sqlc.narg(actor)::text)
  AND (sqlc.narg(entity)::text IS NULL OR a.entity = sqlc.narg(entity)::text)
  AND (sqlc.narg(action)::text IS NULL OR a.action = sqlc.narg(action)::text)
ORDER BY a.occurred_at DESC;
