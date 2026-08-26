ALTER TABLE league_transactions DROP CONSTRAINT league_transactions_kind_check;
ALTER TABLE league_transactions ADD CONSTRAINT league_transactions_kind_check
    CHECK (kind IN ('draft', 'auction', 'trade', 'add', 'drop', 'keeper', 'rollover'));

DROP TABLE IF EXISTS league_fa_offers;
DROP TABLE IF EXISTS league_fa_windows;
ALTER TABLE league_settings DROP COLUMN IF EXISTS fa_reservation_pct;
