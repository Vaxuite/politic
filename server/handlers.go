package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type server struct {
	db *sql.DB
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// canonicalParties is the set of parties we plot on the headline chart.
// Anything else gets bucketed as "Other".
var canonicalParties = map[string]string{
	"Labour":                              "Labour",
	"Labour and Co-operative":             "Labour",
	"Conservative":                        "Conservative",
	"Liberal Democrats":                   "Liberal Democrats",
	"Liberal Democrat":                    "Liberal Democrats",
	"Reform UK":                           "Reform UK",
	"Brexit Party":                        "Reform UK",
	"UK Independence Party (UKIP)":        "UKIP",
	"UK Independence Party":               "UKIP",
	"UKIP":                                "UKIP",
	"Green Party":                         "Green",
	"Scottish Green Party":                "Green",
	"Scottish National Party (SNP)":       "SNP",
	"Scottish National Party":             "SNP",
	"Plaid Cymru - The Party of Wales":    "Plaid Cymru",
	"Plaid Cymru":                         "Plaid Cymru",
	"Democratic Unionist Party":           "DUP",
	"Sinn Féin":                           "Sinn Féin",
	"Social Democratic & Labour Party":    "SDLP",
	"Ulster Unionist Party":               "UUP",
	"Alliance":                            "Alliance",
	"Alliance - Alliance Party of Northern Ireland": "Alliance",
}

func canonical(p string) string {
	if c, ok := canonicalParties[p]; ok {
		return c
	}
	return "Other"
}

type partyShareResp struct {
	Elections []string             `json:"elections"`
	Series    []partyShareSeries   `json:"series"`
	Totals    map[string]int64     `json:"totals_by_election"`
}

type partyShareSeries struct {
	Party  string    `json:"party"`
	Shares []float64 `json:"shares"`
	Votes  []int64   `json:"votes"`
}

func (s *server) partyShare(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT polling_date, party, SUM(candidate_votes)::BIGINT AS votes
		FROM election_results
		WHERE party IS NOT NULL
		GROUP BY polling_date, party
		ORDER BY polling_date
	`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	// election (ISO date) -> canonical party -> votes
	perElection := map[string]map[string]int64{}
	dateOrder := []string{}
	seenDate := map[string]bool{}
	for rows.Next() {
		var date, party string
		var votes int64
		if err := rows.Scan(&date, &party, &votes); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if !seenDate[date] {
			seenDate[date] = true
			dateOrder = append(dateOrder, date)
		}
		key := canonical(party)
		if perElection[date] == nil {
			perElection[date] = map[string]int64{}
		}
		perElection[date][key] += votes
	}

	// Build series in a stable, sensible order.
	partyOrder := []string{"Labour", "Conservative", "Liberal Democrats", "Reform UK", "UKIP", "Green", "SNP", "Plaid Cymru", "DUP", "Sinn Féin", "SDLP", "UUP", "Alliance", "Other"}

	totalByElection := map[string]int64{}
	for d, byParty := range perElection {
		for _, v := range byParty {
			totalByElection[d] += v
		}
	}

	resp := partyShareResp{Elections: dateOrder, Totals: totalByElection}
	for _, p := range partyOrder {
		series := partyShareSeries{Party: p, Shares: make([]float64, len(dateOrder)), Votes: make([]int64, len(dateOrder))}
		any := false
		for i, d := range dateOrder {
			v := perElection[d][p]
			series.Votes[i] = v
			if total := totalByElection[d]; total > 0 {
				series.Shares[i] = float64(v) * 100.0 / float64(total)
			}
			if v > 0 {
				any = true
			}
		}
		if any {
			resp.Series = append(resp.Series, series)
		}
	}
	writeJSON(w, 200, resp)
}

type searchHit struct {
	Type    string `json:"type"` // constituency | ward | council
	Code    string `json:"code"`
	Name    string `json:"name"`
	Context string `json:"context,omitempty"` // e.g. parent council/constituency
}

type searchResp struct {
	Query string      `json:"query"`
	Hits  []searchHit `json:"hits"`
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, 200, searchResp{Query: q, Hits: []searchHit{}})
		return
	}
	pattern := "%" + strings.ToLower(q) + "%"

	rows, err := s.db.QueryContext(r.Context(), `
		WITH constituencies AS (
			SELECT DISTINCT 'constituency' AS type, pcon_code AS code, constituency_name AS name, country AS ctx
			FROM election_results
			WHERE lower(constituency_name) LIKE ? OR lower(pcon_code) LIKE ?
		),
		wards AS (
			SELECT DISTINCT 'ward' AS type, ward_code AS code, ward_name AS name, lad_name AS ctx
			FROM uk_wards
			WHERE lower(ward_name) LIKE ? OR lower(ward_code) LIKE ?
		),
		councils AS (
			SELECT DISTINCT 'council' AS type, lad_code AS code, lad_name AS name, '' AS ctx
			FROM uk_wards
			WHERE lower(lad_name) LIKE ? OR lower(lad_code) LIKE ?
		)
		SELECT type, code, name, ctx FROM constituencies
		UNION ALL SELECT type, code, name, ctx FROM councils
		UNION ALL SELECT type, code, name, ctx FROM wards
		LIMIT 50
	`, pattern, pattern, pattern, pattern, pattern, pattern)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	hits := []searchHit{}
	for rows.Next() {
		var h searchHit
		if err := rows.Scan(&h.Type, &h.Code, &h.Name, &h.Context); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		hits = append(hits, h)
	}
	writeJSON(w, 200, searchResp{Query: q, Hits: hits})
}

// /api/constituency/{code}
type constituencyResp struct {
	Code     string                 `json:"code"`
	Name     string                 `json:"name"`
	Country  string                 `json:"country"`
	Region   string                 `json:"region,omitempty"`
	Results  []electionResultBlock  `json:"results"`
	Wards    []wardInConstituency   `json:"wards"`
}

type electionResultBlock struct {
	PollingDate    string             `json:"polling_date"`
	Year           int                `json:"year"`
	Electorate     int64              `json:"electorate"`
	ValidVotes     int64              `json:"valid_votes"`
	ResultSummary  string             `json:"result_summary"`
	Majority       *int64             `json:"majority,omitempty"`
	Candidates     []candidateRow     `json:"candidates"`
}

type candidateRow struct {
	Position   int    `json:"position"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Party      string `json:"party"`
	Votes      int64  `json:"votes"`
	SittingMP  bool   `json:"sitting_mp"`
}

type wardInConstituency struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	LadCode   string  `json:"lad_code"`
	LadName   string  `json:"lad_name"`
	SplitWard bool    `json:"split_ward"`
	Primary   *bool   `json:"primary,omitempty"`
}

