#!/usr/bin/env bash
# Normalise data/2026_local_election.csv into the same schema as
# data/local_election_ward.csv (the 2025 file), with ONS ward codes
# resolved by joining on ward name + council against ONS lookups.
#
# Output: data/local_election_ward_2026.parquet
#
# Every ward in the 2026 raw file is included (currently 2,554 rows).
# Wards we can't resolve to an ONS code (the brand-new Surrey unitary
# wards) are still emitted with ONS ward code = NULL.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA="$ROOT/data"
SRC="$DATA/2026_local_election.csv"
OUTPUT="$DATA/local_election_ward_2026.parquet"

[[ -f "$SRC" ]] || { echo "Missing $SRC" >&2; exit 1; }

# ONS lookups (small, auto-downloaded if missing)
WD25="$DATA/wd25_pcon_lu.csv"
CED25="$DATA/wd25_ced25_lu.csv"
WD26="$DATA/wd26_lad26_lu.csv"

dl() { [[ -f "$2" ]] || { echo "Downloading $(basename "$2")..."; curl -fsSL -o "$2" "$1"; } }
dl "https://hub.arcgis.com/api/v3/datasets/ac27c7721e664440adcc9e862505c8bc_0/downloads/data?format=csv&spatialRefId=4326" "$WD25"
dl "https://hub.arcgis.com/api/v3/datasets/4263359a579045e79b4a013d857894cd_0/downloads/data?format=csv&spatialRefId=4326" "$CED25"
dl "https://hub.arcgis.com/api/v3/datasets/7447015a1f2f4332807d7341a636f95d_0/downloads/data?format=csv&spatialRefId=4326" "$WD26"

echo "Building $OUTPUT ..."
duckdb <<SQL
-- Raw 2026 rows. The file has 4 banner/header rows above the data.
-- Columns 0..8 are metadata; 9..32 are top-candidate vote counts per party
-- (in this column order: RFM, LAB, CON, LDM, GRN, IND, Localist, TUSC,
--  Workers Party, SDP, Aspire, GYF, All Others, MAX, Christian Peoples
--  Alliance, Heritage, Your Party, Advance UK, Rejoin EU, MRLP, Communist
--  Party of Britain, UKIP, Other, two more zero columns).
CREATE OR REPLACE TEMP TABLE raw AS
SELECT
  row_number() OVER () AS rn,
  trim(column001) AS ward_name,
  trim(column002) AS council_name,
  TRY_CAST(column003 AS INTEGER) AS seats_up,
  column004 AS seat1_winner,
  column005 AS seat2_winner,
  column006 AS seat3_winner,
  -- vote columns (treated as varchar; some are blank)
  TRY_CAST(column009 AS INTEGER) AS v_rfm,
  TRY_CAST(column010 AS INTEGER) AS v_lab,
  TRY_CAST(column011 AS INTEGER) AS v_con,
  TRY_CAST(column012 AS INTEGER) AS v_ldm,
  TRY_CAST(column013 AS INTEGER) AS v_grn,
  TRY_CAST(column014 AS INTEGER) AS v_ind,
  TRY_CAST(column015 AS INTEGER) AS v_localist,
  TRY_CAST(column016 AS INTEGER) AS v_tusc,
  TRY_CAST(column017 AS INTEGER) AS v_wpb,
  TRY_CAST(column018 AS INTEGER) AS v_sdp,
  TRY_CAST(column019 AS INTEGER) AS v_aspire,
  TRY_CAST(column020 AS INTEGER) AS v_gyf,
  TRY_CAST(column021 AS INTEGER) AS v_all_other_a,
  -- column022 = MAX (skip)
  TRY_CAST(column023 AS INTEGER) AS v_cpa,
  TRY_CAST(column024 AS INTEGER) AS v_heritage,
  TRY_CAST(column025 AS INTEGER) AS v_your_party,
  TRY_CAST(column026 AS INTEGER) AS v_advance_uk,
  TRY_CAST(column027 AS INTEGER) AS v_rejoin,
  TRY_CAST(column028 AS INTEGER) AS v_mrlp,
  TRY_CAST(column029 AS INTEGER) AS v_cpb,
  TRY_CAST(column030 AS INTEGER) AS v_ukip,
  TRY_CAST(column031 AS INTEGER) AS v_other
FROM read_csv('$SRC', header=false, skip=4, all_varchar=true, ignore_errors=true, null_padding=true)
WHERE column001 IS NOT NULL AND trim(column001) <> '';

-- Normalisation helper for join keys
CREATE OR REPLACE MACRO norm(s) AS
  regexp_replace(regexp_replace(lower(trim(s)), '\s+(ward|ed|electoral division|division)\s*\$', ''),
                 '[^a-z0-9]+', '', 'g');

