/**
 * Client-side CRDT merge — mirrors the Go server's mergeCRDT() exactly.
 *
 * Field strategies:
 *   immutable    id, created, project      — never overwritten
 *   lww-register title, description,       — latest updated_at wins
 *                status, estimate_h
 *   max-register priority                  — only escalates
 *   or-set       tags, assignees           — union of additions
 *   p-counter    vote_count                — only ever increases
 *   append-log   comments                  — union by id, sorted by ts
 */

const PRIORITY = ['low', 'medium', 'high', 'critical']

/** Merge two ticket objects. Returns a new merged ticket. */
export function mergeTickets(local, remote) {
  // Immutable fields come from whichever version first established them.
  const result = { ...local }

  // lww-register: compare updated_at timestamps
  const lTs = new Date(local.updated_at  || local.created  || 0).getTime()
  const rTs = new Date(remote.updated_at || remote.created || 0).getTime()

  if (rTs > lTs) {
    if (remote.title       != null) result.title       = remote.title
    if (remote.description != null) result.description = remote.description
    if (remote.status      != null) result.status      = remote.status
    if (remote.estimate_h  != null) result.estimate_h  = remote.estimate_h
    result.updated_at = remote.updated_at
  }

  // max-register: priority only escalates
  const li = PRIORITY.indexOf(local.priority  || 'low')
  const ri = PRIORITY.indexOf(remote.priority || 'low')
  result.priority = PRIORITY[Math.max(li, ri)]

  // or-set: union, sorted for stability
  result.tags      = orSet(local.tags,      remote.tags)
  result.assignees = orSet(local.assignees, remote.assignees)

  // p-counter: vote count only goes up
  result.vote_count = Math.max(local.vote_count || 0, remote.vote_count || 0)

  // append-log: union by comment id, sorted by timestamp
  result.comments = mergeComments(local.comments, remote.comments)

  return result
}

function orSet(a = [], b = []) {
  return [...new Set([...a, ...b])].sort()
}

function mergeComments(a = [], b = []) {
  const map = new Map()
  for (const c of [...a, ...b]) {
    if (!map.has(c.id)) map.set(c.id, c)
  }
  return [...map.values()].sort((x, y) => x.ts.localeCompare(y.ts))
}
