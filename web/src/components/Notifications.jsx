import { useState, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { Fa, icons } from './icons'

/*
 * Reyna Notification System
 * Adapted from Monkeytype's notification architecture
 */

const levelConfig = {
  success: {
    icon: icons.success,
    title: 'Success',
    border: 'var(--notif-success-border)',
    bg: 'var(--notif-success-bg)',
    bgHover: 'var(--notif-success-bg-hover)',
  },
  error: {
    icon: icons.error,
    title: 'Error',
    border: 'var(--notif-error-border)',
    bg: 'var(--notif-error-bg)',
    bgHover: 'var(--notif-error-bg-hover)',
  },
  notice: {
    icon: icons.notice,
    title: 'Notice',
    border: 'var(--notif-notice-border)',
    bg: 'var(--notif-notice-bg)',
    bgHover: 'var(--notif-notice-bg-hover)',
  },
}

let _id = 0
let _historyId = 0
let _notifications = []
let _history = []
let _autoRemoveTimers = new Map()
let _loadingCount = 0

let _notifListeners = []
let _historyListeners = []
let _loadingListeners = []

function _emit(type) {
  if (type === 'notif') _notifListeners.forEach(fn => fn([..._notifications]))
  if (type === 'history') _historyListeners.forEach(fn => fn([..._history]))
  if (type === 'loading') _loadingListeners.forEach(fn => fn(_loadingCount))
}

export const notify = {
  success: (msg, opts) => _addNotification(msg, 'success', opts),
  error: (msg, opts) => _addNotification(msg, 'error', opts),
  notice: (msg, opts) => _addNotification(msg, 'notice', opts),
  showLoader: () => { _loadingCount++; _emit('loading') },
  hideLoader: () => { _loadingCount = Math.max(0, _loadingCount - 1); _emit('loading') },
}

function _addNotification(message, level, options = {}) {
  const config = levelConfig[level] || levelConfig.notice
  const title = options.customTitle ?? config.title
  const durationMs = options.durationMs ?? (level === 'error' ? 0 : 3000)

  _history = [..._history, { id: String(_historyId++), title, message, level, timestamp: Date.now() }]
  if (_history.length > 25) _history = _history.slice(-25)
  _emit('history')

  const newId = _id++
  _notifications = [{ id: newId, message, level, durationMs, customTitle: options.customTitle, customIcon: options.customIcon, important: options.important ?? false, exiting: false }, ..._notifications]
  _emit('notif')

  if (durationMs > 0) {
    const timer = setTimeout(() => {
      _autoRemoveTimers.delete(newId)
      _removeNotification(newId)
    }, durationMs + 250)
    _autoRemoveTimers.set(newId, timer)
  }
  return newId
}

function _removeNotification(notifId) {
  const timer = _autoRemoveTimers.get(notifId)
  if (timer !== undefined) { clearTimeout(timer); _autoRemoveTimers.delete(notifId) }

  const notif = _notifications.find(n => n.id === notifId)
  if (!notif || notif.exiting) return

  notif.exiting = true
  _notifications = [..._notifications]
  _emit('notif')

  setTimeout(() => {
    _notifications = _notifications.filter(n => n.id !== notifId)
    _emit('notif')
  }, 250)
}

function _clearAll() {
  for (const [, t] of _autoRemoveTimers) clearTimeout(t)
  _autoRemoveTimers.clear()
  _notifications = []
  _emit('notif')
}

function useNotifications() {
  const [s, set] = useState(_notifications)
  useEffect(() => { _notifListeners.push(set); return () => { _notifListeners = _notifListeners.filter(l => l !== set) } }, [])
  return s
}
function useHistory() {
  const [s, set] = useState(_history)
  useEffect(() => { _historyListeners.push(set); return () => { _historyListeners = _historyListeners.filter(l => l !== set) } }, [])
  return s
}
function useLoading() {
  const [s, set] = useState(_loadingCount)
  useEffect(() => { _loadingListeners.push(set); return () => { _loadingListeners = _loadingListeners.filter(l => l !== set) } }, [])
  return s > 0
}

// ═══════════════════════════════════════════
// NOTIFICATION ITEM
// ═══════════════════════════════════════════

function NotificationItem({ notification }) {
  const config = levelConfig[notification.level] || levelConfig.notice
  const title = notification.customTitle ?? config.title
  const [hovered, setHovered] = useState(false)

  return (
    <div
      className={notification.exiting ? 'notif-exit' : 'notif-enter'}
      onClick={() => _removeNotification(notification.id)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        marginBottom: notification.exiting ? 0 : 16,
        cursor: 'pointer',
        overflow: 'hidden',
        borderRadius: 'var(--roundness)',
        border: `2px solid ${config.border}`,
        background: hovered ? config.bgHover : config.bg,
        backdropFilter: 'blur(15px)',
        WebkitBackdropFilter: 'blur(15px)',
        color: '#fff',
        userSelect: 'none',
        transition: 'background 0.125s, margin-bottom 0.25s, height 0.25s, opacity 0.25s',
        position: 'relative',
      }}
    >
      <div style={{ padding: 16, fontSize: 13 }}>
        <div style={{ paddingBottom: 8, opacity: 0.5, fontSize: 11, display: 'flex', alignItems: 'center', gap: 8 }}>
          <Fa icon={config.icon} fw style={{ fontSize: 10 }} />
          {title}
        </div>
        <div style={{ lineHeight: 1.5, wordBreak: 'break-word' }}>{notification.message}</div>
      </div>
    </div>
  )
}

