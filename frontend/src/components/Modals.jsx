import { useState, useEffect } from 'react'
import { api } from '../api.js'

// ── TicketModal ───────────────────────────────────────────────────────────────

const STATUSES   = ['DRAFT', 'Refinement', 'In Progress', 'Done'] // FIX: Match Jira
const PRIORITIES = ['low', 'medium', 'high', 'critical']

// ── Shared primitives ─────────────────────────────────────────────────────────

function Overlay({ onClose, children }) {
  return (
    <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      {children}
    </div>
  )
}

function Field({ label, children }) {
  return (
    <div className="field">
      <label className="field-label">{label}</label>
      {children}
    </div>
  )
}


export function TicketModal({ mode, ticket, defaultStatus, onSave, onDelete, onClose }) {
  const [form, setForm] = useState({
   title:       ticket?.title        ?? '',
    description: ticket?.description  ?? '',
    status:      ticket?.status       ?? defaultStatus ?? 'DRAFT',
    priority:    ticket?.priority     ?? 'medium',
    tags:        (ticket?.tags        ?? []).join(', '),
    assignees:   (ticket?.assignees   ?? []).join(', '),
    estimate_h:  ticket?.estimate_h   ?? '',
  })

  const set = (k, v) => setForm(f => ({ ...f, [k]: v }))

  const save = () => {
    if (!form.title.trim()) return
    onSave({
      ...form,
      tags:       form.tags      ? form.tags.split(',').map(s => s.trim()).filter(Boolean) : [],
      assignees:  form.assignees ? form.assignees.split(',').map(s => s.trim()).filter(Boolean) : [],
      estimate_h: parseFloat(form.estimate_h) || 0,
    })
  }

  return (
    <Overlay onClose={onClose}>
      <div className="modal-box">
        <div className="modal-header">
          <span className="modal-title">{mode === 'create' ? 'New ticket' : `Edit ${ticket?.id}`}</span>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>

        <Field label="Title">
          <input
            value={form.title}
            onChange={e => set('title', e.target.value)}
            placeholder="Ticket title"
            autoFocus
            onKeyDown={e => e.key === 'Enter' && save()}
          />
        </Field>

        <Field label="Description">
          <textarea
            value={form.description}
            onChange={e => set('description', e.target.value)}
            placeholder="Optional…"
            rows={3}
            style={{ resize: 'vertical' }}
          />
        </Field>

        <div className="field-row">
          <Field label="Status">
            <select value={form.status} onChange={e => set('status', e.target.value)}>
              {STATUSES.map(s => <option key={s} value={s}>{s}</option>)}
            </select>
          </Field>
          <Field label="Priority">
            <select value={form.priority} onChange={e => set('priority', e.target.value)}>
              {PRIORITIES.map(p => <option key={p} value={p}>{p}</option>)}
            </select>
          </Field>
        </div>

        <Field label="Tags (comma-separated)">
          <input value={form.tags} onChange={e => set('tags', e.target.value)} placeholder="ui, bug, auth" />
        </Field>

        <Field label="Assignees (comma-separated)">
          <input value={form.assignees} onChange={e => set('assignees', e.target.value)} placeholder="alice, bob" />
        </Field>

        <Field label="Estimate (hours)">
          <input
            type="number" min="0" step="0.5"
            value={form.estimate_h}
            onChange={e => set('estimate_h', e.target.value)}
            placeholder="0"
            style={{ width: '40%' }}
          />
        </Field>

        <div className="modal-footer">
          {onDelete ? (
            <button className="btn btn-danger" onClick={onDelete}>Delete ticket</button>
          ) : <span />}
          <div className="modal-footer-right">
            <button className="btn" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" onClick={save} disabled={!form.title.trim()}>
              {mode === 'create' ? 'Create ticket' : 'Save changes'}
            </button>
          </div>
        </div>
      </div>
    </Overlay>
  )
}

// ── ProjectModal ──────────────────────────────────────────────────────────────

