import { useState, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { api, getUser } from '../lib/api'
import { useNavigate } from 'react-router-dom'
import { notify } from '../components/Notifications'
import { Fa, icons, fileIconClass, IconBox } from '../components/icons'

function formatBytes(b) {
  if (!b || b <= 0) return '0 B'
  if (b < 1024) return b + ' B'
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB'
  return (b / 1073741824).toFixed(1) + ' GB'
}

function shortenFilename(name, max = 32) {
  if (!name || name.length <= max) return name
  const dot = name.lastIndexOf('.')
  const ext = dot > 0 && name.length - dot <= 6 ? name.slice(dot) : ''
  const base = ext ? name.slice(0, dot) : name
  const keep = max - ext.length - 1
  if (keep < 6) return name.slice(0, max - 1) + '…'
  const head = Math.ceil(keep * 0.6)
  const tail = keep - head
  return base.slice(0, head) + '…' + base.slice(base.length - tail) + ext
}

function timeAgo(d) {
  const s = Math.floor((Date.now() - new Date(d).getTime()) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

/* ── shared styles ── */
const btnBase = {
  background: '#fff', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)',
  padding: '8px 18px', fontSize: 14, fontWeight: 600, cursor: 'pointer', color: 'var(--btn-text, #444)',
  transition: 'all 0.15s', display: 'inline-flex', alignItems: 'center', gap: 7,
}
const btnPrimary = {
  ...btnBase, background: 'var(--reyna-accent)', color: 'var(--btn-text, #fff)', border: '1.5px solid var(--reyna-accent)', fontWeight: 700,
}
const btnDanger = {
  ...btnBase, background: '#fff', color: 'var(--btn-text, var(--error-color))', border: '1.5px solid #e5c4c4', fontWeight: 700,
}
const bHover = (e, on) => { e.currentTarget.style.borderColor = on ? '#888' : '#ccc'; e.currentTarget.style.boxShadow = on ? '0 2px 8px rgba(0,0,0,0.08)' : 'none' }
const bHoverP = (e, on) => { e.currentTarget.style.borderColor = on ? '#16a34a' : 'var(--reyna-accent)'; e.currentTarget.style.boxShadow = on ? '0 3px 12px rgba(37,211,102,0.25)' : 'none' }
const bHoverD = (e, on) => { e.currentTarget.style.borderColor = on ? '#dc2626' : '#e5c4c4'; e.currentTarget.style.boxShadow = on ? '0 2px 8px rgba(220,38,38,0.12)' : 'none' }
const cardStyle = {
  background: 'var(--card-bg)', border: '1px solid var(--card-border)',
  borderRadius: 'var(--roundness)', padding: 28,
  boxShadow: 'var(--card-shadow)',
}

/* ── FolderTreeItem ── */
function FolderTreeItem({ folder, depth, onRefresh }) {
  const [expanded, setExpanded] = useState(false)
  const [children, setChildren] = useState(null)
  const [showActions, setShowActions] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const [renameTo, setRenameTo] = useState(folder.name)
  const [creatingChild, setCreatingChild] = useState(false)
  const [newChildName, setNewChildName] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [busy, setBusy] = useState(false)

  const toggle = async () => {
    if (!expanded && !children) {
      const data = await api.driveTree(folder.id)
      setChildren(data)
    }
    setExpanded(p => !p)
  }
  const reload = async () => { const data = await api.driveTree(folder.id); setChildren(data) }
  const doRename = async () => {
    if (!renameTo.trim() || renameTo === folder.name) { setRenaming(false); return }
    setBusy(true); await api.renameDriveFolder(folder.id, renameTo.trim()); folder.name = renameTo.trim()
    setBusy(false); setRenaming(false); if (onRefresh) onRefresh()
  }
  const doCreateChild = async () => {
    if (!newChildName.trim()) { setCreatingChild(false); return }
    setBusy(true); await api.createDriveFolder(newChildName.trim(), folder.id, false)
    setNewChildName(''); setCreatingChild(false); setBusy(false); await reload(); setExpanded(true)
  }
  const doDelete = async () => {
    setBusy(true); await api.deleteDriveFolder(folder.id)
    setBusy(false); setConfirmDelete(false); if (onRefresh) onRefresh()
  }

  return (
    <div style={{ marginLeft: depth > 0 ? 16 : 0 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '5px 0', fontSize: 12, color: 'var(--text-color)' }}
        onMouseEnter={() => setShowActions(true)} onMouseLeave={() => setShowActions(false)}>
        <span onClick={toggle} style={{ width: 14, cursor: 'pointer', textAlign: 'center', flexShrink: 0 }}>
          <i className={`fas fa-chevron-right reyna-nav-arrow ${expanded ? 'reyna-arrow-open' : ''}`}
            style={{ fontSize: 8, color: 'var(--sub-color)', marginLeft: 0 }} />
        </span>
        <Fa icon={icons.folder} style={{ fontSize: 12, color: 'var(--main-color)' }} />
        {renaming ? (
          <span style={{ display: 'flex', gap: 4, alignItems: 'center' }}>
            <input value={renameTo} onChange={e => setRenameTo(e.target.value)} autoFocus
              onKeyDown={e => { if (e.key === 'Enter') doRename(); if (e.key === 'Escape') setRenaming(false) }}
              style={{ fontSize: 12, padding: '2px 6px', border: '1px solid var(--main-color)', borderRadius: 4, outline: 'none', width: 140, background: '#fff', color: 'var(--text-color)' }} />
            <button onClick={doRename} disabled={busy} style={{ ...btnPrimary, padding: '2px 8px', fontSize: 10 }}><Fa icon={icons.check} /></button>
            <button onClick={() => { setRenaming(false); setRenameTo(folder.name) }} style={{ ...btnBase, padding: '2px 8px', fontSize: 10 }}><Fa icon={icons.close} /></button>
          </span>
        ) : (
          <span onClick={toggle} style={{ fontWeight: 500, cursor: 'pointer' }}>{folder.name}</span>
        )}
        {showActions && !renaming && (
          <span style={{ display: 'flex', gap: 2, marginLeft: 'auto', flexShrink: 0 }}>
            <button onClick={() => { setCreatingChild(true); setExpanded(true) }} title="New subfolder"
              style={{ ...btnBase, padding: '1px 6px', fontSize: 10 }}><Fa icon={icons.add} style={{ fontSize: 8 }} /> new</button>
            <button onClick={() => { setRenaming(true); setRenameTo(folder.name) }} title="Rename"
              style={{ ...btnBase, padding: '1px 6px', fontSize: 10 }}><Fa icon={icons.edit} style={{ fontSize: 8 }} /></button>
            <button onClick={() => setConfirmDelete(true)} title="Delete"
              style={{ ...btnDanger, padding: '1px 6px', fontSize: 10 }}><Fa icon={icons.delete} style={{ fontSize: 8 }} /></button>
          </span>
        )}
      </div>
      {confirmDelete && (
        <div style={{ marginLeft: 28, padding: '6px 10px', background: 'rgba(220,38,38,0.06)', border: '1px solid rgba(220,38,38,0.12)', borderRadius: 'var(--roundness)', marginBottom: 4, fontSize: 11 }}>
          <span style={{ color: 'var(--error-color)', fontWeight: 500 }}>delete "{folder.name}"?</span>
          <span style={{ color: 'var(--sub-color)', marginLeft: 4 }}>(30d recovery)</span>
          <button onClick={doDelete} disabled={busy} style={{ ...btnDanger, padding: '2px 8px', fontSize: 10, marginLeft: 8 }}>{busy ? '...' : 'delete'}</button>
          <button onClick={() => setConfirmDelete(false)} style={{ ...btnBase, padding: '2px 8px', fontSize: 10, marginLeft: 4 }}>cancel</button>
        </div>
      )}
      {creatingChild && (
        <div style={{ marginLeft: 28, padding: '4px 0', display: 'flex', gap: 4, alignItems: 'center' }}>
          <Fa icon={icons.folder} style={{ fontSize: 11, color: 'var(--main-color)' }} />
          <input value={newChildName} onChange={e => setNewChildName(e.target.value)} autoFocus placeholder="folder name..."
            onKeyDown={e => { if (e.key === 'Enter') doCreateChild(); if (e.key === 'Escape') setCreatingChild(false) }}
            style={{ fontSize: 11, padding: '3px 6px', border: '1px solid var(--main-color)', borderRadius: 4, outline: 'none', width: 140, background: '#fff', color: 'var(--text-color)' }} />
          <button onClick={doCreateChild} disabled={busy} style={{ ...btnPrimary, padding: '2px 8px', fontSize: 10 }}>create</button>
          <button onClick={() => setCreatingChild(false)} style={{ ...btnBase, padding: '2px 8px', fontSize: 10 }}><Fa icon={icons.close} /></button>
        </div>
      )}
      {expanded && children && (
        <div>
          {(children.folders || []).map((f, i) => <FolderTreeItem key={f.id || i} folder={f} depth={depth + 1} onRefresh={reload} />)}
          {(children.files || []).map((f, i) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '3px 0', marginLeft: 28, fontSize: 13, color: 'var(--sub-color)' }}>
              <Fa icon={icons.file} style={{ fontSize: 10 }} /><span>{f.name}</span>
              {f.size && <span style={{ color: 'var(--sub-color)', fontSize: 10 }}>({formatBytes(parseInt(f.size || '0'))})</span>}
            </div>
          ))}
          {(children.folders || []).length === 0 && (children.files || []).length === 0 && (
            <div style={{ marginLeft: 28, fontSize: 13, color: 'var(--sub-color)', padding: '4px 0' }}>empty</div>
          )}
        </div>
      )}
    </div>
  )
}