-- Three lookup tables that between them cover ~all wards
CREATE OR REPLACE TEMP TABLE wd25 AS
SELECT DISTINCT WD25CD AS code, WD25NM AS nm, LAD25CD AS lad_cd, LAD25NM AS lad_nm
FROM read_csv_auto('$WD25');

CREATE OR REPLACE TEMP TABLE ced25 AS
SELECT DISTINCT CED25CD AS code, CED25NM AS nm, CTY25CD AS cty_cd, CTY25NM AS cty_nm, LAD25CD AS lad_cd, LAD25NM AS lad_nm
FROM read_csv_auto('$CED25');

CREATE OR REPLACE TEMP TABLE wd26 AS
SELECT DISTINCT WD26CD AS code, WD26NM AS nm, LAD26CD AS lad_cd, LAD26NM AS lad_nm
FROM read_csv_auto('$WD26');

-- Resolve a single ONS code per (ward, council). Prefer WD25 (the most
-- comprehensive lookup that also includes PCON); fall back to CED25 (county
-- electoral divisions) then WD26 (May 2026 boundary changes).
CREATE OR REPLACE TEMP TABLE resolved AS
WITH candidates AS (
  SELECT r.rn, w.code, w.lad_cd, w.lad_nm, 'WD25' AS source, 1 AS prio
  FROM raw r JOIN wd25 w
    ON norm(w.nm) = norm(r.ward_name) AND norm(w.lad_nm) = norm(r.council_name)
  UNION ALL
  SELECT r.rn, c.code, c.lad_cd, c.lad_nm, 'CED25', 2
  FROM raw r JOIN ced25 c
    ON norm(c.nm) = norm(r.ward_name)
   AND (norm(c.cty_nm) = norm(r.council_name) OR norm(c.lad_nm) = norm(r.council_name))
  UNION ALL
  SELECT r.rn, w.code, w.lad_cd, w.lad_nm, 'WD26', 3
  FROM raw r JOIN wd26 w
    ON norm(w.nm) = norm(r.ward_name) AND norm(w.lad_nm) = norm(r.council_name)
)
SELECT DISTINCT ON (rn) rn, code, lad_cd, lad_nm, source
FROM candidates
ORDER BY rn, prio;

-- Final output, in the same column order as data/local_election_ward.csv
COPY (
  SELECT
    r.ward_name                              AS "Ward/ County Electoral District name",
    res.code                                 AS "ons_ward_code",
    res.source                               AS "ONS code source",
    res.lad_nm                               AS "Lower tier authority",
    r.council_name                           AS "Council (raw, from 2026 file)",
    r.seats_up                               AS "Seats",
    'All'                                    AS "Election type",
    r.seat1_winner                           AS "Seat 1 Winner",
    r.seat2_winner                           AS "Seat 2 Winner",
    r.seat3_winner                           AS "Seat 3 Winner",
    COALESCE(r.v_lab, 0)                     AS "LAB",
    COALESCE(r.v_con, 0)                     AS "CON",
    COALESCE(r.v_ldm, 0)                     AS "LD",
    COALESCE(r.v_grn, 0)                     AS "GREEN",
    COALESCE(r.v_rfm, 0)                     AS "REF",
    COALESCE(r.v_ind, 0)                     AS "IND",
    COALESCE(r.v_localist,0) + COALESCE(r.v_tusc,0)
      + COALESCE(r.v_wpb,0) + COALESCE(r.v_sdp,0) + COALESCE(r.v_aspire,0)
      + COALESCE(r.v_gyf,0) + COALESCE(r.v_all_other_a,0) + COALESCE(r.v_cpa,0)
      + COALESCE(r.v_heritage,0) + COALESCE(r.v_your_party,0)
      + COALESCE(r.v_advance_uk,0) + COALESCE(r.v_rejoin,0) + COALESCE(r.v_mrlp,0)
      + COALESCE(r.v_cpb,0) + COALESCE(r.v_ukip,0) + COALESCE(r.v_other,0)
                                             AS "Other parties / candidates"
  FROM raw r
  LEFT JOIN resolved res USING (rn)
  ORDER BY r.council_name, r.ward_name
) TO '$OUTPUT' (FORMAT PARQUET);
SQL

# Report row counts and any unresolved (no ONS code)
duckdb -c "
SELECT
  COUNT(*) AS rows,
  COUNT(\"ons_ward_code\") AS resolved,
  COUNT(*) - COUNT(\"ons_ward_code\") AS unresolved
FROM '$OUTPUT';
SELECT \"Lower tier authority\", COUNT(*) AS missing_code
FROM '$OUTPUT' WHERE \"ons_ward_code\" IS NULL
GROUP BY 1 ORDER BY 2 DESC;
"
echo "Wrote $OUTPUT"