export function ProjectModal({ onSave, onClose }) {
  const [form, setForm] = useState({ id: '', name: '', description: '' })
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }))

  const handleName = v => {
    set('name', v)
    if (!form.id) {
      set('id', v.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 20))
    }
  }

  const valid = form.id.trim() && form.name.trim()

  return (
    <Overlay onClose={onClose}>
      <div className="modal-box" style={{ width: 380 }}>
        <div className="modal-header">
          <span className="modal-title">New project</span>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>

        <Field label="Project name">
          <input
            value={form.name}
            onChange={e => handleName(e.target.value)}
            placeholder="My Project"
            autoFocus
          />
        </Field>
        <Field label="ID (url-safe slug)">
          <input
            value={form.id}
            onChange={e => set('id', e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
            placeholder="my-project"
          />
        </Field>
        <Field label="Description">
          <input value={form.description} onChange={e => set('description', e.target.value)} placeholder="Optional…" />
        </Field>

        <div className="modal-footer">
          <span />
          <div className="modal-footer-right">
            <button className="btn" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" onClick={() => valid && onSave(form)} disabled={!valid}>
              Create project
            </button>
          </div>
        </div>
      </div>
    </Overlay>
  )
}

// ── HistoryModal ──────────────────────────────────────────────────────────────

export function HistoryModal({ projId, onClose }) {
  const [lines, setLines]   = useState(null)
  const [error, setError]   = useState(null)

  useEffect(() => {
    if (!projId) return
    api.getHistory(projId)
      .then(setLines)
      .catch(err => setError(err.message))
  }, [projId])

  return (
    <Overlay onClose={onClose}>
      <div className="modal-box" style={{ width: 560 }}>
        <div className="modal-header">
          <span className="modal-title">Git history — {projId}</span>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>

        <div style={{
          fontFamily: 'monospace', fontSize: 12,
          color: 'var(--text-secondary)',
          background: 'var(--bg-tertiary)',
          borderRadius: 'var(--radius-md)',
          padding: '10px 12px',
          minHeight: 80,
          maxHeight: 360,
          overflowY: 'auto',
          lineHeight: 1.8,
        }}>
          {lines === null && !error && <span style={{ color: 'var(--text-tertiary)' }}>Loading…</span>}
          {error && <span style={{ color: 'var(--color-critical)' }}>Error: {error}</span>}
          {lines?.length === 0 && <span style={{ color: 'var(--text-tertiary)' }}>No commits yet.</span>}
          {lines?.map((l, i) => <div key={i}>{l}</div>)}
        </div>

        <div className="modal-footer">
          <span />
          <button className="btn" onClick={onClose}>Close</button>
        </div>
      </div>
    </Overlay>
  )
}

// ── SettingsModal ─────────────────────────────────────────────────────────────

export function SettingsModal({ currentUrl, onSave, onClose }) {
  const [apiUrl, setApiUrl] = useState(currentUrl)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState(null)

  const testConnection = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      // Temporarily configure the URL just for this test
      const { configure, api } = await import('../api.js')
      configure(apiUrl)
      await api.ping()
      setTestResult({ ok: true, msg: 'Connected successfully' })
    } catch (err) {
      setTestResult({ ok: false, msg: err.message })
    } finally {
      setTesting(false)
    }
  }

  return (
    <Overlay onClose={onClose}>
      <div className="modal-box" style={{ width: 420 }}>
        <div className="modal-header">
          <span className="modal-title">Settings</span>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>

        <Field label="Backend API URL">
          <input
            value={apiUrl}
            onChange={e => { setApiUrl(e.target.value); setTestResult(null) }}
            placeholder="http://localhost:8080"
          />
        </Field>

        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 4 }}>
          <button className="btn" onClick={testConnection} disabled={testing} style={{ flexShrink: 0 }}>
            {testing ? 'Testing…' : 'Test connection'}
          </button>
          {testResult && (
            <span style={{ fontSize: 12, color: testResult.ok ? 'var(--color-closed)' : 'var(--color-critical)' }}>
              {testResult.ok ? '✓' : '✗'} {testResult.msg}
            </span>
          )}
        </div>

        <div style={{ marginTop: 14, fontSize: 12, color: 'var(--text-tertiary)', lineHeight: 1.5 }}>
          Override via <code style={{ fontSize: 11 }}>VITE_API_URL</code> in <code style={{ fontSize: 11 }}>.env.local</code>.
          This setting is persisted in IndexedDB and takes precedence.
        </div>

        <div className="modal-footer">
          <span />
          <div className="modal-footer-right">
            <button className="btn" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" onClick={() => onSave({ apiUrl })}>Save</button>
          </div>
        </div>
      </div>
    </Overlay>
  )
}