export default function Dashboard() {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [driveStatus, setDriveStatus] = useState(null)
  const [connecting, setConnecting] = useState(false)
  const [disconnecting, setDisconnecting] = useState(false)
  const [showFolderPicker, setShowFolderPicker] = useState(false)
  const [rootFolders, setRootFolders] = useState([])
  const [loadingFolders, setLoadingFolders] = useState(false)
  const [driveTree, setDriveTree] = useState(null)
  const [showDriveTree, setShowDriveTree] = useState(false)
  const [showDisconnectConfirm, setShowDisconnectConfirm] = useState(false)
  const [newFolderName, setNewFolderName] = useState('')
  const [creatingFolder, setCreatingFolder] = useState(false)
  const [newRootFolderName, setNewRootFolderName] = useState('')
  const [creatingRootFolder, setCreatingRootFolder] = useState(false)
  const [createError, setCreateError] = useState('')
  const [groupSettings, setGroupSettings] = useState([])
  const [stagedFiles, setStagedFiles] = useState([])
  const [committing, setCommitting] = useState(false)
  const [llmStatus, setLlmStatus] = useState(null)
  const user = getUser()
  const navigate = useNavigate()

  const refreshStatus = () => api.googleStatus().then(setDriveStatus).catch(() => {})
  const refreshTree = () => api.driveTree('').then(setDriveTree).catch(() => {})
  const refreshGroups = () => api.allGroupSettings().then(gs => setGroupSettings(gs || [])).catch(() => {})
  const refreshStaged = () => api.files('', 200).then(files => setStagedFiles((files || []).filter(f => f.status === 'staged'))).catch(() => {})

  useEffect(() => {
    notify.showLoader()
    const fetchData = () => {
      api.dashboard().then(d => { setData(d); setLoading(false); notify.hideLoader() }).catch(() => { setLoading(false); notify.hideLoader() })
      api.googleStatus().then(s => {
        setDriveStatus(s)
        if (s?.connected) setConnecting(false)
      }).catch(() => {})
    }
    fetchData(); refreshGroups(); refreshStaged()
    api.llmStatus().then(setLlmStatus).catch(() => {})
    const interval = setInterval(() => { fetchData(); refreshStaged(); refreshGroups() }, 5000)
    const handler = (e) => {
      if (e.data?.type === 'google_auth_success') { setConnecting(false); refreshStatus() }
      if (e.data?.type === 'google_auth_error') setConnecting(false)
    }
    window.addEventListener('message', handler)
    return () => { clearInterval(interval); window.removeEventListener('message', handler) }
  }, [])

  useEffect(() => {
    if (driveStatus?.connected && showDriveTree) {
      refreshTree()
      const t = setInterval(refreshTree, 30000)
      return () => clearInterval(t)
    }
  }, [driveStatus?.connected, showDriveTree])

  const connectDrive = async () => {
    setConnecting(true)
    notify.notice('Connecting Google Drive...')
    const resp = await api.googleConnect()
    if (resp?.url) {
      const popup = window.open(resp.url, 'google_auth', 'width=500,height=600,left=200,top=100')
      // Fast popup-close detection (every 500ms) separate from the
      // slower auth-status poll (every 2s) so the UI resets instantly
      // when the user closes the dialog.
      const closedCheck = setInterval(() => {
        if (popup && popup.closed) {
          clearInterval(closedCheck)
          clearInterval(pollInterval)
          setConnecting(false)
        }
      }, 500)
      const pollInterval = setInterval(async () => {
        try {
          const status = await api.googleStatus()
          if (status?.connected) {
            clearInterval(pollInterval)
            clearInterval(closedCheck)
            setConnecting(false)
            setDriveStatus(status)
            refreshGroups(); refreshStaged()
            notify.success(`Google Drive connected (${status.email})`)
            try { popup?.close() } catch {}
          }
        } catch {}
      }, 2000)
      // Fallback timeout in case everything else fails
      setTimeout(() => { clearInterval(pollInterval); clearInterval(closedCheck); setConnecting(false) }, 120000)
    } else setConnecting(false)
  }
  const disconnectDrive = async () => {
    setDisconnecting(true); await api.googleDisconnect()
    setDriveStatus({ connected: false, email: '', configured: true })
    setDisconnecting(false); setShowDisconnectConfirm(false); setDriveTree(null)
    notify.notice('Google Drive disconnected')
  }
  const openFolderPicker = async () => {
    setLoadingFolders(true); setShowFolderPicker(true); setCreateError('')
    const folders = await api.driveRootFolders()
    setRootFolders(folders || []); setLoadingFolders(false)
  }
  const selectFolder = async (folderId) => {
    await api.changeDriveRoot(folderId); setShowFolderPicker(false)
    refreshStatus(); setDriveTree(null); if (showDriveTree) refreshTree()
  }
  const createAndSelect = async (name) => {
    if (!name?.trim()) return
    setCreatingRootFolder(true); setCreateError('')
    try {
      const resp = await api.createDriveFolder(name.trim(), '', true)
      if (resp?.id) {
        await api.changeDriveRoot(resp.id)
        refreshStatus(); setShowFolderPicker(false)
        if (showDriveTree) refreshTree()
      } else if (resp?.error) {
        setCreateError(resp.error)
      }
    } catch (e) { setCreateError('Failed to create folder. Try disconnecting and reconnecting Drive.') }
    setNewRootFolderName(''); setCreatingRootFolder(false)
  }
  const createTrackingFolder = async (name) => {
    if (!name?.trim()) return
    setCreatingFolder(true)
    const resp = await api.createDriveFolder(name.trim(), '', true)
    if (resp?.id) { await api.changeDriveRoot(resp.id); refreshStatus(); if (showDriveTree) refreshTree() }
    setNewFolderName(''); setCreatingFolder(false)
  }

  const toggleGroupEnabled = async (groupId, enabled) => {
    await api.updateGroupSettings(groupId, { enabled })
    notify.success(enabled ? 'Group enabled' : 'Group disabled')
    refreshGroups()
  }
  const setTrackingMode = async (groupId, mode) => {
    await api.updateGroupSettings(groupId, { tracking_mode: mode })
    notify.success(`Tracking mode: ${mode === 'auto' ? 'Track all files' : 'Reactions only'}`)
    refreshGroups()
  }
  const removeFromStaging = async (fileId) => {
    setStagedFiles(prev => prev.filter(f => f.id !== fileId))
    const resp = await api.removeStaged(fileId)
    if (resp?.removed > 0) notify.success('Removed from staging')
    refreshStaged()
    api.dashboard().then(d => setData(d)).catch(() => {})
  }
  const commitAllStaged = async () => {
    setCommitting(true)
    try {
      const resp = await api.commitStaged()
      if (resp?.committed > 0) {
        notify.success(`${resp.committed} file(s) committed${resp.uploaded > 0 ? `, ${resp.uploaded} pushed to Drive` : ''}`)
      }
    } catch (e) {
      notify.error('Commit failed: ' + e.message)
    }
    setCommitting(false)
    refreshStaged()
    api.dashboard().then(d => setData(d)).catch(() => {})
  }

  function hoursUntilCommit(createdAt) {
    const created = new Date(createdAt).getTime()
    const deadline = created + 24 * 3600 * 1000
    const remaining = Math.max(0, deadline - Date.now())
    const h = Math.floor(remaining / 3600000)
    const m = Math.floor((remaining % 3600000) / 60000)
    if (remaining <= 0) return 'soon'
    return `${h}h ${m}m`
  }

  if (loading) return <div style={{ padding: 40 }} />

  const stats = data?.stats || { total_files: 0, total_groups: 0, total_size: 0, recent_files: [], subject_breakdown: {}, top_contributors: [] }
  const storageUsed = data?.storage_used || 0
  const storageLimit = data?.storage_limit || 15 * 1024 * 1024 * 1024
  const rootName = driveStatus?.drive_root_name || 'Reyna'
  const hasRoot = driveStatus?.drive_root && driveStatus.drive_root !== ''

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1100 }} className="fade-in">
      <div style={{ marginBottom: 40 }}>
        <h1 style={{ fontSize: 48, fontWeight: 900, letterSpacing: -2, marginBottom: 8, color: '#1a1a1a', lineHeight: 1.1 }}>
          welcome back{user?.name ? `, ${user.name.toLowerCase()}` : ''}
        </h1>
        <p style={{ fontSize: 18, color: '#888' }}>here's what reyna has been up to in your repos.</p>
      </div>

      {/* ── stat cards ── */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 32 }}>
        {[
          { label: 'total files', value: stats.total_files, icon: icons.totalFiles, color: '#1a1a1a' },
          { label: 'groups', value: stats.total_groups, icon: icons.groups, color: '#2563eb' },
          { label: 'storage used', value: driveStatus?.connected ? formatBytes(storageUsed) : '–', icon: icons.storage, color: '#d97706' },
          { label: 'drive free', value: driveStatus?.connected ? formatBytes(Math.max(0, storageLimit - storageUsed)) : '–', icon: icons.cloud, color: '#25D366' },
        ].map((s, i) => (
          <div key={i} style={{ ...cardStyle, borderTop: `3px solid ${s.color}` }}>
            <div style={{ marginBottom: 12 }}>
              <Fa icon={s.icon} style={{ fontSize: 22, color: s.color }} />
            </div>
            <div style={{ fontSize: 34, fontWeight: 800, color: '#1a1a1a', letterSpacing: -1.5, lineHeight: 1 }}>{s.value}</div>
            <div style={{ fontSize: 12, color: '#999', fontWeight: 600, letterSpacing: 0.5, marginTop: 8 }}>{s.label}</div>
          </div>
        ))}
      </div>

      {/* ── AI / LLM Status ── */}
      {llmStatus && (
        <div style={{ ...cardStyle, borderTop: `3px solid ${llmStatus.enabled ? '#7F77DD' : '#ddd'}`, marginBottom: 24, padding: '16px 24px', display: 'flex', alignItems: 'center', gap: 14 }}>
          <Fa icon="fa-brain" style={{ fontSize: 20, color: llmStatus.enabled ? '#7F77DD' : '#ccc' }} />
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--text-color)' }}>
              AI Pipeline {llmStatus.enabled ? 'Active' : 'Inactive'}
            </div>
            <div style={{ fontSize: 12, color: 'var(--sub-color)', marginTop: 2 }}>
              {llmStatus.enabled
                ? `Powered by ${llmStatus.provider} — content extraction, classification, NLP retrieval, Notes Q&A`
                : 'Set ANTHROPIC_API_KEY, GEMINI_API_KEY, OPENAI_API_KEY, or XAI_API_KEY to enable AI features'}
            </div>
          </div>
          <span style={{
            padding: '4px 12px', borderRadius: 20, fontSize: 11, fontWeight: 600,
            background: llmStatus.enabled ? '#EEEDFE' : '#f5f5f5',
            color: llmStatus.enabled ? '#534AB7' : '#999',
          }}>{llmStatus.enabled ? llmStatus.provider : 'keyword-only'}</span>
        </div>
      )}

      {/* ── google drive ── */}
      <div style={{
        ...cardStyle,
        borderTop: `3px solid ${driveStatus?.connected ? '#25D366' : '#1a1a1a'}`,
        marginBottom: 24,
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
            <Fa icon={driveStatus?.connected ? icons.cloud : icons.driveConnect} style={{ fontSize: 24, color: driveStatus?.connected ? '#25D366' : '#1a1a1a' }} />
            <div>
              <div style={{ fontSize: 16, fontWeight: 600, color: '#1a1a1a' }}>{driveStatus?.connected ? 'google drive connected' : 'connect google drive'}</div>
              <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>
                {driveStatus?.connected
                  ? <>{driveStatus.email} <Fa icon={icons.arrowRight} style={{ fontSize: 8, margin: '0 4px' }} /> <Fa icon={icons.folder} style={{ fontSize: 10 }} /> {rootName}</>
                  : driveStatus?.configured === false ? 'server needs Google OAuth env vars' : 'connect drive to sync files across devices.'}
              </div>
            </div>
          </div>
          <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexShrink: 0 }}>
            {driveStatus?.connected ? (<>
              <button onClick={openFolderPicker} style={btnBase}><Fa icon={icons.folder} style={{ fontSize: 10 }} /> change folder</button>
              <button onClick={() => setShowDriveTree(p => !p)} style={btnBase}><Fa icon={icons.folderTree} style={{ fontSize: 10 }} /> {showDriveTree ? 'hide tree' : 'view tree'}</button>
              <button onClick={() => setShowDisconnectConfirm(true)} style={btnDanger}>disconnect</button>
            </>) : driveStatus?.configured !== false ? (
              <button onClick={connectDrive} disabled={connecting} style={{ ...btnPrimary, opacity: connecting ? 0.6 : 1 }}>
                {connecting ? <><Fa icon={icons.loading} spin style={{ fontSize: 10 }} /> connecting...</> : <>connect drive <Fa icon={icons.arrowRight} style={{ fontSize: 9 }} /></>}
              </button>
            ) : null}
          </div>
        </div>

        {driveStatus?.connected && !hasRoot && (
          <div style={{ marginTop: 16, padding: 14, background: 'rgba(37,211,102,0.06)', border: '1px solid rgba(37,211,102,0.12)', borderRadius: 'var(--roundness)' }}>
            <div style={{ fontSize: 12, fontWeight: 500, color: '#1a1a1a', marginBottom: 6, display: 'flex', alignItems: 'center', gap: 6 }}>
              <Fa icon={icons.warning} style={{ fontSize: 11 }} /> no tracking folder set
            </div>
            <p style={{ fontSize: 13, color: 'var(--sub-color)', marginBottom: 10 }}>create a new folder in your Google Drive or pick an existing one.</p>
            <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
              <input value={newFolderName} onChange={e => setNewFolderName(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && createTrackingFolder(newFolderName)}
                placeholder="e.g. Reyna" style={{ padding: '7px 10px', border: '1px solid var(--card-border)', borderRadius: 'var(--roundness)', fontSize: 12, outline: 'none', width: 180, background: '#fff', color: 'var(--text-color)' }} />
              <button onClick={() => createTrackingFolder(newFolderName)} disabled={creatingFolder || !newFolderName.trim()} style={{ ...btnPrimary, opacity: creatingFolder ? 0.6 : 1 }}>
                <Fa icon={icons.add} style={{ fontSize: 9 }} /> create
              </button>
              <button onClick={openFolderPicker} style={btnBase} onMouseEnter={e=>bHover(e,1)} onMouseLeave={e=>bHover(e,0)}>pick existing</button>
            </div>
          </div>
        )}

        {showDriveTree && driveStatus?.connected && hasRoot && (
          <div style={{ marginTop: 16, padding: 14, background: 'var(--bg-color)', borderRadius: 'var(--roundness)', border: '1px solid var(--card-border)', maxHeight: 420, overflowY: 'auto' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <Fa icon={icons.folder} style={{ fontSize: 12, color: 'var(--main-color)' }} /> {rootName}
                <span style={{ fontSize: 13, color: 'var(--sub-color)', fontWeight: 400 }}>(live from drive)</span>
              </div>
              <div style={{ display: 'flex', gap: 4 }}>
                <button onClick={() => { const n = prompt('New subfolder name:'); if (n) api.createDriveFolder(n, driveStatus.drive_root, false).then(refreshTree) }}
                  style={{ ...btnBase, padding: '3px 8px', fontSize: 10 }}><Fa icon={icons.add} style={{ fontSize: 8 }} /> new folder</button>
                <button onClick={refreshTree} style={{ ...btnBase, padding: '3px 8px', fontSize: 10 }}><Fa icon={icons.refresh} style={{ fontSize: 8 }} /> refresh</button>
              </div>
            </div>
            {driveTree ? (<div>
              {(driveTree.folders || []).map((f, i) => <FolderTreeItem key={f.id || i} folder={f} depth={0} onRefresh={refreshTree} />)}
              {(driveTree.files || []).map((f, i) => (
                <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '3px 0', fontSize: 13, color: 'var(--sub-color)' }}>
                  <Fa icon={icons.file} style={{ fontSize: 10 }} /><span>{f.name}</span>
                  {f.size && <span style={{ fontSize: 10 }}>({formatBytes(parseInt(f.size || '0'))})</span>}
                </div>
              ))}
              {(driveTree.folders || []).length === 0 && (driveTree.files || []).length === 0 && (
                <div style={{ fontSize: 13, color: 'var(--sub-color)', textAlign: 'center', padding: 16 }}>empty — files appear after <code>/reyna commit</code></div>
              )}
            </div>) : <div style={{ fontSize: 13, color: 'var(--sub-color)', textAlign: 'center', padding: 16 }}><Fa icon={icons.loading} spin /> loading...</div>}
          </div>
        )}
      </div>

      {/* ── staging area + Group Management ── */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(340px, 1fr))', gap: 12, marginBottom: 20 }}>
        {/* staging area */}
        <div style={{ ...cardStyle, minWidth: 0, overflow: 'hidden' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
            <h2 style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.staging} style={{ fontSize: 13 }} /> staging area
            </h2>
            {stagedFiles.length > 0 && (
              <button onClick={commitAllStaged} disabled={committing} style={{ ...btnPrimary, fontSize: 11, opacity: committing ? 0.6 : 1 }}>
                {committing ? <><Fa icon={icons.loading} spin style={{ fontSize: 9 }} /> pushing...</> : <>push all to drive <Fa icon={icons.arrowRight} style={{ fontSize: 9 }} /></>}
              </button>
            )}
          </div>
          {stagedFiles.length === 0 ? (
            <p style={{ fontSize: 13, color: 'var(--sub-color)', textAlign: 'center', padding: 20 }}>no files staged. files appear here when shared in your groups.</p>
          ) : (
            <div>
              <div style={{ fontSize: 13, color: 'var(--sub-color)', marginBottom: 8 }}>auto-commit in 24 hours. push now or remove files below.</div>
              {stagedFiles.slice(0, 6).map((f, i) => (
                <div key={f.id} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '7px 0', borderBottom: i < Math.min(stagedFiles.length, 6) - 1 ? '1px solid var(--card-border)' : 'none' }}>
                  <IconBox icon={icons.staging} color="var(--main-color)" bg="rgba(37,211,102,0.08)" size={30} iconSize={12} />
                  <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                    <div title={f.file_name} style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '100%' }}>{shortenFilename(f.file_name)}</div>
                    <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>{f.subject || 'General'} · {formatBytes(f.file_size)}</div>
                  </div>
                  <div style={{ fontSize: 10, color: 'var(--main-color)', fontWeight: 500, flexShrink: 0 }}>{hoursUntilCommit(f.created_at)}</div>
                  <button onClick={() => removeFromStaging(f.id)} title="Remove from staging" style={{ ...btnBase, padding: '2px 6px', fontSize: 10 }}><Fa icon={icons.close} style={{ fontSize: 8 }} /></button>
                </div>
              ))}
              {stagedFiles.length > 6 && <div style={{ fontSize: 13, color: 'var(--sub-color)', textAlign: 'center', padding: 6 }}>+{stagedFiles.length - 6} more</div>}
            </div>
          )}
        </div>

        {/* Group Management */}
        <div style={{ ...cardStyle, minWidth: 0 }}>
          <h2 style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-color)', marginBottom: 4, display: 'flex', alignItems: 'center', gap: 8 }}>
            <Fa icon={icons.groups} style={{ fontSize: 13 }} /> active groups
          </h2>
          <p style={{ fontSize: 13, color: 'var(--sub-color)', marginBottom: 14 }}>choose which groups reyna monitors and how files are tracked.</p>
          {groupSettings.length === 0 ? (
            <p style={{ fontSize: 13, color: 'var(--sub-color)', textAlign: 'center', padding: 20 }}>no groups yet. send a message in a WhatsApp group where reyna is added.</p>
          ) : (
            <div>
              {groupSettings.map((gs, i) => {
                const enabled = gs.settings?.enabled || false
                const mode = gs.settings?.tracking_mode || 'auto'
                return (
                <div key={i} style={{ padding: '16px 18px', marginBottom: 8, background: 'var(--bg-color)', border: '1px solid var(--card-border)', borderRadius: 'var(--roundness)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
                      <span style={{ fontSize: 15, fontWeight: 600, color: '#1a1a1a', lineHeight: 1 }}>{gs.group?.name || 'WhatsApp Group'}</span>
                      <span style={{ fontSize: 10, fontWeight: 600, color: enabled ? '#1a1a1a' : '#aaa', marginLeft: 8, background: enabled ? 'rgba(0,0,0,0.06)' : '#f3f3f3', padding: '3px 8px', borderRadius: 4, lineHeight: 1, display: 'inline-flex', alignItems: 'center' }}>{enabled ? 'active' : 'inactive'}</span>
                    </div>
                    <label style={{ position: 'relative', display: 'inline-block', width: 36, height: 20, cursor: 'pointer', flexShrink: 0 }}>
                      <input type="checkbox" checked={enabled} onChange={e => toggleGroupEnabled(gs.group?.id, e.target.checked)} style={{ opacity: 0, width: 0, height: 0, position: 'absolute' }} />
                      <span style={{
                        position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, borderRadius: 10,
                        background: enabled ? '#1a1a1a' : 'var(--sub-color)', transition: 'background 0.2s',
                      }}>
                        <span style={{
                          position: 'absolute', width: 16, height: 16, borderRadius: '50%', background: '#fff', top: 2,
                          left: enabled ? 18 : 2, transition: 'left 0.2s', boxShadow: '0 1px 3px rgba(0,0,0,0.15)',
                        }} />
                      </span>
                    </label>
                  </div>
                  <div style={{ display: 'flex', gap: 6, opacity: enabled ? 1 : 0.4, pointerEvents: enabled ? 'auto' : 'none' }}>
                    {[{ value: 'auto', label: 'track all files', icon: icons.trackAll }, { value: 'reaction', label: 'reactions only', icon: icons.trackReact }].map((opt) => (
                      <button key={opt.value} onClick={() => setTrackingMode(gs.group?.id, opt.value)}
                        className="reyna-pill"
                        style={{
                          padding: '6px 14px', fontSize: 12, fontWeight: 500, cursor: 'pointer',
                          borderRadius: 20, display: 'inline-flex', alignItems: 'center', gap: 5,
                          border: mode === opt.value ? '1px solid #1a1a1a' : '1px solid var(--card-border)',
                          background: mode === opt.value ? '#1a1a1a' : '#fff',
                          color: mode === opt.value ? '#fff' : '#888',
                          transition: 'all 0.15s', fontFamily: 'inherit',
                        }}><Fa icon={opt.icon} style={{ fontSize: 9 }} /> {opt.label}</button>
                    ))}
                  </div>
                  {enabled && (
                    <div style={{ fontSize: 12, color: '#aaa', marginTop: 10, lineHeight: 1.5 }}>
                      {mode === 'auto'
                        ? 'every document shared in this group is automatically staged and synced.'
                        : 'only files that get a pin reaction are staged. everything else is ignored.'}
                    </div>
                  )}
                </div>
              )})}
            </div>
          )}
        </div>
      </div>

      {/* Empress upsell removed — AI features are now core Reyna */}

      {/* ── recent files + Stats ── */}
      <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 16 }}>
        <div style={cardStyle}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
            <h2 style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.file} style={{ fontSize: 13 }} /> recent files
            </h2>
            <button onClick={() => navigate('/files')} style={btnBase}>view all <Fa icon={icons.arrowRight} style={{ fontSize: 8 }} /></button>
          </div>
          {(stats.recent_files || []).length === 0 ? (
            <p style={{ fontSize: 13, color: 'var(--sub-color)', textAlign: 'center', padding: 28 }}>no files yet. use /reyna add in your WhatsApp group!</p>
          ) : (stats.recent_files || []).slice(0, 8).map((f, i) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '8px 0', borderBottom: i < 7 ? '1px solid var(--card-border)' : 'none' }}>
              <IconBox icon={fileIconClass(f.mime_type, f.file_name)} color="var(--reyna-accent)" bg="var(--reyna-accent-dim)" size={34} iconSize={13} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{f.file_name}</div>
                <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>{f.subject || 'General'} · v{f.version} · by {f.shared_by_name || 'unknown'}</div>
              </div>
              <div style={{ fontSize: 13, color: 'var(--sub-color)', flexShrink: 0 }}>{timeAgo(f.created_at)}</div>
            </div>
          ))}
        </div>
        <div>
          <div style={{ ...cardStyle, marginBottom: 12 }}>
            <h2 style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-color)', marginBottom: 14, display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.tag} style={{ fontSize: 13 }} /> by subject
            </h2>
            {Object.entries(stats.subject_breakdown || {}).length === 0 ? <p style={{ fontSize: 13, color: 'var(--sub-color)' }}>no data yet</p> :
              Object.entries(stats.subject_breakdown || {}).sort((a, b) => b[1] - a[1]).map(([sub, cnt], i) => (
                <div key={i} style={{ marginBottom: 8 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 3 }}><span style={{ fontWeight: 500, color: 'var(--text-color)' }}>{sub}</span><span style={{ color: 'var(--sub-color)' }}>{cnt}</span></div>
                  <div style={{ height: 4, background: 'var(--bg-color)', borderRadius: 2 }}><div style={{ height: '100%', borderRadius: 2, background: 'var(--main-color)', width: `${(cnt / stats.total_files) * 100}%`, transition: 'width 0.5s' }} /></div>
                </div>
              ))}
          </div>
          <div style={cardStyle}>
            <h2 style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-color)', marginBottom: 14, display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.trophy} style={{ fontSize: 13 }} /> top contributors
            </h2>
            {(stats.top_contributors || []).length === 0 ? <p style={{ fontSize: 13, color: 'var(--sub-color)' }}>no contributors yet</p> :
              (stats.top_contributors || []).map((c, i) => (
                <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '6px 0', borderBottom: '1px solid var(--card-border)' }}>
                  <div style={{ width: 24, height: 24, borderRadius: '50%', background: i === 0 ? '#1a1a1a' : i === 1 ? '#2563eb' : '#e8e6e1', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, fontWeight: 700, color: i < 2 ? '#fff' : '#777' }}>{i + 1}</div>
                  <div style={{ flex: 1 }}><div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)' }}>{c.name || c.phone}</div></div>
                  <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--reyna-accent)' }}>{c.count} files</div>
                </div>
              ))}
          </div>
        </div>
      </div>

      {/* Disconnect Confirm — portal */}
      {showDisconnectConfirm && createPortal(
        <div onClick={() => setShowDisconnectConfirm(false)} style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 99999 }}>
          <div onClick={e => e.stopPropagation()} style={{ background: '#fff', borderRadius: '12px', padding: 32, width: 420, maxWidth: '90vw', border: '1px solid var(--card-border)', boxShadow: '0 25px 60px rgba(0,0,0,0.15), 0 8px 20px rgba(0,0,0,0.1)' }}>
            <h3 style={{ fontSize: 20, fontWeight: 700, letterSpacing: -0.3, marginBottom: 14, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.warning} style={{ fontSize: 14 }} /> Disconnect Google Drive?
            </h3>
            <p style={{ fontSize: 15, color: 'var(--text-secondary, #555)', lineHeight: 1.6, marginBottom: 24 }}>
              disconnecting <strong style={{ color: 'var(--text-color)' }}>{driveStatus?.email}</strong>. Files already in Drive stay. You can connect a different account after.
            </p>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button onClick={() => setShowDisconnectConfirm(false)} style={btnBase} onMouseEnter={e=>bHover(e,1)} onMouseLeave={e=>bHover(e,0)}>cancel</button>
              <button onClick={disconnectDrive} disabled={disconnecting} style={{ ...btnDanger, opacity: disconnecting ? 0.6 : 1 }}>
                {disconnecting ? 'disconnecting...' : 'disconnect'}
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}

      {/* Folder Picker — portal */}
      {showFolderPicker && createPortal(
        <div onClick={() => setShowFolderPicker(false)} style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 99999, padding: 24 }}>
          <div onClick={e => e.stopPropagation()} style={{ background: '#fff', borderRadius: '12px', padding: 32, width: 520, maxWidth: '95vw', maxHeight: '85vh', overflowY: 'auto', border: '1px solid var(--card-border)', boxShadow: '0 25px 60px rgba(0,0,0,0.15), 0 8px 20px rgba(0,0,0,0.1)' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 16 }}>
              <div>
                <h3 style={{ fontSize: 18, fontWeight: 700, marginBottom: 4, color: '#1a1a1a', display: 'flex', alignItems: 'center', gap: 8 }}>
                  <Fa icon={icons.folder} style={{ fontSize: 14 }} /> Select Tracking Folder
                </h3>
                <p style={{ fontSize: 13, color: 'var(--sub-color)' }}>Pick any folder from your Google Drive, or create a new one.</p>
              </div>
              <button onClick={() => setShowFolderPicker(false)} style={{ ...btnBase, borderRadius: '50%', width: 28, height: 28, padding: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Fa icon={icons.close} style={{ fontSize: 10 }} />
              </button>
            </div>
            <div style={{ display: 'flex', gap: 8, marginBottom: 16, padding: 12, background: 'rgba(37,211,102,0.06)', borderRadius: 'var(--roundness)', border: '1px solid rgba(37,211,102,0.15)' }}>
              <input value={newRootFolderName} onChange={e => setNewRootFolderName(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && createAndSelect(newRootFolderName)}
                placeholder="new folder name..." style={{ flex: 1, padding: '8px 10px', border: '1px solid var(--card-border)', borderRadius: 'var(--roundness)', fontSize: 12, outline: 'none', background: '#fff', color: 'var(--text-color)' }} />
              <button onClick={() => createAndSelect(newRootFolderName)} disabled={creatingRootFolder || !newRootFolderName.trim()}
                style={{ ...btnPrimary, whiteSpace: 'nowrap', opacity: creatingRootFolder ? 0.6 : 1 }}>
                {creatingRootFolder ? <><Fa icon={icons.loading} spin style={{ fontSize: 9 }} /> creating...</> : <><Fa icon={icons.add} style={{ fontSize: 9 }} /> create & select</>}
              </button>
            </div>
            {createError && <div style={{ fontSize: 11, color: 'var(--error-color)', background: 'rgba(220,38,38,0.06)', padding: '6px 10px', borderRadius: 'var(--roundness)', marginBottom: 10 }}>{createError}</div>}
            {loadingFolders ? (
              <div style={{ textAlign: 'center', padding: 28, color: 'var(--sub-color)', fontSize: 12 }}><Fa icon={icons.loading} spin /> loading your drive folders...</div>
            ) : rootFolders.length === 0 ? (
              <div style={{ textAlign: 'center', padding: 28, color: 'var(--sub-color)' }}>
                <p style={{ fontSize: 12, marginBottom: 6 }}>no folders visible from your drive.</p>
                <p style={{ fontSize: 10 }}>if you just connected, try disconnecting and reconnecting.</p>
              </div>
            ) : (<div>
              <div style={{ fontSize: 9, fontWeight: 600, color: 'var(--sub-color)', marginBottom: 10, textTransform: 'uppercase', letterSpacing: 1.5 }}>your drive folders</div>
              {rootFolders.map((f, i) => (
                <div key={i} style={{
                  display: 'flex', alignItems: 'center', gap: 12, padding: '12px 14px', borderBottom: '1px solid var(--card-border)',
                  cursor: 'pointer', borderRadius: 'var(--roundness)', transition: 'background 0.15s',
                  background: driveStatus?.drive_root === f.id ? 'rgba(37,211,102,0.06)' : 'transparent',
                }}
                  onMouseEnter={e => e.currentTarget.style.background = 'rgba(37,211,102,0.06)'}
                  onMouseLeave={e => e.currentTarget.style.background = driveStatus?.drive_root === f.id ? 'rgba(37,211,102,0.06)' : 'transparent'}
                >
                  <Fa icon={icons.folder} style={{ fontSize: 16, color: 'var(--main-color)' }} />
                  <div style={{ flex: 1 }}>
                    <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)' }}>{f.name}</div>
                    {driveStatus?.drive_root === f.id && <div style={{ fontSize: 10, color: 'var(--reyna-accent)', fontWeight: 400 }}>currently active</div>}
                  </div>
                  {driveStatus?.drive_root === f.id ? (
                    <span style={{ fontSize: 11, color: 'var(--reyna-accent)', fontWeight: 500, background: 'var(--reyna-accent-dim)', padding: '3px 10px', borderRadius: 'var(--roundness)', display: 'flex', alignItems: 'center', gap: 4 }}>
                      <Fa icon={icons.check} style={{ fontSize: 9 }} /> active
                    </span>
                  ) : (
                    <button onClick={() => selectFolder(f.id)} style={{ ...btnBase, color: 'var(--reyna-accent)', border: '1px solid rgba(37,211,102,0.25)' }}>
                      select <Fa icon={icons.arrowRight} style={{ fontSize: 8 }} />
                    </button>
                  )}
                </div>
              ))}
            </div>)}
          </div>
        </div>,
        document.body
      )}
    </div>
  )
}
