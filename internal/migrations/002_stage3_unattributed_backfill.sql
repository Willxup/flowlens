INSERT INTO flow_dimension (
    source_family, source_network, source_prefix_len,
    destination_family, destination_ip, destination_port,
    host, network_code, classification_code
) VALUES (0, X'', 0, 0, X'', -1, '', 0, 3)
ON CONFLICT DO NOTHING;

INSERT INTO flow_rollup (
    resolution_sec, bucket_start, dimension_id,
    upload_bytes, download_bytes, flow_observation_count
)
SELECT
    t.resolution_sec,
    t.bucket_start,
    d.id,
    t.upload_bytes,
    t.download_bytes,
    t.counter_observed_seconds
FROM traffic_rollup AS t
JOIN flow_dimension AS d
  ON d.classification_code = 3
 AND d.source_family = 0
 AND d.source_network = X''
 AND d.source_prefix_len = 0
 AND d.destination_family = 0
 AND d.destination_ip = X''
 AND d.destination_port = -1
 AND d.host = ''
 AND d.network_code = 0
WHERE (t.upload_bytes <> 0 OR t.download_bytes <> 0)
  AND NOT EXISTS (
      SELECT 1
      FROM flow_rollup AS f
      WHERE f.resolution_sec = t.resolution_sec
        AND f.bucket_start = t.bucket_start
  );
