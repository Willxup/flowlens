CREATE TABLE schema_migration (
    version INTEGER PRIMARY KEY CHECK (version > 0),
    applied_at INTEGER NOT NULL CHECK (applied_at > 0),
    checksum TEXT NOT NULL CHECK (length(checksum) = 64)
) STRICT;

CREATE TABLE traffic_rollup (
    resolution_sec INTEGER NOT NULL CHECK (resolution_sec IN (10, 60, 1800, 3600, 86400)),
    bucket_start INTEGER NOT NULL,
    bucket_end INTEGER NOT NULL CHECK (bucket_end > bucket_start),
    upload_bytes INTEGER NOT NULL CHECK (upload_bytes >= 0),
    download_bytes INTEGER NOT NULL CHECK (download_bytes >= 0),
    recovered_upload_bytes INTEGER NOT NULL CHECK (recovered_upload_bytes >= 0 AND recovered_upload_bytes <= upload_bytes),
    recovered_download_bytes INTEGER NOT NULL CHECK (recovered_download_bytes >= 0 AND recovered_download_bytes <= download_bytes),
    speed_upload_sample_sum INTEGER NOT NULL CHECK (speed_upload_sample_sum >= 0),
    speed_download_sample_sum INTEGER NOT NULL CHECK (speed_download_sample_sum >= 0),
    speed_sample_count INTEGER NOT NULL CHECK (speed_sample_count >= 0),
    peak_upload_bytes_per_second INTEGER NOT NULL CHECK (peak_upload_bytes_per_second >= 0),
    peak_download_bytes_per_second INTEGER NOT NULL CHECK (peak_download_bytes_per_second >= 0),
    peak_upload_at INTEGER CHECK (peak_upload_at IS NULL OR (peak_upload_at >= bucket_start AND peak_upload_at < bucket_end)),
    peak_download_at INTEGER CHECK (peak_download_at IS NULL OR (peak_download_at >= bucket_start AND peak_download_at < bucket_end)),
    counter_observed_seconds INTEGER NOT NULL CHECK (counter_observed_seconds >= 0),
    attribution_observed_seconds INTEGER NOT NULL CHECK (attribution_observed_seconds >= 0),
    active_connections_sum INTEGER NOT NULL CHECK (active_connections_sum >= 0),
    active_connections_samples INTEGER NOT NULL CHECK (active_connections_samples >= 0),
    active_connections_max INTEGER NOT NULL CHECK (active_connections_max >= 0),
    memory_bytes_sum INTEGER NOT NULL CHECK (memory_bytes_sum >= 0),
    memory_samples INTEGER NOT NULL CHECK (memory_samples >= 0),
    memory_bytes_max INTEGER NOT NULL CHECK (memory_bytes_max >= 0),
    unattributed_upload_bytes INTEGER NOT NULL CHECK (unattributed_upload_bytes >= 0 AND unattributed_upload_bytes <= upload_bytes),
    unattributed_download_bytes INTEGER NOT NULL CHECK (unattributed_download_bytes >= 0 AND unattributed_download_bytes <= download_bytes),
    reset_count INTEGER NOT NULL CHECK (reset_count >= 0),
    quality_flags INTEGER NOT NULL CHECK (quality_flags >= 0),
    PRIMARY KEY (resolution_sec, bucket_start)
) STRICT, WITHOUT ROWID;

CREATE TABLE flow_dimension (
    id INTEGER PRIMARY KEY,
    source_family INTEGER NOT NULL CHECK (source_family IN (0, 4, 6)),
    source_network BLOB NOT NULL,
    source_prefix_len INTEGER NOT NULL CHECK (source_prefix_len BETWEEN 0 AND 128),
    destination_family INTEGER NOT NULL CHECK (destination_family IN (0, 4, 6)),
    destination_ip BLOB NOT NULL,
    destination_port INTEGER NOT NULL CHECK (destination_port BETWEEN -1 AND 65535),
    host TEXT NOT NULL CHECK (length(CAST(host AS BLOB)) <= 253),
    network_code INTEGER NOT NULL CHECK (network_code BETWEEN 0 AND 2),
    classification_code INTEGER NOT NULL CHECK (classification_code BETWEEN 1 AND 3),
    CHECK (
        (source_family = 0 AND length(source_network) = 0 AND source_prefix_len = 0) OR
        (source_family = 4 AND length(source_network) = 4 AND source_prefix_len BETWEEN 0 AND 32) OR
        (source_family = 6 AND length(source_network) = 16 AND source_prefix_len BETWEEN 0 AND 128)
    ),
    CHECK (
        (destination_family = 0 AND length(destination_ip) = 0) OR
        (destination_family = 4 AND length(destination_ip) = 4) OR
        (destination_family = 6 AND length(destination_ip) = 16)
    ),
    CHECK (
        classification_code = 1 OR
        (source_family = 0 AND length(source_network) = 0 AND source_prefix_len = 0 AND
         destination_family = 0 AND length(destination_ip) = 0 AND destination_port = -1 AND
         host = '' AND network_code = 0)
    ),
    UNIQUE (
        source_family,
        source_network,
        source_prefix_len,
        destination_family,
        destination_ip,
        destination_port,
        host,
        network_code,
        classification_code
    )
) STRICT;

