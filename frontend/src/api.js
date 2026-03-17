// Default URL from env var; can be overridden at runtime via configure()
let BASE = (import.meta.env.VITE_API_URL || 'http://localhost:8080').replace(/\/$/, '')

export function getBaseURL()      { return BASE }
export function configure(url)    { BASE = url.replace(/\/$/, '') }

async function req(method, path, body) {
  const res = await fetch(`${BASE}/api${path}`, {
    method,
    headers: body != null ? { 'Content-Type': 'application/json' } : {},
    body:    body != null ? JSON.stringify(body) : undefined,
    signal:  AbortSignal.timeout(6000),
  })
  if (res.status === 204) return null
  const json = await res.json()
  if (!res.ok) throw new Error(json.error || res.statusText)
  return json
}

export const api = {
  /** Lightweight connectivity check — just lists projects */
  ping:          ()           => req('GET',    '/projects'),

  listProjects:  ()           => req('GET',    '/projects'),
  createProject: (p)          => req('POST',   '/projects', p),

  listTickets:   (proj)       => req('GET',    `/projects/${proj}/tickets`),
  createTicket:  (proj, t)    => req('POST',   `/projects/${proj}/tickets`, t),
  updateTicket:  (proj, id, t)=> req('PUT',    `/projects/${proj}/tickets/${id}`, t),
  deleteTicket:  (proj, id)   => req('DELETE', `/projects/${proj}/tickets/${id}`),

  getBoard:      (proj)       => req('GET',    `/projects/${proj}/board`),
  getHistory:    (proj, n)    => req('GET',    `/projects/${proj}/history${n ? `?n=${n}` : ''}`),
}
