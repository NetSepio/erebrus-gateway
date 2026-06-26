-- Optional sub-region label (e.g. east, west) within a country region (US).
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS zone text;

CREATE INDEX IF NOT EXISTS idx_nodes_status_region_zone ON nodes (status, region, zone);