CREATE TABLE flow_rollup (
    resolution_sec INTEGER NOT NULL CHECK (resolution_sec IN (10, 60, 1800, 3600, 86400)),
    bucket_start INTEGER NOT NULL,
    dimension_id INTEGER NOT NULL REFERENCES flow_dimension(id) ON DELETE RESTRICT,
    upload_bytes INTEGER NOT NULL CHECK (upload_bytes >= 0),
    download_bytes INTEGER NOT NULL CHECK (download_bytes >= 0),
    flow_observation_count INTEGER NOT NULL CHECK (flow_observation_count >= 0),
    PRIMARY KEY (resolution_sec, bucket_start, dimension_id)
) STRICT, WITHOUT ROWID;

CREATE TABLE runtime_session (
    id TEXT PRIMARY KEY NOT NULL CHECK (length(CAST(id AS BLOB)) BETWEEN 1 AND 128),
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    start_reason TEXT NOT NULL CHECK (length(CAST(start_reason AS BLOB)) BETWEEN 1 AND 64),
    end_reason TEXT CHECK (end_reason IS NULL OR length(CAST(end_reason AS BLOB)) BETWEEN 1 AND 64),
    last_upload_total INTEGER NOT NULL CHECK (last_upload_total >= 0),
    last_download_total INTEGER NOT NULL CHECK (last_download_total >= 0),
    last_seen_at INTEGER NOT NULL CHECK (last_seen_at >= started_at),
    sing_box_version TEXT NOT NULL CHECK (length(CAST(sing_box_version AS BLOB)) BETWEEN 1 AND 256),
    host_boot_id TEXT CHECK (host_boot_id IS NULL OR length(CAST(host_boot_id AS BLOB)) BETWEEN 1 AND 128),
    data_gap_before_seconds INTEGER NOT NULL CHECK (data_gap_before_seconds >= 0),
    CHECK (
        (ended_at IS NULL AND end_reason IS NULL) OR
        (ended_at IS NOT NULL AND ended_at >= started_at AND end_reason IS NOT NULL)
    )
) STRICT;

CREATE TABLE collector_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    runtime_session_id TEXT NOT NULL REFERENCES runtime_session(id) ON DELETE RESTRICT,
    last_upload_total INTEGER NOT NULL CHECK (last_upload_total >= 0),
    last_download_total INTEGER NOT NULL CHECK (last_download_total >= 0),
    last_sample_at INTEGER NOT NULL,
    last_batch_id TEXT NOT NULL CHECK (length(CAST(last_batch_id AS BLOB)) BETWEEN 1 AND 128),
    bucket_timezone TEXT NOT NULL CHECK (length(CAST(bucket_timezone AS BLOB)) BETWEEN 1 AND 128)
) STRICT;

CREATE TABLE service_label (
    id INTEGER PRIMARY KEY,
    label_type TEXT NOT NULL CHECK (label_type IN ('host', 'endpoint')),
    match_value TEXT NOT NULL CHECK (length(CAST(match_value AS BLOB)) BETWEEN 1 AND 512),
    display_name TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 64),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    UNIQUE (label_type, match_value)
) STRICT;

CREATE TABLE quality_event (
    id INTEGER PRIMARY KEY,
    batch_id TEXT NOT NULL CHECK (length(CAST(batch_id AS BLOB)) BETWEEN 1 AND 128),
    code TEXT NOT NULL CHECK (length(CAST(code AS BLOB)) BETWEEN 1 AND 64),
    started_at INTEGER NOT NULL,
    ended_at INTEGER CHECK (ended_at IS NULL OR ended_at >= started_at),
    flags INTEGER NOT NULL CHECK (flags >= 0),
    detail TEXT NOT NULL CHECK (length(CAST(detail AS BLOB)) <= 4096)
) STRICT;

CREATE TABLE maintenance_run (
    id INTEGER PRIMARY KEY,
    operation TEXT NOT NULL CHECK (length(CAST(operation AS BLOB)) BETWEEN 1 AND 64),
    started_at INTEGER NOT NULL,
    ended_at INTEGER CHECK (ended_at IS NULL OR ended_at >= started_at),
    deleted_rows INTEGER NOT NULL CHECK (deleted_rows >= 0),
    database_bytes INTEGER NOT NULL CHECK (database_bytes >= 0),
    wal_bytes INTEGER NOT NULL CHECK (wal_bytes >= 0),
    error TEXT CHECK (error IS NULL OR length(CAST(error AS BLOB)) <= 4096)
) STRICT;

CREATE INDEX quality_event_started_at_idx ON quality_event(started_at);
CREATE INDEX maintenance_run_started_at_idx ON maintenance_run(started_at);
CREATE INDEX runtime_session_started_at_idx ON runtime_session(started_at);
