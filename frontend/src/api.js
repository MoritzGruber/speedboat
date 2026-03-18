let BASE = (import.meta.env.VITE_API_URL || 'http://localhost:8080').replace(/\/$/, '')

export function getBaseURL()      { return BASE }
export function configure(url)    { BASE = url.replace(/\/$/, '') }

async function req(method, path, body) {
  // We removed the '/api' prefix because cmd/main.go mounts directly to root
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: body != null ? { 'Content-Type': 'application/json' } : {},
    body:    body != null ? JSON.stringify(body) : undefined,
    signal:  AbortSignal.timeout(6000),
  })
  if (res.status === 204) return null
  
  // Handle empty bodies gracefully
  const text = await res.text()
  const json = text ? JSON.parse(text) : {}
  
  if (!res.ok) throw new Error(json.error || res.statusText)
  return json
}

export const api = {
  ping: () => req('GET', '/issues'),

  listTickets: async () => {
    const issues = await req('GET', '/issues')
    // Flatten the backend Issue struct into the frontend Ticket struct
    return (issues || []).map(i => ({ id: i.id, ...i.fields }))
  },
  
  createTicket: (t) => req('POST', '/issues', { id: t.id, key: t.id, fields: t }),
  updateTicket: (id, t) => req('PATCH', `/issues/${id}`, { id, fields: t }),
  deleteTicket: (id) => req('DELETE', `/issues/${id}`),
}