// ═══════════════════════════════════════════
// NOTIFICATIONS OVERLAY
// ═══════════════════════════════════════════

function NotificationsOverlay() {
  const notifications = useNotifications()
  const stickyCount = notifications.filter(n => n.durationMs === 0 && !n.exiting).length

  return (
    <div style={{
      position: 'fixed', right: 16, top: 16, zIndex: 99999999,
      width: 350, paddingTop: 4, pointerEvents: 'none',
    }}>
      {stickyCount > 1 && (
        <button onClick={_clearAll} style={{
          color: '#fff', background: 'none', border: 'none', cursor: 'pointer',
          fontSize: 11, marginBottom: 16, width: '100%', textAlign: 'center',
          pointerEvents: 'auto', opacity: 0.7, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
        }}><Fa icon={icons.close} style={{ fontSize: 10 }} /> Clear all</button>
      )}
      {notifications.map(n => (
        <div key={n.id} style={{ pointerEvents: 'auto' }}>
          <NotificationItem notification={n} />
        </div>
      ))}
    </div>
  )
}

// ═══════════════════════════════════════════
// LOADER BAR
// ═══════════════════════════════════════════

function LoaderBar() {
  const isLoading = useLoading()
  if (!isLoading) return null

  return (
    <div style={{ position: 'fixed', top: 0, left: 0, right: 0, height: 3, zIndex: 99999, pointerEvents: 'none' }}>
      <div className="reyna-loader-bar" style={{
        height: '100%', background: '#2563eb', width: '100%',
      }} />
    </div>
  )
}

// ═══════════════════════════════════════════
// ALERTS SIDEBAR
// ═══════════════════════════════════════════

