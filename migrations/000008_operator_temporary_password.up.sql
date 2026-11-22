ALTER TABLE operators ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;
UPDATE operators SET must_change_password = true WHERE status = 'invited';
