ALTER TABLE users
    DROP COLUMN IF EXISTS ui_notifications,
    DROP COLUMN IF EXISTS ui_language,
    DROP COLUMN IF EXISTS ui_theme;
