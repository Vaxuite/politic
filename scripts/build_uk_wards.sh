#!/usr/bin/env bash
# Build data/uk_wards.csv: every UK ward with its lower-tier authority
# and parliamentary constituency. Wards that straddle multiple constituencies
# produce one row per constituency, with `is_primary_by_area` flagging the
# constituency containing the largest share of the ward's area.
#
# Vintage: May 2026 wards (WD26). Wards whose codes changed in the May 2026
# boundary review (e.g. Gateshead "Crawcrook & Greenside" = E05016551,
# previously E05001072 = "Crawcrook and Greenside") appear under their new
# codes; unchanged wards keep their existing WD25 codes.
#
# Coverage notes:
#   - GB (England, Wales, Scotland): full area-based primary calculation via
#     OS Boundary-Line polygons. PCON name comes from the shapefile.
#   - Northern Ireland: included via the ONS UK-wide ward list, but NI ward
#     boundaries are not in Boundary-Line, so PCON is filled in from the
#     WD25 -> PCON24 lookup (NI ward codes didn't change for May 2026), with
#     is_primary_by_area = NULL on split wards.
#
# Idempotent: re-running skips already-downloaded inputs.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA="$ROOT/data"
mkdir -p "$DATA"

LOOKUP_26="$DATA/wd26_lad26_lu.csv"   # UK-wide ward -> LAD (May 2026 vintage)
LOOKUP_25="$DATA/wd25_pcon_lu.csv"    # WD25 -> PCON24 (used as NI fallback only)
OSBL_DIR="$DATA/osbl"
OSBL_ZIP="$OSBL_DIR/bdline.zip"
OUTPUT="$DATA/uk_wards.parquet"

# 1. ONS Ward (May 2026) -> LAD (May 2026) UK-wide lookup
if [[ ! -f "$LOOKUP_26" ]]; then
  echo "Downloading ONS WD26 -> LAD26 lookup..."
  curl -fsSL -o "$LOOKUP_26" \
    "https://hub.arcgis.com/api/v3/datasets/7447015a1f2f4332807d7341a636f95d_0/downloads/data?format=csv&spatialRefId=4326"
fi

# 2. ONS Ward (May 2025) -> PCON24 lookup, used only as NI fallback.
if [[ ! -f "$LOOKUP_25" ]]; then
  echo "Downloading ONS WD25 -> PCON24 lookup (NI fallback)..."
  curl -fsSL -o "$LOOKUP_25" \
    "https://hub.arcgis.com/api/v3/datasets/ac27c7721e664440adcc9e862505c8bc_0/downloads/data?format=csv&spatialRefId=4326"
fi

# 3. OS Boundary-Line shapefile (~700MB; GB only). The May 2026 release
#    contains the new WD26 boundaries for councils that reorganised.
if [[ ! -f "$OSBL_DIR/Data/GB/district_borough_unitary_ward_region.shp" ]]; then
  mkdir -p "$OSBL_DIR"
  echo "Downloading OS Boundary-Line shapefile (~700MB)..."
  curl -fsSL -o "$OSBL_ZIP" \
    "https://api.os.uk/downloads/v1/products/BoundaryLine/downloads?format=ESRI%C2%AE+Shapefile&redirect"
  (cd "$OSBL_DIR" && unzip -q -o bdline.zip)
fi

# 4. Build the output via DuckDB spatial extension.
echo "Building $OUTPUT (this takes a few minutes — spatial intersection over ~9k wards x ~570 PCONs)..."
duckdb <<SQL
INSTALL spatial;
LOAD spatial;

-- GB ward polygons. Boundary-Line splits these across two shapefiles:
--   district_borough_unitary_ward_region: English (E05) + Scottish (S13) wards
--   unitary_electoral_division_region:    English unitary (E05) + Welsh (W05) wards
CREATE OR REPLACE TEMP TABLE ward_geom AS
  SELECT CODE AS ward_code, NAME AS ward_name_geom, geom
  FROM ST_Read('$OSBL_DIR/Data/GB/district_borough_unitary_ward_region.shp')
  UNION ALL
  SELECT CODE, NAME, geom
  FROM ST_Read('$OSBL_DIR/Data/GB/unitary_electoral_division_region.shp');

