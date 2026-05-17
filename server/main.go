package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	_ "github.com/duckdb/duckdb-go/v2"
)

func main() {
	dataDir := flag.String("data", "./data", "Path to the directory containing election_results.parquet, local_election_ward_2026.csv and uk_wards.csv")
	addr := flag.String("addr", ":8080", "Listen address")
	flag.Parse()

	absData, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatalf("resolve data dir: %v", err)
	}
	for _, f := range []string{"election_results.parquet", "local_election_ward_2026.csv", "uk_wards.csv"} {
		if _, err := os.Stat(filepath.Join(absData, f)); err != nil {
			log.Fatalf("data file missing: %s (data dir = %s)", f, absData)
		}
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		log.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	if err := setupViews(db, absData); err != nil {
		log.Fatalf("setup views: %v", err)
	}
	log.Printf("duckdb ready, data dir = %s", absData)

	srv := &server{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", srv.health)
	mux.HandleFunc("/api/elections/party-share", srv.partyShare)
	mux.HandleFunc("/api/search", srv.search)
	mux.HandleFunc("/api/constituency/", srv.constituency)
	mux.HandleFunc("/api/ward/", srv.ward)
	mux.HandleFunc("/api/council/", srv.council)

	handler := withCORS(withLogging(mux))
	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
}

func setupViews(db *sql.DB, dataDir string) error {
	stmts := []string{
		fmt.Sprintf(`CREATE OR REPLACE VIEW election_results AS
			SELECT
				"General election polling date" AS polling_date,
				CAST(strftime("General election polling date", '%%Y') AS INTEGER) AS election_year,
				"Country name"                 AS country,
				"English region name"          AS region,
				constituency_name,
				pcon_code,
				"Electorate"                   AS electorate,
				"Election valid vote count"    AS valid_votes,
				"Election result summary"      AS result_summary,
				"Candidate family name"        AS candidate_family_name,
				"Candidate given name"         AS candidate_given_name,
				"Candidate is sitting MP"      AS sitting_mp,
				"Main party name"              AS party,
				"Candidate vote count"         AS candidate_votes,
				"Majority"                     AS majority,
				"Candidate result position"    AS position
			FROM read_parquet('%s')`, filepath.Join(dataDir, "election_results.parquet")),

		fmt.Sprintf(`CREATE OR REPLACE VIEW uk_wards AS
			SELECT * FROM read_csv_auto('%s')`, filepath.Join(dataDir, "uk_wards.csv")),

		fmt.Sprintf(`CREATE OR REPLACE VIEW local_2026 AS
			SELECT
				"Ward/ County Electoral District name" AS ward_name,
				ons_ward_code                          AS ward_code,
				"ONS code source"                      AS ons_source,
				"Lower tier authority"                 AS lad_name,
				"Council (raw, from 2026 file)"        AS council_raw,
				"Seats"                                AS seats,
				"Election type"                        AS election_type,
				"Seat 1 Winner"                        AS seat1_winner,
				"Seat 2 Winner"                        AS seat2_winner,
				"Seat 3 Winner"                        AS seat3_winner,
				LAB, CON, LD, GREEN, REF, IND,
				"Other parties / candidates"           AS other_votes
			FROM read_csv_auto('%s')`, filepath.Join(dataDir, "local_election_ward_2026.csv")),
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", s[:60], err)
		}
	}
	return nil
}

func withLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.RequestURI())
		h.ServeHTTP(w, r)
	})
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}
