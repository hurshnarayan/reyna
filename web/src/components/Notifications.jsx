import { useState, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { Fa, icons } from './icons'

/*
 * Reyna Notification System — Bottom-Up Stacking Toasts & Task Tracker
 * Replicated from Monkeytype notification architecture.
 */

const levelConfig = {
  loading: {
    icon: 'fa-circle-notch',
    spin: true,
    title: 'pushing',
    border: 'rgba(59, 130, 246, 0.35)',
    bg: '#0f172a',
    bgHover: '#1e293b',
    color: '#f8fafc',
    iconColor: '#60a5fa',
  },
  success: {
    icon: icons.success,
    spin: false,
    title: 'success',
    border: 'rgba(34, 197, 94, 0.35)',
    bg: '#14532d',
    bgHover: '#166534',
    color: '#f0fdf4',
    iconColor: '#4ade80',
  },
  error: {
    icon: icons.error,
    spin: false,
    title: 'error',
    border: 'rgba(239, 68, 68, 0.35)',
    bg: '#7f1d1d',
    bgHover: '#991b1b',
    color: '#fef2f2',
    iconColor: '#f87171',
  },
  notice: {
    icon: icons.notice,
    spin: false,
    title: 'notice',
    border: 'rgba(148, 163, 184, 0.25)',
    bg: '#1e293b',
    bgHover: '#334155',
    color: '#f1f5f9',
    iconColor: '#94a3b8',
  },
}

let _id = 0
let _historyId = 0
let _notifications = []
let _history = []
const _autoRemoveTimers = new Map()
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
  success: (msg, opts) => _addNotification(msg, 'success', { durationMs: 3500, ...opts }),
  error: (msg, opts) => _addNotification(msg, 'error', { durationMs: 5000, ...opts }),
  notice: (msg, opts) => _addNotification(msg, 'notice', { durationMs: 3500, ...opts }),
  info: (msg, opts) => _addNotification(msg, 'notice', { durationMs: 3500, ...opts }),
  loading: (msg, opts) => _addNotification(msg, 'loading', { durationMs: 0, persistent: true, ...opts }),
  update: (id, updates) => _updateNotification(id, updates),
  dismiss: (id) => _removeNotification(id),

  /**
   * Runs an async task while showing a persistent loading toast at the bottom,
   * transitioning in-place to success or error when completed.
   */
  task: async (promiseOrFn, { loading, success, error } = {}) => {
    const notifId = notify.loading(typeof loading === 'string' ? loading : 'pushing staged files to google drive...')
    try {
      const result = typeof promiseOrFn === 'function' ? await promiseOrFn() : await promiseOrFn
      const successMsg = typeof success === 'function' ? success(result) : (success || 'completed successfully')
      notify.update(notifId, { message: successMsg, level: 'success', durationMs: 3500 })
      return result
    } catch (err) {
      const errorMsg = typeof error === 'function' ? error(err) : (error || err.message || 'operation failed')
      notify.update(notifId, { message: errorMsg, level: 'error', durationMs: 5000 })
      throw err
    }
  },

  showLoader: () => { _loadingCount++; _emit('loading') },
  hideLoader: () => { _loadingCount = Math.max(0, _loadingCount - 1); _emit('loading') },
}

function _addNotification(message, level, options = {}) {
  const config = levelConfig[level] || levelConfig.notice
  const title = options.customTitle ?? config.title
  const durationMs = options.durationMs !== undefined ? options.durationMs : (level === 'loading' ? 0 : level === 'error' ? 5000 : 3500)

  _history = [..._history, { id: String(_historyId++), title, message, level, timestamp: Date.now() }]
  if (_history.length > 25) _history = _history.slice(-25)
  _emit('history')

  const newId = ++_id
  const item = {
    id: newId,
    message,
    level,
    durationMs,
    customTitle: options.customTitle,
    customIcon: options.customIcon,
    exiting: false,
    persistent: options.persistent ?? (durationMs === 0),
  }

  // Prepend so the newest notification is at index 0 (bottom of the reverse column stack)
  _notifications = [item, ..._notifications]
  _emit('notif')

  if (durationMs > 0) {
    const timer = setTimeout(() => {
      _autoRemoveTimers.delete(newId)
      _removeNotification(newId)
    }, durationMs + 220)
    _autoRemoveTimers.set(newId, timer)
  }
  return newId
}

function _updateNotification(notifId, updates = {}) {
  const existingTimer = _autoRemoveTimers.get(notifId)
  if (existingTimer !== undefined) {
    clearTimeout(existingTimer)
    _autoRemoveTimers.delete(notifId)
  }

  const notif = _notifications.find(n => n.id === notifId)
  if (!notif) return

  if (updates.message !== undefined) notif.message = updates.message
  if (updates.level !== undefined) notif.level = updates.level
  if (updates.customTitle !== undefined) notif.customTitle = updates.customTitle
  if (updates.customIcon !== undefined) notif.customIcon = updates.customIcon
  notif.exiting = false

  const durationMs = updates.durationMs !== undefined
    ? updates.durationMs
    : (notif.level === 'loading' ? 0 : notif.level === 'error' ? 5000 : 3500)
  notif.durationMs = durationMs

  _history = [..._history, { id: String(_historyId++), title: notif.customTitle || notif.level, message: notif.message, level: notif.level, timestamp: Date.now() }]
  if (_history.length > 25) _history = _history.slice(-25)
  _emit('history')

  _notifications = [..._notifications]
  _emit('notif')

  if (durationMs > 0) {
    const timer = setTimeout(() => {
      _autoRemoveTimers.delete(notifId)
      _removeNotification(notifId)
    }, durationMs + 220)
    _autoRemoveTimers.set(notifId, timer)
  }
}