-- Westminster constituency polygons (PCON24 — no boundary review since 2024).
-- Strip the " Boro Const" / " Co Const" / " Burgh Const" / " County Const"
-- suffix that Boundary-Line tacks onto NAME, so it matches the clean names
-- used in election_results.parquet ("Blaydon and Consett", not
-- "Blaydon and Consett Co Const").
CREATE OR REPLACE TEMP TABLE pcon_geom AS
  SELECT
    CODE AS pcon_code,
    regexp_replace(NAME, ' (Boro Const|Co Const|Burgh Const|County Const)$', '') AS pcon_name,
    geom
  FROM ST_Read('$OSBL_DIR/Data/GB/westminster_const_region.shp');

-- UK-wide canonical ward list (May 2026 vintage).
CREATE OR REPLACE TEMP TABLE wards_uk AS
  SELECT DISTINCT WD26CD AS ward_code, WD26NM AS ward_name,
                  LAD26CD AS lad_code, LAD26NM AS lad_name
  FROM read_csv_auto('$LOOKUP_26');

-- NI PCON fallback (WD25 → PCON24; NI ward codes are stable).
CREATE OR REPLACE TEMP TABLE wd25_pcon AS
  SELECT WD25CD AS ward_code, PCON24CD AS pcon_code, PCON24NM AS pcon_name
  FROM read_csv_auto('$LOOKUP_25');

-- Spatial intersection: every (ward, pcon) pair with non-sliver overlap.
-- ST_Area returns m² in EPSG:27700 (British National Grid).
CREATE OR REPLACE TEMP TABLE areas AS
  WITH raw AS (
    SELECT
      w.ward_code,
      p.pcon_code,
      p.pcon_name,
      ST_Area(ST_Intersection(w.geom, p.geom)) AS area_m2
    FROM ward_geom w
    JOIN pcon_geom p ON ST_Intersects(w.geom, p.geom)
  )
  SELECT * FROM raw WHERE area_m2 > 0.5;  -- drop slivers (<0.5 m²)

COPY (
  WITH gb AS (
    SELECT
      u.ward_code, u.ward_name, u.lad_code, u.lad_name,
      a.pcon_code, a.pcon_name,
      a.area_m2,
      (COUNT(*) OVER (PARTITION BY u.ward_code)) > 1 AS split_ward
    FROM wards_uk u
    JOIN areas a USING (ward_code)
  ),
  ni AS (
    SELECT
      u.ward_code, u.ward_name, u.lad_code, u.lad_name,
      p.pcon_code, p.pcon_name,
      CAST(NULL AS DOUBLE) AS area_m2,
      FALSE AS split_ward
    FROM wards_uk u
    LEFT JOIN wd25_pcon p USING (ward_code)
    WHERE u.ward_code NOT IN (SELECT ward_code FROM areas)
      AND p.pcon_code IS NOT NULL
  ),
  all_rows AS (
    SELECT * FROM gb UNION ALL SELECT * FROM ni
  )
  SELECT
    ward_code, ward_name, lad_code, lad_name, pcon_code, pcon_name,
    split_ward,
    CASE
      WHEN area_m2 IS NULL    THEN NULL                                          -- NI: no GB geometry
      WHEN split_ward = FALSE THEN TRUE                                          -- only one PCON: it's primary
      ELSE area_m2 = MAX(area_m2) OVER (PARTITION BY ward_code)
    END AS is_primary_by_area,
    ROUND(area_m2 / 1e6, 4) AS intersection_area_km2
  FROM all_rows
  ORDER BY ward_code, is_primary_by_area DESC NULLS LAST, intersection_area_km2 DESC NULLS LAST
) TO '$OUTPUT' (FORMAT PARQUET);
SQL

duckdb -c "
SELECT
  COUNT(*)                                    AS rows,
  COUNT(DISTINCT ward_code)                   AS wards,
  SUM(CASE WHEN split_ward THEN 1 ELSE 0 END) AS split_rows,
  SUM(CASE WHEN pcon_code IS NULL THEN 1 ELSE 0 END) AS no_pcon
FROM '$OUTPUT';
"
echo "Wrote $OUTPUT"
