import { useState, useEffect, useCallback, useRef } from 'react'
import { SyncEngine } from './sync.js'
import { configure, getBaseURL } from './api.js'
import {
  dbGetProjects, dbSaveProject,
  dbGetTicketsByProject,
  dbSaveTicket, dbDeleteTicket,
  dbGetSetting, dbSetSetting,
} from './db.js'
import Board from './components/Board.jsx'
import { TicketModal, ProjectModal, HistoryModal, SettingsModal } from './components/Modals.jsx'
import SyncBadge from './components/SyncBadge.jsx'

const now = () => new Date().toISOString()

function makeTicketId(projId, count) {
  const prefix = projId.slice(0, 4).toUpperCase()
  return `${prefix}-${count + 1}`
}

export default function App() {
  const [projects,    setProjects]    = useState([])
  const [tickets,     setTickets]     = useState([])   // all tickets across all projects
  const [projId,      setProjId]      = useState(null)
  const [syncStatus,  setSyncStatus]  = useState('offline')
  const [modal,       setModal]       = useState(null)
  const [dragging,    setDragging]    = useState(null)
  const engineRef = useRef(null)

  // ── Init: load IDB, restore settings, start sync engine ───────────────────
  useEffect(() => {
    async function init() {
      // Restore saved API URL override
      const savedUrl = await dbGetSetting('api_url')
      if (savedUrl) configure(savedUrl)

      // Load local data
      const ps = await dbGetProjects()
      setProjects(ps)
      if (ps.length > 0) {
        const lastProjId = await dbGetSetting('last_project', ps[0].id)
        const validId = ps.find(p => p.id === lastProjId) ? lastProjId : ps[0].id
        setProjId(validId)
        const ts = await dbGetTicketsByProject(validId)
        setTickets(ts)
      }
    }
    init()

    // Start sync engine
    const engine = new SyncEngine({
      onStatusChange: setSyncStatus,
      onDataChange: async () => {
        // Remote data arrived — reload from IDB
        const ps = await dbGetProjects()
        setProjects(ps)
        if (ps.length > 0) {
          const id = ps[0].id
          setProjId(prev => prev || id)
          const all = await Promise.all(ps.map(p => dbGetTicketsByProject(p.id)))
          setTickets(all.flat())
        }
      },
    })
    engine.start()
    engineRef.current = engine

    return () => engine.stop()
  }, [])

  // ── Reload tickets when project switches ──────────────────────────────────
  useEffect(() => {
    if (!projId) return
    dbGetTicketsByProject(projId).then(ts => {
      setTickets(prev => [...prev.filter(t => t.project !== projId), ...ts])
    })
    dbSetSetting('last_project', projId)
  }, [projId])

  // ── CRUD helpers ──────────────────────────────────────────────────────────

  const createProject = useCallback(async (data) => {
    const proj = { ...data, created: now() }
    await dbSaveProject(proj)
    setProjects(ps => [...ps, proj])
    setProjId(proj.id)
    await engineRef.current?.queueOp({
      op: 'create', entityType: 'project', projId: proj.id, data: proj,
    })
  }, [])

  const createTicket = useCallback(async (data) => {
    const count = tickets.filter(t => t.project === projId).length
    const ticket = {
      id:          makeTicketId(projId, count),
      project:     projId,
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
      op: 'create', entityType: 'ticket', entityId: ticket.id, projId, data: ticket,
    })
    return ticket
  }, [projId, tickets])

  const updateTicket = useCallback(async (id, updates) => {
    setTickets(ts => ts.map(t => {
      if (t.id !== id) return t
      const merged = { ...t, ...updates, updated_at: now() }
      dbSaveTicket(merged)
      engineRef.current?.queueOp({
        op: 'update', entityType: 'ticket', entityId: id, projId: t.project, data: updates,
      })
      return merged
    }))
  }, [])

  const deleteTicket = useCallback(async (id) => {
    const ticket = tickets.find(t => t.id === id)
    if (!ticket) return
    await dbDeleteTicket(id)
    setTickets(ts => ts.filter(t => t.id !== id))
    await engineRef.current?.queueOp({
      op: 'delete', entityType: 'ticket', entityId: id, projId: ticket.project,
    })
  }, [tickets])

  const handleDrop = useCallback((status) => {
    if (dragging) {
      updateTicket(dragging, { status })
      setDragging(null)
    }
  }, [dragging, updateTicket])

  // ── Settings save ─────────────────────────────────────────────────────────
  const saveSettings = useCallback(async ({ apiUrl }) => {
    await dbSetSetting('api_url', apiUrl)
    configure(apiUrl)
    setModal(null)
    // Trigger a reconnect attempt
    engineRef.current?.stop()
    engineRef.current?.start()
  }, [])

  // ── Derived ───────────────────────────────────────────────────────────────
  const projTickets = tickets.filter(t => t.project === projId)
  const currentProj = projects.find(p => p.id === projId)

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>

      {/* Sidebar */}
      <aside style={{
        width: 220, flexShrink: 0,
        background: 'var(--bg-secondary)',
        borderRight: '1px solid var(--border)',
        display: 'flex', flexDirection: 'column',
      }}>
        <div style={{ padding: '14px 16px 12px', fontSize: 14, fontWeight: 500, borderBottom: '1px solid var(--border)' }}>
          Ticket Store
        </div>

        <div style={{ flex: 1, overflowY: 'auto', padding: '6px 0' }}>
          {projects.map(p => (
            <div
              key={p.id}
              onClick={() => setProjId(p.id)}
              style={{
                padding: '7px 16px', cursor: 'pointer', fontSize: 13,
                display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                borderLeft: `2px solid ${p.id === projId ? 'var(--text-primary)' : 'transparent'}`,
                background: p.id === projId ? 'var(--bg-primary)' : 'transparent',
                color:      p.id === projId ? 'var(--text-primary)' : 'var(--text-secondary)',
                fontWeight: p.id === projId ? 500 : 400,
              }}
            >
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{p.name}</span>
              <span style={{ fontSize: 11, color: 'var(--text-tertiary)', marginLeft: 8, flexShrink: 0 }}>
                {tickets.filter(t => t.project === p.id).length}
              </span>
            </div>
          ))}
        </div>

        <div style={{ padding: '10px 12px', borderTop: '1px solid var(--border)', display: 'flex', flexDirection: 'column', gap: 6 }}>
          <button className="btn" style={{ width: '100%', fontSize: 12 }} onClick={() => setModal({ type: 'project' })}>
            + New project
          </button>
          <button className="btn" style={{ width: '100%', fontSize: 12 }} onClick={() => setModal({ type: 'settings' })}>
            Settings
          </button>
        </div>
      </aside>

      {/* Main */}
      <main style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {/* Topbar */}
        <header style={{
          padding: '10px 20px',
          background: 'var(--bg-secondary)',
          borderBottom: '1px solid var(--border)',
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          flexShrink: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 14, fontWeight: 500 }}>{currentProj?.name ?? '—'}</span>
            {currentProj?.description && (
              <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>{currentProj.description}</span>
            )}
            <SyncBadge status={syncStatus} />
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <button className="btn" style={{ fontSize: 12 }} onClick={() => setModal({ type: 'history' })}>
              History
            </button>
            <button className="btn btn-primary" onClick={() => setModal({ type: 'ticket', defaultStatus: 'open' })}>
              + New ticket
            </button>
          </div>
        </header>

        {/* Board */}
        {projId ? (
          <Board
            tickets={projTickets}
            onDragStart={setDragging}
            onDrop={handleDrop}
            onEdit={t  => setModal({ type: 'ticket', mode: 'edit', ticket: t })}
            onDelete={deleteTicket}
            onAdd={status => setModal({ type: 'ticket', mode: 'create', defaultStatus: status })}
          />
        ) : (
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-tertiary)', fontSize: 13 }}>
            Create a project to get started
          </div>
        )}
      </main>

      {/* Modals */}
      {modal?.type === 'ticket' && (
        <TicketModal
          mode={modal.mode ?? 'create'}
          ticket={modal.ticket}
          defaultStatus={modal.defaultStatus}
          onSave={data => {
            modal.mode === 'edit' ? updateTicket(modal.ticket.id, data) : createTicket(data)
            setModal(null)
          }}
          onDelete={modal.mode === 'edit' ? () => { deleteTicket(modal.ticket.id); setModal(null) } : null}
          onClose={() => setModal(null)}
        />
      )}
      {modal?.type === 'project' && (
        <ProjectModal
          onSave={data => { createProject(data); setModal(null) }}
          onClose={() => setModal(null)}
        />
      )}
      {modal?.type === 'history' && (
        <HistoryModal projId={projId} onClose={() => setModal(null)} />
      )}
      {modal?.type === 'settings' && (
        <SettingsModal
          currentUrl={getBaseURL()}
          onSave={saveSettings}
          onClose={() => setModal(null)}
        />
      )}
    </div>
  )
}