func pathTail(prefix, path string) string {
	return strings.TrimPrefix(path, prefix)
}

func (s *server) constituency(w http.ResponseWriter, r *http.Request) {
	code := pathTail("/api/constituency/", r.URL.Path)
	if code == "" {
		writeErr(w, 400, "constituency code required")
		return
	}

	resp := constituencyResp{Code: code, Results: []electionResultBlock{}, Wards: []wardInConstituency{}}
	err := s.db.QueryRowContext(r.Context(), `
		SELECT constituency_name, country, COALESCE(region, '')
		FROM election_results
		WHERE pcon_code = ?
		ORDER BY polling_date DESC
		LIMIT 1
	`, code).Scan(&resp.Name, &resp.Country, &resp.Region)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "constituency not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	candRows, err := s.db.QueryContext(r.Context(), `
		SELECT polling_date, election_year, electorate, valid_votes, result_summary,
			majority, position, COALESCE(candidate_given_name,''), candidate_family_name,
			COALESCE(party,'Unknown'), candidate_votes, COALESCE(sitting_mp, false)
		FROM election_results
		WHERE pcon_code = ?
		ORDER BY polling_date DESC, position ASC
	`, code)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer candRows.Close()

	blockByDate := map[string]*electionResultBlock{}
	order := []string{}
	for candRows.Next() {
		var (
			date            string
			year            int
			electorate      int64
			validVotes      int64
			summary         string
			majority        sql.NullInt64
			position        int
			given, family   string
			party           string
			votes           int64
			sittingMP       bool
		)
		if err := candRows.Scan(&date, &year, &electorate, &validVotes, &summary, &majority, &position, &given, &family, &party, &votes, &sittingMP); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		blk, ok := blockByDate[date]
		if !ok {
			blk = &electionResultBlock{
				PollingDate:   date,
				Year:          year,
				Electorate:    electorate,
				ValidVotes:    validVotes,
				ResultSummary: summary,
			}
			if majority.Valid {
				m := majority.Int64
				blk.Majority = &m
			}
			blockByDate[date] = blk
			order = append(order, date)
		}
		blk.Candidates = append(blk.Candidates, candidateRow{
			Position: position, GivenName: given, FamilyName: family,
			Party: party, Votes: votes, SittingMP: sittingMP,
		})
	}
	for _, d := range order {
		resp.Results = append(resp.Results, *blockByDate[d])
	}

	wardRows, err := s.db.QueryContext(r.Context(), `
		SELECT ward_code, ward_name, lad_code, lad_name, split_ward, is_primary_by_area
		FROM uk_wards
		WHERE pcon_code = ?
		ORDER BY ward_name
	`, code)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer wardRows.Close()
	for wardRows.Next() {
		var w0 wardInConstituency
		var primary sql.NullBool
		if err := wardRows.Scan(&w0.Code, &w0.Name, &w0.LadCode, &w0.LadName, &w0.SplitWard, &primary); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if primary.Valid {
			b := primary.Bool
			w0.Primary = &b
		}
		resp.Wards = append(resp.Wards, w0)
	}
	writeJSON(w, 200, resp)
}

// /api/ward/{code}
type wardResp struct {
	Code           string             `json:"code"`
	Name           string             `json:"name"`
	LadCode        string             `json:"lad_code"`
	LadName        string             `json:"lad_name"`
	Constituencies []wardPconLink     `json:"constituencies"`
	Local2026      *local2026Block    `json:"local_2026,omitempty"`
}

type wardPconLink struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Primary *bool  `json:"primary,omitempty"`
	Split   bool   `json:"split"`
}

type local2026Block struct {
	Seats        *int64           `json:"seats,omitempty"`
	ElectionType string           `json:"election_type"`
	SeatWinners  []string         `json:"seat_winners"`
	Votes        []partyVoteEntry `json:"votes"`
}

type partyVoteEntry struct {
	Party string  `json:"party"`
	Votes int64   `json:"votes"`
	Share float64 `json:"share"`
}

func (s *server) ward(w http.ResponseWriter, r *http.Request) {
	code := pathTail("/api/ward/", r.URL.Path)
	if code == "" {
		writeErr(w, 400, "ward code required")
		return
	}

	resp := wardResp{Code: code, Constituencies: []wardPconLink{}}
	err := s.db.QueryRowContext(r.Context(), `
		SELECT ward_name, lad_code, lad_name FROM uk_wards WHERE ward_code = ? LIMIT 1
	`, code).Scan(&resp.Name, &resp.LadCode, &resp.LadName)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "ward not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	pconRows, err := s.db.QueryContext(r.Context(), `
		SELECT pcon_code, pcon_name, split_ward, is_primary_by_area
		FROM uk_wards
		WHERE ward_code = ?
		ORDER BY is_primary_by_area DESC NULLS LAST, pcon_name
	`, code)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer pconRows.Close()
	for pconRows.Next() {
		var p wardPconLink
		var primary sql.NullBool
		if err := pconRows.Scan(&p.Code, &p.Name, &p.Split, &primary); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if primary.Valid {
			b := primary.Bool
			p.Primary = &b
		}
		resp.Constituencies = append(resp.Constituencies, p)
	}

	// 2026 local result, if any
	var (
		seats        sql.NullInt64
		electionType sql.NullString
		s1, s2, s3   sql.NullString
		vLab, vCon, vLD, vGreen, vRef, vInd, vOther sql.NullInt64
	)
	// Match by WD26 ward_code if it happens to be the same vintage, otherwise
	// fall back to a (normalised ward_name, lad_name) bridge — local_2026 uses
	// WD26 boundary codes while uk_wards uses WD25.
	err = s.db.QueryRowContext(r.Context(), `
		WITH target AS (
			SELECT ward_key(ward_name, lad_name) AS k FROM uk_wards WHERE ward_code = ? LIMIT 1
		)
		SELECT seats, election_type, seat1_winner, seat2_winner, seat3_winner,
			LAB, CON, LD, GREEN, REF, IND, other_votes
		FROM local_2026
		WHERE ward_code = ?
		   OR ward_key(ward_name, lad_name) = (SELECT k FROM target)
		LIMIT 1
	`, code, code).Scan(&seats, &electionType, &s1, &s2, &s3, &vLab, &vCon, &vLD, &vGreen, &vRef, &vInd, &vOther)
	if err == nil {
		blk := &local2026Block{}
		if seats.Valid {
			n := seats.Int64
			blk.Seats = &n
		}
		if electionType.Valid {
			blk.ElectionType = electionType.String
		}
		for _, s := range []sql.NullString{s1, s2, s3} {
			if s.Valid && s.String != "" {
				blk.SeatWinners = append(blk.SeatWinners, s.String)
			}
		}
		raw := []struct {
			party string
			v     sql.NullInt64
		}{
			{"Labour", vLab}, {"Conservative", vCon}, {"Liberal Democrats", vLD},
			{"Green", vGreen}, {"Reform UK", vRef}, {"Independent", vInd}, {"Other", vOther},
		}
		var total int64
		for _, e := range raw {
			if e.v.Valid {
				total += e.v.Int64
			}
		}
		for _, e := range raw {
			if !e.v.Valid || e.v.Int64 == 0 {
				continue
			}
			share := 0.0
			if total > 0 {
				share = float64(e.v.Int64) * 100.0 / float64(total)
			}
			blk.Votes = append(blk.Votes, partyVoteEntry{Party: e.party, Votes: e.v.Int64, Share: share})
		}
		resp.Local2026 = blk
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, resp)
}

// /api/council/{code}
type councilResp struct {
	Code      string             `json:"code"`
	Name      string             `json:"name"`
	WardCount int                `json:"ward_count"`
	Summary2026 []partyVoteEntry `json:"summary_2026"`
	Wards     []councilWardRow   `json:"wards"`
}

type councilWardRow struct {
	WardCode    string           `json:"ward_code"`
	WardName    string           `json:"ward_name"`
	Constituencies []string      `json:"constituencies"`
	Local2026   []partyVoteEntry `json:"local_2026,omitempty"`
	SeatWinners []string         `json:"seat_winners,omitempty"`
}

func (s *server) council(w http.ResponseWriter, r *http.Request) {
	code := pathTail("/api/council/", r.URL.Path)
	if code == "" {
		writeErr(w, 400, "council code required")
		return
	}

	resp := councilResp{Code: code, Wards: []councilWardRow{}, Summary2026: []partyVoteEntry{}}
	err := s.db.QueryRowContext(r.Context(), `
		SELECT lad_name FROM uk_wards WHERE lad_code = ? LIMIT 1
	`, code).Scan(&resp.Name)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "council not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	// Ward list + constituencies per ward
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT ward_code,
			any_value(ward_name)               AS ward_name,
			any_value(ward_key(ward_name, lad_name)) AS k,
			string_agg(pcon_name, ' / ' ORDER BY is_primary_by_area DESC NULLS LAST, pcon_name) AS pcons
		FROM uk_wards
		WHERE lad_code = ?
		GROUP BY ward_code
		ORDER BY ward_name
	`, code)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	// Two indexes so a local_2026 row can attach by either its (matching)
	// WD26 code or by the normalised (ward_name, lad_name) bridge.
	codeIndex := map[string]int{}
	keyIndex := map[string]int{}
	for rows.Next() {
		var wr councilWardRow
		var key, pcons string
		if err := rows.Scan(&wr.WardCode, &wr.WardName, &key, &pcons); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if pcons != "" {
			wr.Constituencies = strings.Split(pcons, " / ")
		}
		idx := len(resp.Wards)
		resp.Wards = append(resp.Wards, wr)
		codeIndex[wr.WardCode] = idx
		if key != "" {
			keyIndex[key] = idx
		}
	}
	resp.WardCount = len(resp.Wards)

	// 2026 ward-level votes by lad_name (some rows lack an ONS code entirely).
	localRows, err := s.db.QueryContext(r.Context(), `
		SELECT ward_code,
			ward_name,
			ward_key(ward_name, lad_name) AS k,
			COALESCE(seat1_winner,''), COALESCE(seat2_winner,''), COALESCE(seat3_winner,''),
			LAB, CON, LD, GREEN, REF, IND, other_votes
		FROM local_2026
		WHERE lad_name = ?
	`, resp.Name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer localRows.Close()

	var (
		totLab, totCon, totLD, totGreen, totRef, totInd, totOther int64
	)
	for localRows.Next() {
		var (
			wardCode sql.NullString
			wardName string
			key      string
			s1, s2, s3 string
			vLab, vCon, vLD, vGreen, vRef, vInd, vOther int64
		)
		if err := localRows.Scan(&wardCode, &wardName, &key, &s1, &s2, &s3, &vLab, &vCon, &vLD, &vGreen, &vRef, &vInd, &vOther); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		totLab += vLab; totCon += vCon; totLD += vLD; totGreen += vGreen
		totRef += vRef; totInd += vInd; totOther += vOther

		raw := []struct {
			party string
			v     int64
		}{
			{"Labour", vLab}, {"Conservative", vCon}, {"Liberal Democrats", vLD},
			{"Green", vGreen}, {"Reform UK", vRef}, {"Independent", vInd}, {"Other", vOther},
		}
		var total int64
		for _, e := range raw {
			total += e.v
		}
		entries := []partyVoteEntry{}
		for _, e := range raw {
			if e.v == 0 {
				continue
			}
			share := 0.0
			if total > 0 {
				share = float64(e.v) * 100.0 / float64(total)
			}
			entries = append(entries, partyVoteEntry{Party: e.party, Votes: e.v, Share: share})
		}
		winners := []string{}
		for _, w0 := range []string{s1, s2, s3} {
			if w0 != "" {
				winners = append(winners, w0)
			}
		}

		idx := -1
		if wardCode.Valid {
			if i, ok := codeIndex[wardCode.String]; ok {
				idx = i
			}
		}
		if idx < 0 && key != "" {
			if i, ok := keyIndex[key]; ok {
				idx = i
			}
		}
		if idx >= 0 {
			resp.Wards[idx].Local2026 = entries
			resp.Wards[idx].SeatWinners = winners
			continue
		}
		// No match — emit as a standalone row (e.g. new unitary wards with no
		// WD25 entry in uk_wards).
		resp.Wards = append(resp.Wards, councilWardRow{
			WardCode: "", WardName: wardName, Local2026: entries, SeatWinners: winners,
		})
	}

	totalSummary := []struct {
		party string
		v     int64
	}{
		{"Labour", totLab}, {"Conservative", totCon}, {"Liberal Democrats", totLD},
		{"Green", totGreen}, {"Reform UK", totRef}, {"Independent", totInd}, {"Other", totOther},
	}
	var summaryTotal int64
	for _, e := range totalSummary {
		summaryTotal += e.v
	}
	for _, e := range totalSummary {
		if e.v == 0 {
			continue
		}
		share := 0.0
		if summaryTotal > 0 {
			share = float64(e.v) * 100.0 / float64(summaryTotal)
		}
		resp.Summary2026 = append(resp.Summary2026, partyVoteEntry{Party: e.party, Votes: e.v, Share: share})
	}

	writeJSON(w, 200, resp)
}