function AlertsSidebar({ open, onClose }) {
  const history = useHistory()
  const reversed = [...history].reverse()

  if (!open) return null

  return createPortal(
    <div onClick={onClose} style={{ position: 'fixed', inset: 0, zIndex: 99999998, background: 'rgba(0,0,0,0.5)' }}>
      <div
        onClick={e => e.stopPropagation()}
        className="alerts-sidebar-enter"
        style={{
          position: 'fixed', right: 0, top: 0, bottom: 0,
          width: 350, maxWidth: 'calc(100vw - 5rem)',
          background: 'var(--sidebar-bg)', padding: '32px 16px 16px',
          overflowY: 'auto', overflowX: 'hidden',
          borderLeft: '1px solid var(--sidebar-border)',
          display: 'flex', flexDirection: 'column', gap: 32,
        }}
      >
        <button onClick={onClose} style={{
          background: '#222', border: 'none', borderRadius: 'var(--roundness)',
          color: '#888', padding: '8px 0', cursor: 'pointer',
          fontSize: 12, fontWeight: 400, width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
        }}><Fa icon={icons.close} style={{ fontSize: 10 }} /> close</button>

        <SidebarSection icon={icons.inbox} title="inbox">
          <div style={{ color: '#555', fontSize: 11, placeSelf: 'center' }}>nothing to show</div>
        </SidebarSection>

        <div style={{ height: 3, borderRadius: 2, background: '#1a1a1a' }} />

        <SidebarSection icon={icons.announce} title="announcements">
          <div style={{ color: '#555', fontSize: 11, placeSelf: 'center' }}>nothing to show</div>
        </SidebarSection>

        <div style={{ height: 3, borderRadius: 2, background: '#1a1a1a' }} />

        <SidebarSection icon={icons.notifHistory} title="notifications">
          {reversed.length === 0 ? (
            <div style={{ color: '#555', fontSize: 11, placeSelf: 'center' }}>nothing to show</div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16, width: '100%' }}>
              {reversed.map(entry => {
                const barColor = entry.level === 'error' ? 'var(--error-color)' : entry.level === 'success' ? 'var(--reyna-accent)' : '#555'
                return (
                  <div key={entry.id} style={{
                    display: 'grid', gridTemplateColumns: '3px 1fr', gap: '4px 8px',
                  }}>
                    <div style={{ borderRadius: 2, gridRow: '1 / 3', background: barColor, transition: 'background 0.125s' }} />
                    <div style={{ fontSize: 10, color: '#555' }}>{entry.title}</div>
                    <div style={{ fontSize: 12, color: '#ccc', wordBreak: 'break-word', lineHeight: 1.5 }}>{entry.message}</div>
                  </div>
                )
              })}
            </div>
          )}
        </SidebarSection>
      </div>
    </div>,
    document.body
  )
}

function SidebarSection({ icon, title, children }) {
  return (
    <div>
      <div style={{ fontSize: 16, fontWeight: 700, color: '#fff', marginBottom: 16, display: 'flex', alignItems: 'center', gap: 10 }}>
        <Fa icon={icon} style={{ fontSize: 14 }} /> {title}
      </div>
      <div style={{ display: 'grid', minHeight: 80, alignItems: 'center', gap: 16 }}>
        {children}
      </div>
    </div>
  )
}

// ═══════════════════════════════════════════
// BELL BUTTON
// ═══════════════════════════════════════════

export function NotificationBell() {
  const [open, setOpen] = useState(false)
  const [hovered, setHovered] = useState(false)
  const history = useHistory()
  const [lastSeen, setLastSeen] = useState(0)
  const unread = history.length - lastSeen

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        style={{
          display: 'flex', alignItems: 'center', gap: 12, padding: '10px 20px',
          fontSize: 15, fontWeight: open ? 600 : 400,
          color: open ? 'var(--reyna-accent)' : hovered ? '#bbb' : '#888',
          background: open ? 'rgba(37,211,102,0.08)' : hovered ? 'rgba(255,255,255,0.03)' : 'transparent',
          borderLeft: open ? '2px solid var(--reyna-accent)' : '2px solid transparent',
          transition: 'all 0.15s',
          border: 'none', cursor: 'pointer', width: '100%',
          textAlign: 'left', position: 'relative', letterSpacing: 0.3,
          fontFamily: 'inherit',
        }}
      >
        <Fa icon={icons.alerts} fw style={{ fontSize: 18, width: 22, textAlign: 'center' }} />
        alerts
        {unread > 0 && (
          <span style={{
            width: 7, height: 7, borderRadius: '50%',
            background: '#2563eb', boxShadow: '0 0 0 2px #111',
            position: 'absolute', left: 30, top: 8,
            transition: '0.125s',
          }} />
        )}
        <i className={`fas fa-chevron-right reyna-nav-arrow ${open ? 'reyna-arrow-active' : ''}`} />
      </button>
      <AlertsSidebar open={open} onClose={() => { setOpen(false); setLastSeen(history.length) }} />
    </>
  )
}

// ═══════════════════════════════════════════
// CONTAINER
// ═══════════════════════════════════════════

export default function NotificationContainer() {
  return createPortal(
    <>
      <LoaderBar />
      <NotificationsOverlay />
    </>,
    document.body
  )
}
