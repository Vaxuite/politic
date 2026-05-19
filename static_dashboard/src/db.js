// Browser-side analytical query layer. Initialises DuckDB WASM in a
// worker, registers the three data files over HTTP, and exposes the
// same set of queries the Go service did. All public methods return
// plain JS objects (no Arrow Tables, no BigInts).

import * as duckdb from '@duckdb/duckdb-wasm'

let initPromise = null

async function init() {
  const bundles = duckdb.getJsDelivrBundles()
  const bundle = await duckdb.selectBundle(bundles)
  // selectBundle returns module URLs pointing at jsdelivr; we still need
  // to spin up the worker ourselves. The worker URL has to be loaded as
  // a same-origin blob so that the worker can import the wasm.
  const worker_url = URL.createObjectURL(
    new Blob([`importScripts("${bundle.mainWorker}");`], { type: 'text/javascript' })
  )
  const worker = new Worker(worker_url)
  const logger = new duckdb.ConsoleLogger(duckdb.LogLevel.WARNING)
  const db = new duckdb.AsyncDuckDB(logger, worker)
  await db.instantiate(bundle.mainModule, bundle.pthreadWorker)
  URL.revokeObjectURL(worker_url)

  const base = import.meta.env.BASE_URL.replace(/\/$/, '')
  const url = (f) => `${window.location.origin}${base}/data/${f}`

  await db.registerFileURL('election_results.parquet',     url('election_results.parquet'),     duckdb.DuckDBDataProtocol.HTTP, false)
  await db.registerFileURL('uk_wards.parquet',             url('uk_wards.parquet'),             duckdb.DuckDBDataProtocol.HTTP, false)
  await db.registerFileURL('local_election_ward_2026.parquet', url('local_election_ward_2026.parquet'), duckdb.DuckDBDataProtocol.HTTP, false)

  const conn = await db.connect()

  await conn.query(`
    CREATE OR REPLACE VIEW election_results AS
    SELECT
      "General election polling date"   AS polling_date,
      CAST(strftime("General election polling date", '%Y') AS INTEGER) AS election_year,
      "Country name"                    AS country,
      "English region name"             AS region,
      constituency_name,
      pcon_code,
      "Electorate"                      AS electorate,
      "Election valid vote count"       AS valid_votes,
      "Election result summary"         AS result_summary,
      "Candidate family name"           AS candidate_family_name,
      "Candidate given name"            AS candidate_given_name,
      "Candidate is sitting MP"         AS sitting_mp,
      "Main party name"                 AS party,
      "Candidate vote count"            AS candidate_votes,
      "Majority"                        AS majority,
      "Candidate result position"       AS position
    FROM read_parquet('election_results.parquet')
  `)
  await conn.query(`CREATE OR REPLACE VIEW uk_wards AS SELECT * FROM read_parquet('uk_wards.parquet')`)
  await conn.query(`
    CREATE OR REPLACE VIEW local_2026 AS
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
    FROM read_parquet('local_election_ward_2026.parquet')
  `)
  // Bridge WD25 ward codes (uk_wards) and WD26 (local_2026) by a
  // normalised (ward_name, lad_name) key.
  await conn.query(`
    CREATE OR REPLACE MACRO ward_key(ward_name, lad_name) AS
      regexp_replace(
        replace(lower(coalesce(ward_name,'') || '|' || coalesce(lad_name,'')), '&', 'and'),
        '[^a-z0-9|]+', '', 'g'
      )
  `)
  return conn
}

function ready() {
  if (!initPromise) initPromise = init()
  return initPromise
}

// --- helpers ------------------------------------------------------

// Convert an Arrow table to plain row objects, demoting BigInts and
// normalising Date columns to ISO date strings.
function toRows(table) {
  return table.toArray().map((row) => {
    const o = {}
    for (const k of Object.keys(row)) o[k] = unwrap(row[k])
    return o
  })
}

function unwrap(v) {
  if (typeof v === 'bigint') return Number(v)
  if (v instanceof Date) return v.toISOString().slice(0, 10)
  return v
}

async function run(sql, params = []) {
  const conn = await ready()
  if (params.length === 0) return toRows(await conn.query(sql))
  const stmt = await conn.prepare(sql)
  try {
    return toRows(await stmt.query(...params))
  } finally {
    await stmt.close()
  }
}

