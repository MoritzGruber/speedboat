let BASE = (import.meta.env.VITE_API_URL || 'http://localhost:8080').replace(/\/$/, '')

export function getBaseURL()      { return BASE }
export function configure(url)    { BASE = url.replace(/\/$/, '') }

async function req(method, path, body) {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: body != null ? { 'Content-Type': 'application/json' } : {},
    body:    body != null ? JSON.stringify(body) : undefined,
    signal:  AbortSignal.timeout(6000),
  })
  if (res.status === 204) return null
  
  const text = await res.text()
  const json = text ? JSON.parse(text) : {}
  
  if (!res.ok) throw new Error(json.error || res.statusText)
  return json
}

export const api = {
  ping: () => req('GET', '/issues'),

  listTickets: async () => {
    const issues = await req('GET', '/issues')
    console.log('[API] Raw backend response:', issues)

    const mapped = (issues || []).map(i => {
      // Jira often nests field values inside objects
      const rawStatus = i.fields?.status
      const statusStr = (typeof rawStatus === 'object' && rawStatus !== null) ? rawStatus.name : (rawStatus || 'DRAFT')
      
      const rawPriority = i.fields?.priority
      const priorityStr = (typeof rawPriority === 'object' && rawPriority !== null) ? rawPriority.name : (rawPriority || 'Medium')
      
      return { 
        id: i.id, 
        ...i.fields,
        status: statusStr,                     // Extract string safely
        priority: priorityStr.toLowerCase()    // Extract string safely
      }
    })

    console.log('[API] Mapped tickets:', mapped)
    return mapped
  },
  
  createTicket: (t) => req('POST', '/issues', { id: t.id, key: t.id, fields: t }),
  updateTicket: (id, t) => req('PATCH', `/issues/${id}`, { id, fields: t }),
  deleteTicket: (id) => req('DELETE', `/issues/${id}`),
}