-- Per-user quiet hours, expressed in the user's own timezone. NULL means the user has
-- no quiet window; quiet_hours_end at or before quiet_hours_start wraps past midnight.
ALTER TABLE users
    ADD COLUMN quiet_hours_start TIME,
    ADD COLUMN quiet_hours_end   TIME,
    ADD CONSTRAINT quiet_hours_both_or_neither
        CHECK ((quiet_hours_start IS NULL) = (quiet_hours_end IS NULL));
