import { useState } from 'react'

const STATUSES      = ['open', 'in-progress', 'review', 'closed']
const STATUS_LABEL  = { 'open': 'Open', 'in-progress': 'In Progress', 'review': 'Review', 'closed': 'Closed' }
const STATUS_COLOR  = {
  'open':        'var(--color-open)',
  'in-progress': 'var(--color-in-progress)',
  'review':      'var(--color-review)',
  'closed':      'var(--color-closed)',
}
const PRI_COLOR = {
  low:      'var(--color-low)',
  medium:   'var(--color-medium)',
  high:     'var(--color-high)',
  critical: 'var(--color-critical)',
}

function toBoard(tickets) {
  const map = Object.fromEntries(STATUSES.map(s => [s, []]))
  for (const t of tickets) {
    const col = map[t.status] ?? (map[t.status] = [])
    col.push(t)
  }
  return STATUSES.map(s => ({ status: s, tickets: map[s] || [] }))
}

// ── TicketCard ────────────────────────────────────────────────────────────────

function TicketCard({ ticket, onDragStart, onEdit, onDelete }) {
  const pc = PRI_COLOR[ticket.priority] ?? '#888'
  return (
    <div
      draggable
      onDragStart={() => onDragStart(ticket.id)}
      style={{
        margin: '0 8px 6px',
        padding: '10px 11px',
        background: 'var(--bg-primary)',
        border: `1px solid var(--border)`,
        borderLeft: `3px solid ${pc}`,
        borderRadius: 'var(--radius-md)',
        cursor: 'grab',
        userSelect: 'none',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 4 }}>
        <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>{ticket.id}</span>
        <div style={{ display: 'flex', gap: 1 }}>
          <ActionBtn title="Edit"   onClick={() => onEdit(ticket)}>✎</ActionBtn>
          <ActionBtn title="Delete" onClick={() => onDelete(ticket.id)}>×</ActionBtn>
        </div>
      </div>
      <div style={{ fontSize: 13, fontWeight: 500, lineHeight: 1.35, marginBottom: ticket.description ? 4 : 6 }}>
        {ticket.title}
      </div>
      {ticket.description && (
        <div style={{
          fontSize: 12, color: 'var(--text-secondary)', marginBottom: 6,
          overflow: 'hidden', display: '-webkit-box',
          WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', lineHeight: 1.4,
        }}>
          {ticket.description}
        </div>
      )}
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 3, alignItems: 'center' }}>
        <Badge style={{ background: `${pc}22`, color: pc }}>{ticket.priority}</Badge>
        {(ticket.tags ?? []).slice(0, 2).map(tag => (
          <Badge key={tag} style={{ background: 'var(--bg-tertiary)', color: 'var(--text-tertiary)', fontWeight: 400 }}>
            {tag}
          </Badge>
        ))}
        {(ticket.assignees ?? []).length > 0 && (
          <span style={{ marginLeft: 'auto', fontSize: 10, color: 'var(--text-tertiary)' }}>
            {ticket.assignees.map(a => a.toUpperCase()).join(' · ')}
          </span>
        )}
      </div>
    </div>
  )
}

function ActionBtn({ onClick, title, children }) {
  return (
    <button
      title={title}
      onClick={onClick}
      style={{
        background: 'none', border: 'none', cursor: 'pointer',
        fontSize: 13, color: 'var(--text-tertiary)', padding: '0 3px', lineHeight: 1,
      }}
    >
      {children}
    </button>
  )
}

function Badge({ children, style }) {
  return (
    <span style={{ fontSize: 10, padding: '1px 5px', borderRadius: 3, fontWeight: 500, ...style }}>
      {children}
    </span>
  )
}

// ── Column ────────────────────────────────────────────────────────────────────

function Column({ col, onDragStart, onDrop, onEdit, onDelete, onAdd }) {
  const [over, setOver] = useState(false)
  const color = STATUS_COLOR[col.status]

  return (
    <div style={{ width: 260, flexShrink: 0, display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      <div style={{
        padding: '8px 12px',
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        borderBottom: `2px solid ${color}`,
        borderRadius: 'var(--radius-lg) var(--radius-lg) 0 0',
        display: 'flex', alignItems: 'center', gap: 7,
      }}>
        <span style={{ width: 7, height: 7, borderRadius: '50%', background: color, flexShrink: 0 }} />
        <span style={{ fontSize: 13, fontWeight: 500, flex: 1 }}>{STATUS_LABEL[col.status]}</span>
        <span style={{ fontSize: 11, color: 'var(--text-tertiary)', background: 'var(--bg-tertiary)', padding: '1px 6px', borderRadius: 10 }}>
          {col.tickets.length}
        </span>
      </div>

      {/* Body */}
      <div
        onDragOver={e => { e.preventDefault(); setOver(true) }}
        onDragLeave={() => setOver(false)}
        onDrop={() => { onDrop(col.status); setOver(false) }}
        style={{
          flex: 1,
          minHeight: 60,
          maxHeight: 'calc(100vh - 180px)',
          overflowY: 'auto',
          paddingTop: 6,
          paddingBottom: 2,
          border: `1px solid ${over ? 'rgba(59,130,246,0.5)' : 'var(--border)'}`,
          borderTop: 'none',
          borderRadius: '0 0 var(--radius-lg) var(--radius-lg)',
          background: over ? 'rgba(59,130,246,0.04)' : 'transparent',
          transition: 'background 0.12s, border-color 0.12s',
        }}
      >
        {col.tickets.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '20px 0 16px', fontSize: 12, color: 'var(--text-tertiary)' }}>
            Drop tickets here
          </div>
        ) : (
          col.tickets.map(t => (
            <TicketCard key={t.id} ticket={t} onDragStart={onDragStart} onEdit={onEdit} onDelete={onDelete} />
          ))
        )}
      </div>

      {/* Add button */}
      <button
        onClick={onAdd}
        style={{
          marginTop: 4, padding: '5px 0', width: '100%',
          background: 'transparent',
          border: '1px dashed var(--border-hover)',
          borderRadius: 'var(--radius-md)',
          cursor: 'pointer', fontSize: 12, color: 'var(--text-tertiary)',
        }}
      >
        + Add
      </button>
    </div>
  )
}

// ── Board ─────────────────────────────────────────────────────────────────────

export default function Board({ tickets, onDragStart, onDrop, onEdit, onDelete, onAdd }) {
  const cols = toBoard(tickets)
  return (
    <div style={{
      flex: 1,
      overflowX: 'auto',
      overflowY: 'hidden',
      padding: '16px 20px',
      display: 'flex',
      gap: 14,
      alignItems: 'flex-start',
    }}>
      {cols.map(col => (
        <Column
          key={col.status}
          col={col}
          onDragStart={onDragStart}
          onDrop={onDrop}
          onEdit={onEdit}
          onDelete={onDelete}
          onAdd={() => onAdd(col.status)}
        />
      ))}
    </div>
  )
}
