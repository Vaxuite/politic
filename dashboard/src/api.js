const BASE = '/api'

async function get(path) {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(`HTTP ${res.status}: ${text || res.statusText}`)
  }
  return res.json()
}

export const api = {
  partyShare: () => get('/elections/party-share'),
  search: (q) => get(`/search?q=${encodeURIComponent(q)}`),
  constituency: (code) => get(`/constituency/${encodeURIComponent(code)}`),
  ward: (code) => get(`/ward/${encodeURIComponent(code)}`),
  council: (code) => get(`/council/${encodeURIComponent(code)}`),
}
