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

CREATE TABLE IF NOT EXISTS collection_runs
(
    provider LowCardinality(String),
    billing_account_id String,
    window_start DateTime64(3, 'UTC'),
    window_end DateTime64(3, 'UTC'),
    started_at DateTime64(3, 'UTC'),
    completed_at DateTime64(3, 'UTC'),
    data_ingested_at DateTime64(3, 'UTC'),
    record_count UInt64,
    has_latest_usage_end UInt8,
    latest_usage_end DateTime64(3, 'UTC'),
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(window_start)
ORDER BY (provider, billing_account_id, window_start, window_end);
