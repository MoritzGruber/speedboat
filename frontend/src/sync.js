import { api } from './api.js'
import { mergeTickets } from './crdt.js'
import {
  dbGetTicketsByProject,
  dbSaveTicket,
  dbEnqueue, dbGetQueue, dbDequeue,
} from './db.js'

export class SyncEngine {
  constructor({ onStatusChange, onDataChange }) {
    this.connected      = false
    this._interval      = null
    this._onStatus      = onStatusChange || (() => {})
    this._onData        = onDataChange   || (() => {})
  }

  start() {
    this._tick()
    this._interval = setInterval(() => this._tick(), 5000)
  }

  stop() {
    clearInterval(this._interval)
    this._interval = null
  }

  async queueOp(op) {
    await dbEnqueue(op)
    if (this.connected) await this._flushQueue()
  }

  async _tick() {
    try {
      await api.ping()
      if (!this.connected) {
        this.connected = true
        this._onStatus('syncing')
        await this._fullSync()
        this._onData()
      } else {
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

  async _fullSync() {
    await this._flushQueue()

    try {
      const remoteTickets = await api.listTickets()
      const localTickets  = await dbGetTicketsByProject('STACKITPMO')
      const localMap      = new Map(localTickets.map(t => [t.id, t]))

      for (const rt of remoteTickets) {
        // Enforce the hardcoded project ID for the local database
        rt.project = 'STACKITPMO' 
        const lt = localMap.get(rt.id)
        await dbSaveTicket(lt ? mergeTickets(lt, rt) : rt)
      }
    } catch { /* non-fatal */ }
  }

  async _flushQueue() {
    const queue = await dbGetQueue()
    for (const item of queue) {
      try {
        await this._dispatch(item)
        await dbDequeue(item._qid)
      } catch (err) {
        console.warn('[sync] flush stalled at item', item._qid, err.message)
        this._onStatus('offline')
        this.connected = false
        break
      }
    }
  }

  async _dispatch({ op, entityType, entityId, data }) {
    if (entityType === 'ticket') {
      if      (op === 'create') await api.createTicket(data)
      else if (op === 'update') await api.updateTicket(entityId, data)
      else if (op === 'delete') await api.deleteTicket(entityId)
    }
  }
}