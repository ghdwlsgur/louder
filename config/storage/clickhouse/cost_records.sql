CREATE TABLE IF NOT EXISTS cost_records
(
    provider LowCardinality(String),
    billing_account_id String,
    source_record_id String,
    cost_basis LowCardinality(String),
    amount String,
    currency FixedString(3),
    usage_start DateTime64(3, 'UTC'),
    usage_end DateTime64(3, 'UTC'),
    version UInt64,
    ingested_at DateTime64(6, 'UTC')
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(usage_start)
ORDER BY (provider, billing_account_id, source_record_id);