function _removeNotification(notifId) {
  const timer = _autoRemoveTimers.get(notifId)
  if (timer !== undefined) {
    clearTimeout(timer)
    _autoRemoveTimers.delete(notifId)
  }

  const notif = _notifications.find(n => n.id === notifId)
  if (!notif || notif.exiting) return

  notif.exiting = true
  _notifications = [..._notifications]
  _emit('notif')

  setTimeout(() => {
    _notifications = _notifications.filter(n => n.id !== notifId)
    _emit('notif')
  }, 220)
}

function _clearAll() {
  for (const [, t] of _autoRemoveTimers) clearTimeout(t)
  _autoRemoveTimers.clear()
  _notifications = []
  _emit('notif')
}

function useNotifications() {
  const [s, set] = useState(_notifications)
  useEffect(() => {
    _notifListeners.push(set)
    return () => { _notifListeners = _notifListeners.filter(l => l !== set) }
  }, [])
  return s
}

function useHistory() {
  const [s, set] = useState(_history)
  useEffect(() => {
    _historyListeners.push(set)
    return () => { _historyListeners = _historyListeners.filter(l => l !== set) }
  }, [])
  return s
}

function useLoading() {
  const [s, set] = useState(_loadingCount)
  useEffect(() => {
    _loadingListeners.push(set)
    return () => { _loadingListeners = _loadingListeners.filter(l => l !== set) }
  }, [])
  return s > 0
}

// ═══════════════════════════════════════════
// NOTIFICATION TOAST (BOTTOM-UP STACKED)
// ═══════════════════════════════════════════

function NotificationToast({ notification }) {
  const config = levelConfig[notification.level] || levelConfig.notice
  const [hovered, setHovered] = useState(false)

  return (
    <div
      className={notification.exiting ? 'reyna-toast-exit' : 'reyna-toast-enter'}
      onClick={() => _removeNotification(notification.id)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        pointerEvents: 'auto',
        cursor: 'pointer',
        background: hovered ? config.bgHover : config.bg,
        border: `1px solid ${config.border}`,
        borderRadius: 12,
        padding: '11px 18px',
        boxShadow: '0 10px 25px -4px rgba(0, 0, 0, 0.35), 0 4px 10px -2px rgba(0, 0, 0, 0.2)',
        backdropFilter: 'blur(16px)',
        WebkitBackdropFilter: 'blur(16px)',
        color: config.color,
        display: 'inline-flex',
        alignItems: 'center',
        gap: 12,
        width: 'max-content',
        maxWidth: '100%',
        fontSize: 13,
        fontWeight: 600,
        textTransform: 'lowercase',
        letterSpacing: '-0.01em',
        userSelect: 'none',
        transition: 'background 0.15s, border-color 0.15s',
      }}
    >
      <div style={{
        fontSize: 13,
        color: config.iconColor,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
      }}>
        <Fa icon={notification.customIcon || config.icon} spin={config.spin} />
      </div>

      <div style={{
        lineHeight: 1.4,
        wordBreak: 'break-word',
        flex: 1,
      }}>
        {notification.message}
      </div>

      <button
        onClick={(e) => {
          e.stopPropagation()
          _removeNotification(notification.id)
        }}
        title="dismiss"
        style={{
          background: 'none',
          border: 'none',
          color: 'rgba(255, 255, 255, 0.5)',
          cursor: 'pointer',
          padding: '2px 4px',
          fontSize: 11,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderRadius: 4,
          transition: 'color 0.15s',
          marginLeft: 4,
          flexShrink: 0,
        }}
        onMouseEnter={e => e.currentTarget.style.color = '#ffffff'}
        onMouseLeave={e => e.currentTarget.style.color = 'rgba(255, 255, 255, 0.5)'}
      >
        <Fa icon={icons.close} />
      </button>
    </div>
  )
}

// ═══════════════════════════════════════════
// BOTTOM NOTIFICATIONS OVERLAY (STACKED VERTICALLY)
// ═══════════════════════════════════════════

function NotificationsOverlay() {
  const notifications = useNotifications()
  if (notifications.length === 0) return null

  return (
    <div style={{
      position: 'fixed',
      bottom: 24,
      left: '50%',
      transform: 'translateX(-50%)',
      zIndex: 99999999,
      display: 'flex',
      flexDirection: 'column-reverse', // Newest at bottom, subsequent stack cleanly upward
      alignItems: 'center',
      gap: 10,
      pointerEvents: 'none',
      width: '100%',
      maxWidth: 'min(540px, calc(100vw - 32px))',
    }}>
      {notifications.map(n => (
        <NotificationToast key={n.id} notification={n} />
      ))}
    </div>
  )
}

// ═══════════════════════════════════════════
// TOP LOADER BAR
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
// ALERTS SIDEBAR (HISTORY VIEW)
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
// ROOT CONTAINER
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
