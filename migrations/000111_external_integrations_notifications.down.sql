ALTER TABLE notification_preferences
    DROP COLUMN IF EXISTS sms_enabled,
    DROP COLUMN IF EXISTS whatsapp_enabled;

ALTER TABLE users DROP COLUMN IF EXISTS phone;
