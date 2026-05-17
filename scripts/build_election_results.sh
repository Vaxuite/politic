#!/usr/bin/env bash
# Build data/election_results.parquet from the candidate-level CSVs in data/.
#
# Source: https://electionresults.parliament.uk/general-elections/{1..6}/constituency-areas
# Drop the downloaded CSVs into data/ as
#   data/candidate-level-results-general-election-<DD-MM-YYYY>.csv
# and re-run; the glob picks up everything matching that pattern.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA="$ROOT/data"
GLOB="$DATA/candidate-level-results-general-election-*.csv"
OUTPUT="$DATA/election_results.parquet"

shopt -s nullglob
files=( $GLOB )
if (( ${#files[@]} == 0 )); then
  echo "No input CSVs found matching $GLOB" >&2
  exit 1
fi
echo "Combining ${#files[@]} CSV(s) -> $OUTPUT"

duckdb <<SQL
COPY (
  SELECT
    "General election polling date",
    "Country name",
    "English region name",
    "Constituency name" AS constituency_name,
    "Constituency geographic code" AS pcon_code,
    "Electorate",
    "Election valid vote count",
    "Election result summary",
    "Candidate family name",
    "Candidate given name",
    "Candidate is sitting MP",
    "Main party name",
    "Candidate vote count",
    "Majority",
    "Candidate result position"
  FROM read_csv_auto('$GLOB', union_by_name=true)
) TO '$OUTPUT' (FORMAT PARQUET);
SQL

duckdb -c "
SELECT
  COUNT(*) AS rows,
  COUNT(DISTINCT \"General election polling date\") AS elections,
  COUNT(DISTINCT pcon_code) AS constituencies
FROM '$OUTPUT';
"
