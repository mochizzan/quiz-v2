-- 0010_profile_photo.up.sql
-- Profile photos (AVT-010…AVT-012): the generated avatar file name per
-- account, or NULL when the account has no photo. The unique key is the
-- serving index and forbids two live accounts sharing one file; it includes
-- the deleted_at sentinel (schema §5.3), so soft-deleted rows never collide
-- with live ones.
-- Forward-only and idempotent: re-running this file is a no-op.

ALTER TABLE accounts
  ADD COLUMN IF NOT EXISTS avatar_name VARCHAR(37)
    CHARACTER SET ascii COLLATE ascii_bin NULL;

ALTER TABLE accounts
  ADD UNIQUE KEY IF NOT EXISTS uq_accounts_avatar (avatar_name, deleted_at);