// --- canonical party mapping (mirrors the Go service) -------------

const CANONICAL = {
  'Labour': 'Labour',
  'Labour and Co-operative': 'Labour',
  'Conservative': 'Conservative',
  'Liberal Democrats': 'Liberal Democrats',
  'Liberal Democrat': 'Liberal Democrats',
  'Reform UK': 'Reform UK',
  'Brexit Party': 'Reform UK',
  'UK Independence Party (UKIP)': 'UKIP',
  'UK Independence Party': 'UKIP',
  'UKIP': 'UKIP',
  'Green Party': 'Green',
  'Scottish Green Party': 'Green',
  'Scottish National Party (SNP)': 'SNP',
  'Scottish National Party': 'SNP',
  'Plaid Cymru - The Party of Wales': 'Plaid Cymru',
  'Plaid Cymru': 'Plaid Cymru',
  'Democratic Unionist Party': 'DUP',
  'Sinn Féin': 'Sinn Féin',
  'Social Democratic & Labour Party': 'SDLP',
  'Ulster Unionist Party': 'UUP',
  'Alliance': 'Alliance',
  'Alliance - Alliance Party of Northern Ireland': 'Alliance',
}
const PARTY_ORDER = ['Labour', 'Conservative', 'Liberal Democrats', 'Reform UK', 'UKIP', 'Green', 'SNP', 'Plaid Cymru', 'DUP', 'Sinn Féin', 'SDLP', 'UUP', 'Alliance', 'Other']

function canonical(p) { return CANONICAL[p] ?? 'Other' }

// --- public API ---------------------------------------------------

