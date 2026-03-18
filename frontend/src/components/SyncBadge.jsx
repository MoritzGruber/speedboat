const CONFIG = {
  offline: { dot: '#9c9a93', label: 'offline',  title: 'Backend unreachable — changes saved locally' },
  syncing: { dot: '#f59e0b', label: 'syncing…', title: 'Syncing with backend' },
  synced:  { dot: '#22c55e', label: 'live',     title: 'Connected and in sync' },
  error:   { dot: '#ef4444', label: 'error',    title: 'Sync error — check console' },
}

export default function SyncBadge({ status }) {
  const { dot, label, title } = CONFIG[status] ?? CONFIG.offline
  return (
    <span
      title={title}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 5,
        fontSize: 11,
        padding: '2px 7px',
        borderRadius: 4,
        background: 'var(--bg-tertiary)',
        color: 'var(--text-tertiary)',
        border: '1px solid var(--border)',
        userSelect: 'none',
      }}
    >
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: dot, flexShrink: 0 }} />
      {label}
    </span>
  )
}
