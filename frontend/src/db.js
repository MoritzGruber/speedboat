import { openDB } from 'idb'

const DB_NAME    = 'ticket-store'
const DB_VERSION = 1

let _db = null

async function db() {
  if (_db) return _db
  _db = await openDB(DB_NAME, DB_VERSION, {
    upgrade(db) {
      if (!db.objectStoreNames.contains('projects')) {
        db.createObjectStore('projects', { keyPath: 'id' })
      }
      if (!db.objectStoreNames.contains('tickets')) {
        const ts = db.createObjectStore('tickets', { keyPath: 'id' })
        ts.createIndex('by_project', 'project', { unique: false })
        ts.createIndex('by_status',  'status',  { unique: false })
      }
      if (!db.objectStoreNames.contains('sync_queue')) {
        db.createObjectStore('sync_queue', { keyPath: '_qid', autoIncrement: true })
      }
      if (!db.objectStoreNames.contains('settings')) {
        db.createObjectStore('settings', { keyPath: 'key' })
      }
    },
  })
  return _db
}

// ── Projects ──────────────────────────────────────────────────────────────────

export async function dbGetProjects() {
  return (await db()).getAll('projects')
}

export async function dbSaveProject(proj) {
  await (await db()).put('projects', proj)
}

// ── Tickets ───────────────────────────────────────────────────────────────────

export async function dbGetTicketsByProject(projectId) {
  const d   = await db()
  const idx = d.transaction('tickets').store.index('by_project')
  return idx.getAll(projectId)
}

export async function dbGetAllTickets() {
  return (await db()).getAll('tickets')
}

export async function dbSaveTicket(ticket) {
  await (await db()).put('tickets', ticket)
}

export async function dbDeleteTicket(id) {
  await (await db()).delete('tickets', id)
}

// ── Sync queue ────────────────────────────────────────────────────────────────

/**
 * Enqueue a pending operation to be flushed to the backend.
 * @param {{ op: 'create'|'update'|'delete', entityType: 'project'|'ticket', entityId: string, projId: string, data: object }} item
 */
export async function dbEnqueue(item) {
  await (await db()).add('sync_queue', { ...item, ts: new Date().toISOString() })
}

export async function dbGetQueue() {
  return (await db()).getAll('sync_queue')
}

export async function dbDequeue(qid) {
  await (await db()).delete('sync_queue', qid)
}

export async function dbClearQueue() {
  await (await db()).clear('sync_queue')
}

// ── Settings ──────────────────────────────────────────────────────────────────

export async function dbGetSetting(key, fallback = null) {
  const row = await (await db()).get('settings', key)
  return row ? row.value : fallback
}

export async function dbSetSetting(key, value) {
  await (await db()).put('settings', { key, value })
}
