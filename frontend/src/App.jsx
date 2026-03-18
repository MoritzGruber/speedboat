import { useState, useEffect, useCallback, useRef } from 'react'
import { SyncEngine } from './sync.js'
import { configure, getBaseURL } from './api.js'
import {
  dbGetTicketsByProject,
  dbSaveTicket, dbDeleteTicket,
  dbGetSetting, dbSetSetting,
} from './db.js'
import Board from './components/Board.jsx'
import { TicketModal, SettingsModal } from './components/Modals.jsx'
import SyncBadge from './components/SyncBadge.jsx'

const now = () => new Date().toISOString()
const PROJ_ID = 'STACKITPMO'

function makeTicketId(count) {
  return `${PROJ_ID}-${count + 1}`
}

export default function App() {
  const [tickets,     setTickets]     = useState([])
  const [syncStatus,  setSyncStatus]  = useState('offline')
  const [modal,       setModal]       = useState(null)
  const [dragging,    setDragging]    = useState(null)
  const engineRef = useRef(null)

  useEffect(() => {
    async function init() {
      const savedUrl = await dbGetSetting('api_url')
      if (savedUrl) configure(savedUrl)
      
      const ts = await dbGetTicketsByProject(PROJ_ID)
      setTickets(ts)
    }
    init()

    const engine = new SyncEngine({
      onStatusChange: setSyncStatus,
      onDataChange: async () => {
        setTickets(await dbGetTicketsByProject(PROJ_ID))
      },
    })
    engine.start()
    engineRef.current = engine

    return () => engine.stop()
  }, [])

  const createTicket = useCallback(async (data) => {
    const ticket = {
      id:          makeTicketId(tickets.length),
      project:     PROJ_ID,
      created:     now(),
      updated_at:  now(),
      vote_count:  0,
      comments:    [],
      tags:        [],
      assignees:   [],
      status:      'open',
      priority:    'medium',
      ...data,
    }
    await dbSaveTicket(ticket)
    setTickets(ts => [...ts, ticket])
    await engineRef.current?.queueOp({
      op: 'create', entityType: 'ticket', entityId: ticket.id, data: ticket,
    })
    return ticket
  }, [tickets])

  const updateTicket = useCallback(async (id, updates) => {
    setTickets(ts => ts.map(t => {
      if (t.id !== id) return t
      const merged = { ...t, ...updates, updated_at: now() }
      dbSaveTicket(merged)
      engineRef.current?.queueOp({
        op: 'update', entityType: 'ticket', entityId: id, data: updates,
      })
      return merged
    }))
  }, [])

  const deleteTicket = useCallback(async (id) => {
    await dbDeleteTicket(id)
    setTickets(ts => ts.filter(t => t.id !== id))
    await engineRef.current?.queueOp({
      op: 'delete', entityType: 'ticket', entityId: id,
    })
  }, [])

  const handleDrop = useCallback((status) => {
    if (dragging) {
      updateTicket(dragging, { status })
      setDragging(null)
    }
  }, [dragging, updateTicket])

  const saveSettings = useCallback(async ({ apiUrl }) => {
    await dbSetSetting('api_url', apiUrl)
    configure(apiUrl)
    setModal(null)
    engineRef.current?.stop()
    engineRef.current?.start()
  }, [])

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <aside style={{ width: 220, flexShrink: 0, background: 'var(--bg-secondary)', borderRight: '1px solid var(--border)', display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '14px 16px 12px', fontSize: 14, fontWeight: 500, borderBottom: '1px solid var(--border)' }}>
          Ticket Store
        </div>
        <div style={{ flex: 1, overflowY: 'auto', padding: '6px 0' }}>
          <div style={{ padding: '7px 16px', fontSize: 13, display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderLeft: `2px solid var(--text-primary)`, background: 'var(--bg-primary)', color: 'var(--text-primary)', fontWeight: 500 }}>
            <span>{PROJ_ID}</span>
            <span style={{ fontSize: 11, color: 'var(--text-tertiary)', marginLeft: 8, flexShrink: 0 }}>
              {tickets.length}
            </span>
          </div>
        </div>
        <div style={{ padding: '10px 12px', borderTop: '1px solid var(--border)', display: 'flex', flexDirection: 'column', gap: 6 }}>
          <button className="btn" style={{ width: '100%', fontSize: 12 }} onClick={() => setModal({ type: 'settings' })}>
            Settings
          </button>
        </div>
      </aside>

      <main style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <header style={{ padding: '10px 20px', background: 'var(--bg-secondary)', borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexShrink: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 14, fontWeight: 500 }}>{PROJ_ID}</span>
            <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>Jira Connected Board</span>
            <SyncBadge status={syncStatus} />
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <button className="btn btn-primary" onClick={() => setModal({ type: 'ticket', defaultStatus: 'open' })}>+ New ticket</button>
          </div>
        </header>

        <Board tickets={tickets} onDragStart={setDragging} onDrop={handleDrop} onEdit={t  => setModal({ type: 'ticket', mode: 'edit', ticket: t })} onDelete={deleteTicket} onAdd={status => setModal({ type: 'ticket', mode: 'create', defaultStatus: status })} />
      </main>

      {modal?.type === 'ticket' && (
        <TicketModal mode={modal.mode ?? 'create'} ticket={modal.ticket} defaultStatus={modal.defaultStatus} onSave={data => { modal.mode === 'edit' ? updateTicket(modal.ticket.id, data) : createTicket(data); setModal(null) }} onDelete={modal.mode === 'edit' ? () => { deleteTicket(modal.ticket.id); setModal(null) } : null} onClose={() => setModal(null)} />
      )}
      {modal?.type === 'settings' && <SettingsModal currentUrl={getBaseURL()} onSave={saveSettings} onClose={() => setModal(null)} />}
    </div>
  )
}