export const db = {
  ready,

  async partyShare() {
    const rows = await run(`
      SELECT polling_date, party, SUM(candidate_votes)::BIGINT AS votes
      FROM election_results
      WHERE party IS NOT NULL
      GROUP BY polling_date, party
      ORDER BY polling_date
    `)
    const perElection = new Map() // date -> Map(party -> votes)
    const dates = []
    for (const r of rows) {
      const date = r.polling_date
      if (!perElection.has(date)) { perElection.set(date, new Map()); dates.push(date) }
      const k = canonical(r.party)
      const m = perElection.get(date)
      m.set(k, (m.get(k) ?? 0) + Number(r.votes))
    }
    const totals = {}
    for (const [d, m] of perElection) {
      let t = 0; for (const v of m.values()) t += v
      totals[d] = t
    }
    const series = []
    for (const p of PARTY_ORDER) {
      const shares = []
      const votes = []
      let any = false
      for (const d of dates) {
        const v = perElection.get(d).get(p) ?? 0
        votes.push(v)
        shares.push(totals[d] > 0 ? (v * 100) / totals[d] : 0)
        if (v > 0) any = true
      }
      if (any) series.push({ party: p, shares, votes })
    }
    return { elections: dates, series, totals_by_election: totals }
  },

  async search(q) {
    const term = (q ?? '').trim()
    if (!term) return { query: term, hits: [] }
    const pattern = `%${term.toLowerCase()}%`
    const rows = await run(`
      WITH constituencies AS (
        SELECT 'constituency' AS type,
               MAX(pcon_code)        AS code,
               constituency_name     AS name,
               any_value(country)    AS ctx
        FROM election_results
        WHERE lower(constituency_name) LIKE ? OR lower(pcon_code) LIKE ?
        GROUP BY constituency_name
      ),
      councils AS (
        SELECT DISTINCT 'council' AS type, lad_code AS code, lad_name AS name, '' AS ctx
        FROM uk_wards
        WHERE lower(lad_name) LIKE ? OR lower(lad_code) LIKE ?
      ),
      wards AS (
        SELECT DISTINCT 'ward' AS type, ward_code AS code, ward_name AS name, lad_name AS ctx
        FROM uk_wards
        WHERE lower(ward_name) LIKE ? OR lower(ward_code) LIKE ?
      )
      SELECT type, code, name, ctx FROM constituencies
      UNION ALL SELECT type, code, name, ctx FROM councils
      UNION ALL SELECT type, code, name, ctx FROM wards
      LIMIT 50
    `, [pattern, pattern, pattern, pattern, pattern, pattern])
    return {
      query: term,
      hits: rows.map((r) => ({ type: r.type, code: r.code, name: r.name, context: r.ctx ?? '' })),
    }
  },

  async constituency(code) {
    const [head] = await run(`
      WITH input AS (
        SELECT constituency_name AS name
        FROM election_results
        WHERE pcon_code = ?
        LIMIT 1
      )
      SELECT constituency_name AS name, country, COALESCE(region,'') AS region,
             MAX(pcon_code) OVER (PARTITION BY constituency_name) AS canonical_code
      FROM election_results
      WHERE constituency_name = (SELECT name FROM input)
      ORDER BY polling_date DESC
      LIMIT 1
    `, [code])
    if (!head) throw new Error('constituency not found')
    const canonicalCode = head.canonical_code

    const candidates = await run(`
      SELECT polling_date, election_year AS year, electorate, valid_votes, result_summary,
        majority, position, COALESCE(candidate_given_name,'') AS given_name,
        candidate_family_name AS family_name, COALESCE(party,'Unknown') AS party,
        candidate_votes AS votes, COALESCE(sitting_mp,false) AS sitting_mp
      FROM election_results
      WHERE constituency_name = ?
      ORDER BY polling_date DESC, position ASC
    `, [head.name])

    const byDate = new Map()
    const order = []
    for (const c of candidates) {
      let blk = byDate.get(c.polling_date)
      if (!blk) {
        blk = {
          polling_date: c.polling_date,
          year: c.year,
          electorate: c.electorate,
          valid_votes: c.valid_votes,
          result_summary: c.result_summary,
          majority: c.majority ?? null,
          candidates: [],
        }
        byDate.set(c.polling_date, blk)
        order.push(c.polling_date)
      }
      blk.candidates.push({
        position: c.position, given_name: c.given_name, family_name: c.family_name,
        party: c.party, votes: c.votes, sitting_mp: c.sitting_mp,
      })
    }

    const wards = (await run(`
      SELECT ward_code AS code, ward_name AS name, lad_code, lad_name,
        split_ward, is_primary_by_area AS primary_
      FROM uk_wards
      WHERE pcon_code = ?
      ORDER BY ward_name
    `, [canonicalCode])).map((w) => ({
      code: w.code, name: w.name, lad_code: w.lad_code, lad_name: w.lad_name,
      split_ward: w.split_ward, primary: w.primary_ ?? null,
    }))

    return {
      code: canonicalCode, name: head.name, country: head.country, region: head.region,
      results: order.map((d) => byDate.get(d)),
      wards,
    }
  },

  async ward(code) {
    const [head] = await run(`SELECT ward_name AS name, lad_code, lad_name FROM uk_wards WHERE ward_code = ? LIMIT 1`, [code])
    if (!head) throw new Error('ward not found')

    const constituencies = (await run(`
      SELECT pcon_code AS code, pcon_name AS name, split_ward AS split, is_primary_by_area AS primary_
      FROM uk_wards
      WHERE ward_code = ?
      ORDER BY is_primary_by_area DESC NULLS LAST, pcon_name
    `, [code])).map((p) => ({ code: p.code, name: p.name, split: p.split, primary: p.primary_ ?? null }))

    const [local] = await run(`
      WITH target AS (
        SELECT ward_key(ward_name, lad_name) AS k FROM uk_wards WHERE ward_code = ? LIMIT 1
      )
      SELECT seats, election_type,
        COALESCE(seat1_winner,'') AS s1, COALESCE(seat2_winner,'') AS s2, COALESCE(seat3_winner,'') AS s3,
        LAB, CON, LD, GREEN, REF, IND, other_votes
      FROM local_2026
      WHERE ward_code = ?
         OR ward_key(ward_name, lad_name) = (SELECT k FROM target)
      LIMIT 1
    `, [code, code])

    let local_2026 = null
    if (local) {
      const raw = [
        ['Labour', local.LAB], ['Conservative', local.CON], ['Liberal Democrats', local.LD],
        ['Green', local.GREEN], ['Reform UK', local.REF], ['Independent', local.IND], ['Other', local.other_votes],
      ]
      const total = raw.reduce((s, [, v]) => s + (v ?? 0), 0)
      const votes = raw
        .filter(([, v]) => v && v > 0)
        .map(([party, v]) => ({ party, votes: v, share: total ? (v * 100) / total : 0 }))
      local_2026 = {
        seats: local.seats ?? null,
        election_type: local.election_type ?? '',
        seat_winners: [local.s1, local.s2, local.s3].filter((x) => x),
        votes,
      }
    }

    return { code, name: head.name, lad_code: head.lad_code, lad_name: head.lad_name, constituencies, local_2026 }
  },

  async council(code) {
    const [head] = await run(`SELECT lad_name AS name FROM uk_wards WHERE lad_code = ? LIMIT 1`, [code])
    if (!head) throw new Error('council not found')

    const wardRows = await run(`
      SELECT ward_code,
        any_value(ward_name)                       AS ward_name,
        any_value(ward_key(ward_name, lad_name))   AS k,
        string_agg(pcon_name, ' / ' ORDER BY is_primary_by_area DESC NULLS LAST, pcon_name) AS pcons
      FROM uk_wards
      WHERE lad_code = ?
      GROUP BY ward_code
      ORDER BY ward_name
    `, [code])

    const wards = []
    const codeIndex = new Map()
    const keyIndex = new Map()
    for (const w of wardRows) {
      const row = {
        ward_code: w.ward_code,
        ward_name: w.ward_name,
        constituencies: w.pcons ? w.pcons.split(' / ') : [],
        local_2026: null,
        seat_winners: null,
      }
      const i = wards.push(row) - 1
      codeIndex.set(w.ward_code, i)
      if (w.k) keyIndex.set(w.k, i)
    }

    const local = await run(`
      SELECT ward_code, ward_name, ward_key(ward_name, lad_name) AS k,
        COALESCE(seat1_winner,'') AS s1, COALESCE(seat2_winner,'') AS s2, COALESCE(seat3_winner,'') AS s3,
        LAB, CON, LD, GREEN, REF, IND, other_votes
      FROM local_2026
      WHERE lad_name = ?
    `, [head.name])

    let totLab = 0, totCon = 0, totLD = 0, totGreen = 0, totRef = 0, totInd = 0, totOther = 0
    for (const l of local) {
      totLab += l.LAB; totCon += l.CON; totLD += l.LD; totGreen += l.GREEN
      totRef += l.REF; totInd += l.IND; totOther += l.other_votes
      const raw = [
        ['Labour', l.LAB], ['Conservative', l.CON], ['Liberal Democrats', l.LD],
        ['Green', l.GREEN], ['Reform UK', l.REF], ['Independent', l.IND], ['Other', l.other_votes],
      ]
      const total = raw.reduce((s, [, v]) => s + (v ?? 0), 0)
      const entries = raw
        .filter(([, v]) => v && v > 0)
        .map(([party, v]) => ({ party, votes: v, share: total ? (v * 100) / total : 0 }))
        .sort((a, b) => b.votes - a.votes)
      const winners = [l.s1, l.s2, l.s3].filter((x) => x)

      let idx = -1
      if (l.ward_code && codeIndex.has(l.ward_code)) idx = codeIndex.get(l.ward_code)
      if (idx < 0 && l.k && keyIndex.has(l.k)) idx = keyIndex.get(l.k)
      if (idx >= 0) {
        wards[idx].local_2026 = entries
        wards[idx].seat_winners = winners
      } else {
        wards.push({
          ward_code: '', ward_name: l.ward_name,
          constituencies: [], local_2026: entries, seat_winners: winners,
        })
      }
    }

    const summaryRaw = [
      ['Labour', totLab], ['Conservative', totCon], ['Liberal Democrats', totLD],
      ['Green', totGreen], ['Reform UK', totRef], ['Independent', totInd], ['Other', totOther],
    ]
    const summaryTotal = summaryRaw.reduce((s, [, v]) => s + v, 0)
    const summary_2026 = summaryRaw
      .filter(([, v]) => v > 0)
      .map(([party, v]) => ({ party, votes: v, share: summaryTotal ? (v * 100) / summaryTotal : 0 }))

    return { code, name: head.name, ward_count: wardRows.length, summary_2026, wards }
  },
}
