import { api } from './api.js'
import { mergeTickets } from './crdt.js'
import {
  dbGetProjects, dbSaveProject,
  dbGetTicketsByProject,
  dbSaveTicket,
  dbEnqueue, dbGetQueue, dbDequeue,
} from './db.js'

/**
 * SyncEngine
 *
 * Lifecycle:
 *   engine.start()       — begin polling
 *   engine.stop()        — stop polling
 *   engine.queueOp(op)   — enqueue a local change; flushes immediately if connected
 *
 * Status values emitted via onStatusChange:
 *   'offline'  — backend unreachable
 *   'syncing'  — actively flushing / pulling
 *   'synced'   — up to date
 *   'error'    — unrecoverable problem (rare)
 */
export class SyncEngine {
  constructor({ onStatusChange, onDataChange }) {
    this.connected      = false
    this._interval      = null
    this._onStatus      = onStatusChange || (() => {})
    this._onData        = onDataChange   || (() => {})
  }

  // ── Public API ─────────────────────────────────────────────────────────────

  start() {
    this._tick()
    this._interval = setInterval(() => this._tick(), 5000)
  }

  stop() {
    clearInterval(this._interval)
    this._interval = null
  }

  /**
   * Call after every local write.
   * Persists the op to the queue, then tries to flush immediately if online.
   */
  async queueOp(op) {
    await dbEnqueue(op)
    if (this.connected) {
      await this._flushQueue()
    }
  }

  // ── Internal ───────────────────────────────────────────────────────────────

  async _tick() {
    try {
      await api.ping()
      if (!this.connected) {
        // Newly connected — do a full sync
        this.connected = true
        this._onStatus('syncing')
        await this._fullSync()
        this._onData()
      } else {
        // Already connected — just flush pending ops
        const queue = await dbGetQueue()
        if (queue.length > 0) {
          this._onStatus('syncing')
          await this._flushQueue()
          this._onStatus('synced')
        }
      }
      this._onStatus('synced')
    } catch {
      if (this.connected) {
        this.connected = false
        this._onStatus('offline')
      }
    }
  }

  /** Full sync: flush queue → pull remote projects → pull remote tickets → CRDT merge */
  async _fullSync() {
    // 1. Send pending local changes first so the server is up to date before we pull
    await this._flushQueue()

    // 2. Pull and persist all remote projects
    let remoteProjects = []
    try {
      remoteProjects = await api.listProjects()
      for (const rp of remoteProjects) {
        await dbSaveProject(rp)
      }
    } catch { /* non-fatal */ }

    // 3. Pull tickets for every known project and CRDT-merge into local IDB
    const localProjects = await dbGetProjects()
    const allProjIds = [
      ...new Set([
        ...localProjects.map(p => p.id),
        ...remoteProjects.map(p => p.id),
      ]),
    ]

    for (const projId of allProjIds) {
      try {
        const remoteTickets = await api.listTickets(projId)
        const localTickets  = await dbGetTicketsByProject(projId)
        const localMap      = new Map(localTickets.map(t => [t.id, t]))

        for (const rt of remoteTickets) {
          const lt = localMap.get(rt.id)
          await dbSaveTicket(lt ? mergeTickets(lt, rt) : rt)
        }
      } catch { /* skip unreachable projects */ }
    }
  }

  /** Drain the sync queue, stopping at the first failure. */
  async _flushQueue() {
    const queue = await dbGetQueue()
    for (const item of queue) {
      try {
        await this._dispatch(item)
        await dbDequeue(item._qid)
      } catch (err) {
        // Leave remaining items in the queue; retry on next tick
        console.warn('[sync] flush stalled at item', item._qid, err.message)
        this._onStatus('offline')
        this.connected = false
        break
      }
    }
  }

  /** Translate a queue item into the corresponding API call. */
  async _dispatch({ op, entityType, entityId, projId, data }) {
    if (entityType === 'project') {
      if (op === 'create') await api.createProject(data)
    } else if (entityType === 'ticket') {
      if      (op === 'create') await api.createTicket(projId, data)
      else if (op === 'update') await api.updateTicket(projId, entityId, data)
      else if (op === 'delete') await api.deleteTicket(projId, entityId)
    }
  }
}
