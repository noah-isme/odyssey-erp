ALTER TABLE fixed_asset_categories
    ADD COLUMN disposal_gain_account_id BIGINT REFERENCES accounts(id),
    ADD COLUMN disposal_loss_account_id BIGINT REFERENCES accounts(id),
    ADD COLUMN cash_proceeds_account_id BIGINT REFERENCES accounts(id);
