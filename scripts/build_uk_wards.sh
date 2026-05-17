#!/usr/bin/env bash
# Build data/uk_wards.csv: every UK ward with its lower-tier authority
# and parliamentary constituency. Wards that straddle multiple constituencies
# produce one row per constituency, with `is_primary_by_area` flagging the
# constituency containing the largest share of the ward's area.
#
# Coverage notes:
#   - England, Wales, Scotland: full area-based primary calculation via OS
#     Boundary-Line polygons.
#   - Northern Ireland: included in output, but NI ward boundaries are not in
#     Boundary-Line, so split NI wards get is_primary_by_area = NULL.
#
# Idempotent: re-running skips already-downloaded inputs.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA="$ROOT/data"
mkdir -p "$DATA"

LOOKUP="$DATA/wd25_pcon_lu.csv"
OSBL_DIR="$DATA/osbl"
OSBL_ZIP="$OSBL_DIR/bdline.zip"
OUTPUT="$DATA/uk_wards.csv"

# 1. ONS Ward (May 2025) -> Westminster PCON (July 2024) UK-wide lookup
if [[ ! -f "$LOOKUP" ]]; then
  echo "Downloading ONS WD25 -> PCON24 lookup..."
  curl -fsSL -o "$LOOKUP" \
    "https://hub.arcgis.com/api/v3/datasets/ac27c7721e664440adcc9e862505c8bc_0/downloads/data?format=csv&spatialRefId=4326"
fi

# 2. OS Boundary-Line shapefile (~700MB; GB only)
if [[ ! -f "$OSBL_DIR/Data/GB/district_borough_unitary_ward_region.shp" ]]; then
  mkdir -p "$OSBL_DIR"
  echo "Downloading OS Boundary-Line shapefile (~700MB)..."
  curl -fsSL -o "$OSBL_ZIP" \
    "https://api.os.uk/downloads/v1/products/BoundaryLine/downloads?format=ESRI%C2%AE+Shapefile&redirect"
  (cd "$OSBL_DIR" && unzip -q -o bdline.zip)
fi

# 3. Build the output via DuckDB spatial extension
echo "Building $OUTPUT ..."
duckdb <<SQL
INSTALL spatial;
LOAD spatial;

-- All GB ward-equivalent polygons:
--   district_borough_unitary_ward_region: English (E05) + Scottish (S13) wards
--   unitary_electoral_division_region:    English unitary (E05) + Welsh (W05) wards
CREATE OR REPLACE TEMP TABLE ward_geom AS
  SELECT CODE AS ward_cd, geom
  FROM ST_Read('$OSBL_DIR/Data/GB/district_borough_unitary_ward_region.shp')
  UNION ALL
  SELECT CODE, geom
  FROM ST_Read('$OSBL_DIR/Data/GB/unitary_electoral_division_region.shp');

CREATE OR REPLACE TEMP TABLE pcon_geom AS
  SELECT CODE AS pcon_cd, geom
  FROM ST_Read('$OSBL_DIR/Data/GB/westminster_const_region.shp');

CREATE OR REPLACE TEMP TABLE lookup AS
  SELECT WD25CD, WD25NM, LAD25CD, LAD25NM, PCON24CD, PCON24NM,
         COALESCE(SPLIT_WARD, FALSE) AS split_ward
  FROM read_csv_auto('$LOOKUP');

-- Intersection area only needed for split wards (the rest have one PCON).
-- ST_Area returns m² in EPSG:27700 (British National Grid).
CREATE OR REPLACE TEMP TABLE areas AS
  SELECT l.WD25CD, l.PCON24CD, ST_Area(ST_Intersection(w.geom, p.geom)) AS area_m2
  FROM lookup l
  JOIN ward_geom w ON w.ward_cd = l.WD25CD
  JOIN pcon_geom p ON p.pcon_cd = l.PCON24CD
  WHERE l.split_ward = TRUE;

COPY (
  WITH joined AS (
    SELECT
      l.WD25CD   AS ward_code,
      l.WD25NM   AS ward_name,
      l.LAD25CD  AS lad_code,
      l.LAD25NM  AS lad_name,
      l.PCON24CD AS pcon_code,
      l.PCON24NM AS pcon_name,
      l.split_ward,
      a.area_m2
    FROM lookup l
    LEFT JOIN areas a ON a.WD25CD = l.WD25CD AND a.PCON24CD = l.PCON24CD
  )
  SELECT
    ward_code, ward_name, lad_code, lad_name, pcon_code, pcon_name,
    split_ward,
    CASE
      WHEN split_ward = FALSE THEN TRUE
      WHEN area_m2 IS NULL    THEN NULL            -- NI: no GB geometry
      ELSE area_m2 = MAX(area_m2) OVER (PARTITION BY ward_code)
    END AS is_primary_by_area,
    ROUND(area_m2 / 1e6, 4) AS intersection_area_km2
  FROM joined
  ORDER BY ward_code, is_primary_by_area DESC NULLS LAST, intersection_area_km2 DESC NULLS LAST
) TO '$OUTPUT' (HEADER, DELIMITER ',');
SQL

echo "Wrote $(wc -l < "$OUTPUT") lines to $OUTPUT"
