import { useState, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { api, getToken } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa, icons, fileIconClass, IconBox } from '../components/icons'
import { useJobs } from '../components/BackgroundJobs'

function formatBytes(b) {
  if (b < 1024) return b + ' B'
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'
  return (b / 1048576).toFixed(1) + ' MB'
}

function displayName(file) {
  const name = file.shared_by_name || ''
  const phone = file.shared_by_phone || ''
  const hasRealName = name && !/^\+?\d[\d\s\-()]*$/.test(name.trim())
  const digits = phone.replace(/\D/g, '')
  const hasRealPhone = digits.length >= 10
  const lastFour = hasRealPhone ? '·' + digits.slice(-4) : ''
  if (hasRealName && lastFour) return `${name} (${lastFour})`
  if (hasRealName) return name
  if (lastFour) return '···' + digits.slice(-4)
  return '—'
}

function timeAgo(d) {
  const s = Math.floor((Date.now() - new Date(d).getTime()) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

const subjectColors = {
  DSA: '#25D366', OS: '#0ea5e9', CN: '#e2b714', DBMS: '#8b5cf6',
  DAA: '#ca4754', COA: '#ec4899', General: '#646669', Uncategorized: '#646669',
}

function canPreview(mimeType, fileName) {
  const m = (mimeType || '').toLowerCase()
  const ext = (fileName || '').split('.').pop()?.toLowerCase()
  if (m.includes('pdf') || ext === 'pdf') return 'pdf'
  if (m.includes('image') || ['png','jpg','jpeg','gif','webp','svg'].includes(ext)) return 'image'
  if (m.includes('text') || ['txt','md','csv','json','js','py','go','html','css'].includes(ext)) return 'text'
  return null
}

/* Shared styles */
const btnBase = {
  background: '#fff', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)',
  padding: '8px 16px', fontSize: 14, fontWeight: 600, cursor: 'pointer', color: 'var(--btn-text, #555)',
  transition: 'all 0.15s', display: 'inline-flex', alignItems: 'center', gap: 6,
  boxShadow: '0 1px 2px rgba(0,0,0,0.04)',
}
const btnPrimary = { ...btnBase, background: 'var(--reyna-accent)', color: 'var(--btn-text, #fff)', border: '1.5px solid var(--reyna-accent)', fontWeight: 700, boxShadow: '0 1px 3px rgba(37,211,102,0.3)' }
const btnDanger = { ...btnBase, background: '#fef2f2', color: 'var(--btn-text, var(--error-color))', border: '1.5px solid rgba(220,38,38,0.15)' }
const cardStyle = { background: 'var(--card-bg)', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)', padding: 24, boxShadow: 'var(--card-shadow)' }
const iconBtnStyle = (bg, color, border) => ({
  background: bg, border: `1px solid ${border}`, borderRadius: 'var(--roundness)', width: 30, height: 30,
  cursor: 'pointer', fontSize: 12, display: 'flex', alignItems: 'center', justifyContent: 'center', color,
  boxShadow: '0 1px 2px rgba(0,0,0,0.04)',
})

export default function Files() {
  const [files, setFiles] = useState([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('all')
  const [selectedFile, setSelectedFile] = useState(null)
  const [versions, setVersions] = useState([])
  const [viewMode, setViewMode] = useState('list')
  const [previewFile, setPreviewFile] = useState(null)
  const [previewData, setPreviewData] = useState(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [deleting, setDeleting] = useState(false)
  const [selected, setSelected] = useState(new Set())
  const [showBatchDelete, setShowBatchDelete] = useState(false)
  const [sortBy, setSortBy] = useState('date')
  const [sortOrder, setSortOrder] = useState('desc')
  // Sync/push status comes from the global JobsProvider, so leaving this
  // page (or never visiting it) doesn't interrupt work that's in flight.
  // The button mirrors whatever the global ingest-drive job reports.
  const { jobs, refresh: refreshJobs } = useJobs()
  const ingestJob = jobs.ingest_drive
  const syncing = ingestJob?.state === 'running'

  // Kicks off the async Drive ingest. Server returns immediately with a
  // job handle; the JobsProvider starts polling and the sticky pill
  // appears globally. Survives tab switches — the goroutine on the
  // server doesn't know (or care) that we navigated away.
  const handleSyncDrive = async () => {
    if (syncing) return
    notify.success('syncing from drive in the background')
    const res = await api.syncFromDrive()
    if (!res) return
    refreshJobs()
    // Refetch files every ~3s while sync is running so newly-ingested
    // files start appearing in the list without needing a reload.
    const poller = setInterval(async () => {
      const s = await api.jobsStatus()
      if (!s?.ingest_drive || s.ingest_drive.state !== 'running') {
        clearInterval(poller)
        fetchFiles()
        return
      }
      fetchFiles()
    }, 3000)
  }

  const fetchFiles = () => api.files('', 200, sortBy, sortOrder).then(f => { setFiles(f || []); setLoading(false); notify.hideLoader() })

  useEffect(() => { notify.showLoader(); fetchFiles(); const interval = setInterval(fetchFiles, 5000); return () => clearInterval(interval) }, [sortBy, sortOrder])

  const staged = files.filter(f => f.status === 'staged')
  const committed = files.filter(f => f.status === 'committed' || !f.status)
  const subjects = ['all', ...new Set(files.map(f => f.subject || 'General'))]
  const filtered = filter === 'all' ? files : files.filter(f => (f.subject || 'General') === filter)

  const showVersions = async (f) => { setSelectedFile(f); const v = await api.versions(f.id); setVersions(v || []) }

  const openPreview = async (f) => {
    setPreviewFile(f); setPreviewLoading(true); setPreviewData(null)
    const type = canPreview(f.mime_type, f.file_name)
    const hasDriveId = f.drive_file_id && !f.drive_file_id.startsWith('local_') && !f.drive_file_id.startsWith('meta_')
    if (hasDriveId) {
      setPreviewData({ type: 'drive', url: `https://drive.google.com/file/d/${f.drive_file_id}/preview`, driveUrl: `https://drive.google.com/file/d/${f.drive_file_id}/view` })
      setPreviewLoading(false); return
    }
    try {
      const resp = await fetch(api.downloadUrl(f.id), { headers: { 'Authorization': `Bearer ${getToken()}` } })
      const contentType = resp.headers.get('content-type') || ''
      if (contentType.includes('json')) {
        const info = await resp.json()
        if (info.preview_url || info.drive_url) { setPreviewData({ type: 'drive', url: info.preview_url || info.drive_url, driveUrl: info.drive_url }) }
        else { setPreviewData({ type: 'not_available' }) }
      } else if (type === 'image') { const blob = await resp.blob(); setPreviewData({ type: 'image', url: URL.createObjectURL(blob) }) }
      else if (type === 'text') { const text = await resp.text(); setPreviewData({ type: 'text', content: text }) }
      else if (type === 'pdf') { const blob = await resp.blob(); setPreviewData({ type: 'pdf', url: URL.createObjectURL(blob) }) }
      else { setPreviewData({ type: 'not_available' }) }
    } catch { setPreviewData({ type: 'not_available' }) }
    setPreviewLoading(false)
  }

  const downloadFile = (f) => {
    if (f.drive_file_id && !f.drive_file_id.startsWith('local_') && !f.drive_file_id.startsWith('meta_')) { window.open(`https://drive.google.com/file/d/${f.drive_file_id}/view`, '_blank'); return }
    fetch(api.downloadUrl(f.id), { headers: { 'Authorization': `Bearer ${getToken()}` } })
      .then(r => r.blob()).then(blob => { const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = f.file_name; a.click(); URL.revokeObjectURL(url) })
  }

  const confirmDelete = (f) => setDeleteTarget(f)
  const doDelete = async () => {
    if (!deleteTarget) return; setDeleting(true)
    await api.deleteFile(deleteTarget.id); setFiles(prev => prev.filter(f => f.id !== deleteTarget.id))
    notify.success(`Deleted ${deleteTarget.file_name}`); setDeleteTarget(null); setDeleting(false)
  }
  const removeStaged = async (fileId) => {
    setFiles(prev => prev.filter(p => p.id !== fileId)); const resp = await api.removeStaged(fileId)
    if (resp?.removed > 0) notify.success('Removed from staging'); fetchFiles()
  }
  const removeAllStaged = async () => {
    setFiles(prev => prev.filter(f => f.status !== 'staged')); const resp = await api.removeStagedAll()
    if (resp?.removed > 0) notify.success(`${resp.removed} file(s) removed`); fetchFiles()
  }
  // Push-to-Drive reads from the global job registry so navigating away
  // mid-push doesn't stop the upload. Locally we also hold a short-lived
  // "just-clicked" flag so the button reflects activity immediately even
  // when the backend finishes faster than the next poll (a real case for
  // zero-staged pushes — the goroutine completes in milliseconds and the
  // job is already 'done' by the time JobsProvider polls again).
  const pushJob = jobs.push_staged
  const [pushLocal, setPushLocal] = useState(false)
  const committing = pushLocal || pushJob?.state === 'running'
  const commitAllStaged = async () => {
    if (committing) return
    if (staged.length === 0) {
      notify.error('nothing to push — staging area is empty')
      return
    }
    setPushLocal(true)
    notify.success(`pushing ${staged.length} file${staged.length === 1 ? '' : 's'} to drive...`)
    try {
      await api.commitStaged()
      refreshJobs()
      // Watchdog: poll for completion and drop the local flag once either
      // (a) the job ends or (b) we haven't seen a running state in 3
      // consecutive polls (meaning it finished before we could see it).
      let seenRunning = 0, notRunning = 0
      const poller = setInterval(async () => {
        const s = await api.jobsStatus()
        const state = s?.push_staged?.state
        if (state === 'running') seenRunning++
        else notRunning++
        fetchFiles()
        if (state && state !== 'running') {
          clearInterval(poller); setPushLocal(false)
        } else if (notRunning > 3 && seenRunning === 0) {
          // Push was too fast to observe — drop the flag.
          clearInterval(poller); setPushLocal(false)
        }
      }, 1500)
    } catch (e) {
      setPushLocal(false)
      notify.error('commit failed: ' + e.message)
    }
  }
  function hoursUntilCommit(createdAt) {
    const created = new Date(createdAt).getTime(); const remaining = Math.max(0, created + 86400000 - Date.now())
    const h = Math.floor(remaining / 3600000); const m = Math.floor((remaining % 3600000) / 60000)
    if (remaining <= 0) return 'soon'; return `${h}h ${m}m`
  }
  const toggleSelect = (id) => { setSelected(prev => { const next = new Set(prev); if (next.has(id)) next.delete(id); else next.add(id); return next }) }
  const selectAll = () => { if (selected.size === filtered.length) setSelected(new Set()); else setSelected(new Set(filtered.map(f => f.id))) }
  const doBatchDelete = async () => {
    if (selected.size === 0) return; const count = selected.size; setDeleting(true)
    await api.deleteFiles([...selected]); setFiles(prev => prev.filter(f => !selected.has(f.id)))
    notify.success(`${count} file(s) deleted`); setSelected(new Set()); setShowBatchDelete(false); setDeleting(false)
  }

  if (loading) return <div style={{ padding: 40 }} />

  return (
    <div style={{ padding: '32px 40px', maxWidth: 1000 }} className="fade-in">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 44, fontWeight: 900, letterSpacing: -0.5, marginBottom: 6, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 10 }}>
            <Fa icon={icons.files} style={{ fontSize: 32 }} /> files
          </h1>
          <p style={{ fontSize: 13, color: 'var(--sub-color)' }}>{committed.length} committed · {staged.length} staged</p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <button onClick={handleSyncDrive} disabled={syncing}
            title="pull every file from your drive root into reyna so recall can search them"
            style={{
              ...btnBase,
              opacity: syncing ? 0.65 : 1,
              cursor: syncing ? 'progress' : 'pointer',
            }}>
            <Fa icon={syncing ? icons.loading : 'fa-rotate'} spin={syncing} style={{ fontSize: 11 }} />
            {syncing ? 'syncing...' : 'sync from drive'}
          </button>
          {selected.size > 0 && (
            <button onClick={() => setShowBatchDelete(true)} style={btnDanger}>
              <Fa icon={icons.delete} style={{ fontSize: 10 }} /> delete {selected.size}
            </button>
          )}
          <div style={{ display: 'flex', gap: 3, background: 'var(--card-bg)', borderRadius: 'var(--roundness)', padding: 3 }}>
            {['list', 'grid'].map(m => (
              <button key={m} onClick={() => setViewMode(m)} style={{
                padding: '5px 10px', border: 'none', borderRadius: 'var(--roundness)', fontSize: 12, cursor: 'pointer',
                background: viewMode === m ? 'var(--bg-color)' : 'transparent', color: viewMode === m ? 'var(--text-color)' : 'var(--sub-color)',
              }}>
                <Fa icon={m === 'list' ? 'fa-list' : 'fa-th'} style={{ fontSize: 11 }} />
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* staging area */}
      {staged.length > 0 && (
        <div style={{ ...cardStyle, marginBottom: 16, borderTop: '2px solid var(--main-color)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
            <h2 style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.staging} style={{ fontSize: 13 }} /> staging area
            </h2>
            <div style={{ display: 'flex', gap: 6 }}>
              <button onClick={removeAllStaged} style={btnDanger}><Fa icon={icons.close} style={{ fontSize: 9 }} /> remove all</button>
              <button onClick={commitAllStaged} disabled={committing} style={{ ...btnPrimary, opacity: committing ? 0.6 : 1 }}>
                {committing ? <><Fa icon={icons.loading} spin style={{ fontSize: 9 }} /> pushing...</> : <>push all to drive <Fa icon={icons.arrowRight} style={{ fontSize: 9 }} /></>}
              </button>
            </div>
          </div>
          <div style={{ fontSize: 13, color: 'var(--sub-color)', marginBottom: 8 }}>auto-commit in 24 hours.</div>
          {staged.slice(0, 8).map((f, i) => (
            <div key={f.id} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '7px 0', borderBottom: i < Math.min(staged.length, 8) - 1 ? '1px solid var(--card-border)' : 'none' }}>
              <IconBox icon={icons.staging} color="var(--main-color)" bg="rgba(37,211,102,0.08)" size={30} iconSize={12} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{f.file_name}</div>
                <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>{f.subject || 'Unsorted'} · {formatBytes(f.file_size)}</div>
              </div>
              <div style={{ fontSize: 10, color: 'var(--main-color)', fontWeight: 500, flexShrink: 0 }}>{hoursUntilCommit(f.created_at)}</div>
              <button onClick={() => removeStaged(f.id)} style={{ ...btnBase, padding: '2px 6px' }}><Fa icon={icons.close} style={{ fontSize: 8 }} /></button>
            </div>
          ))}
        </div>
      )}

      {/* Sort controls */}
      <div style={{ display: 'flex', gap: 6, marginBottom: 10, alignItems: 'center' }}>
        <span style={{ fontSize: 13, color: 'var(--sub-color)', fontWeight: 400, marginRight: 4 }}>sort by:</span>
        {[{ key: 'date', label: 'date' }, { key: 'name', label: 'name' }, { key: 'size', label: 'size' }, { key: 'subject', label: 'subject' }].map(s => (
          <button key={s.key} onClick={() => {
            if (sortBy === s.key) setSortOrder(o => o === 'asc' ? 'desc' : 'asc')
            else { setSortBy(s.key); setSortOrder(s.key === 'name' ? 'asc' : 'desc') }
          }} style={{
            padding: '4px 10px', fontSize: 11, fontWeight: 400, borderRadius: 'var(--roundness)', cursor: 'pointer',
            border: sortBy === s.key ? '1px solid var(--reyna-accent)' : '1px solid var(--card-border)',
            background: sortBy === s.key ? 'var(--reyna-accent-dim)' : 'var(--card-bg)',
            color: sortBy === s.key ? 'var(--reyna-accent)' : 'var(--sub-color)',
          }}>
            {s.label} {sortBy === s.key ? (sortOrder === 'asc' ? <Fa icon="fa-arrow-up" style={{ fontSize: 8 }} /> : <Fa icon="fa-arrow-down" style={{ fontSize: 8 }} />) : ''}
          </button>
        ))}
      </div>

      {/* Subject filters */}
      <div style={{ display: 'flex', gap: 6, marginBottom: 20, flexWrap: 'wrap' }}>
        {subjects.map(s => (
          <button key={s} onClick={() => setFilter(s)} style={{
            padding: '5px 12px', border: '1px solid', borderRadius: 'var(--roundness)', fontSize: 11, cursor: 'pointer',
            fontWeight: 400, transition: 'all 0.15s',
            background: filter === s ? (subjectColors[s] || 'var(--sub-color)') : 'var(--card-bg)',
            color: filter === s ? '#111' : 'var(--sub-color)',
            borderColor: filter === s ? (subjectColors[s] || 'var(--sub-color)') : 'var(--card-border)',
          }}>
            {s === 'all' ? `all (${files.length})` : `${s.toLowerCase()} (${files.filter(f => (f.subject || 'General') === s).length})`}
          </button>
        ))}
      </div>

      {/* Batch select */}
      {viewMode === 'list' && filtered.length > 0 && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
          <input type="checkbox" checked={selected.size === filtered.length && filtered.length > 0} onChange={selectAll} style={{ cursor: 'pointer', accentColor: 'var(--main-color)' }} />
          <span style={{ fontSize: 13, color: 'var(--sub-color)' }}>select all ({filtered.length})</span>
        </div>
      )}

      {/* File List */}
      {filtered.length === 0 ? (
        <div style={{ textAlign: 'center', padding: 60, color: 'var(--sub-color)' }}>
          <Fa icon={icons.inbox} style={{ fontSize: 40, marginBottom: 12 }} />
          <p style={{ fontSize: 14 }}>no files here yet.</p>
          <p style={{ fontSize: 11, marginTop: 4 }}>use <code>/reyna add</code> in your WhatsApp group to store files.</p>
        </div>
      ) : viewMode === 'list' ? (
        <div style={{ ...cardStyle, padding: 0, overflow: 'hidden' }}>
          <div style={{ display: 'grid', gridTemplateColumns: '32px 1fr 90px 70px 140px 50px 80px', gap: 10, padding: '8px 16px', background: '#f3f0ea', borderBottom: '1px solid var(--card-border)', fontSize: 9, fontWeight: 400, color: 'var(--sub-color)', textTransform: 'lowercase', letterSpacing: 1.5 }}>
            <span></span><span>file</span><span>subject</span><span>size</span><span>shared by</span><span>ver</span><span>actions</span>
          </div>
          {filtered.map((f, i) => (
            <div key={i} style={{
              display: 'grid', gridTemplateColumns: '32px 1fr 90px 70px 140px 50px 80px', gap: 10,
              padding: '10px 16px', borderBottom: '1px solid var(--card-border)',
              transition: 'background 0.15s', alignItems: 'center',
              background: selected.has(f.id) ? 'rgba(37,211,102,0.06)' : 'var(--card-bg, #FAF8F4)',
            }}
              onMouseEnter={e => { if (!selected.has(f.id)) e.currentTarget.style.background = 'var(--card-hover, #f5f2ec)' }}
              onMouseLeave={e => { e.currentTarget.style.background = selected.has(f.id) ? 'rgba(37,211,102,0.06)' : 'var(--card-bg)' }}
            >
              <input type="checkbox" checked={selected.has(f.id)} onChange={() => toggleSelect(f.id)} style={{ cursor: 'pointer', accentColor: 'var(--main-color)' }} />
              <div onClick={() => showVersions(f)} style={{ cursor: 'pointer' }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 6 }}>
                  <Fa icon={fileIconClass(f.mime_type, f.file_name)} style={{ fontSize: 11, color: 'var(--main-color)' }} /> {f.file_name}
                </div>
                <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>{timeAgo(f.created_at)}</div>
              </div>
              <span style={{
                fontSize: 10, fontWeight: 400, padding: '2px 6px', borderRadius: 'var(--roundness)',
                background: (subjectColors[f.subject] || '#646669') + '20',
                color: subjectColors[f.subject] || '#646669',
                display: 'inline-block', width: 'fit-content',
              }}>{f.subject || 'General'}</span>
              <span style={{ fontSize: 13, color: 'var(--sub-color)' }}>{formatBytes(f.file_size)}</span>
              <span style={{ fontSize: 13, color: 'var(--sub-color)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={displayName(f)}>{displayName(f)}</span>
              <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                <span style={{ fontSize: 11, color: 'var(--reyna-accent)', fontWeight: 500 }}>v{f.version}</span>
                <Fa icon={f.status === 'committed' ? icons.check : icons.staging} style={{ fontSize: 8, color: f.status === 'committed' ? 'var(--reyna-accent)' : 'var(--main-color)' }} />
              </div>
              <div style={{ display: 'flex', gap: 3 }}>
                <button onClick={(e) => { e.stopPropagation(); openPreview(f) }} title="Preview" style={iconBtnStyle('rgba(37,211,102,0.1)', 'var(--reyna-accent)', 'rgba(37,211,102,0.2)')}>
                  <Fa icon={icons.preview} />
                </button>
                <button onClick={(e) => { e.stopPropagation(); downloadFile(f) }} title="Download" style={iconBtnStyle('rgba(14,165,233,0.1)', '#0ea5e9', 'rgba(14,165,233,0.2)')}>
                  <Fa icon={icons.download} />
                </button>
                <button onClick={(e) => { e.stopPropagation(); confirmDelete(f) }} title="Delete" style={iconBtnStyle('rgba(220,38,38,0.06)', 'var(--error-color)', 'rgba(220,38,38,0.12)')}>
                  <Fa icon={icons.delete} />
                </button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(190px, 1fr))', gap: 10 }}>
          {filtered.map((f, i) => (
            <div key={i} style={{
              ...cardStyle, cursor: 'pointer', transition: 'all 0.15s',
              borderTop: `2px solid ${subjectColors[f.subject] || 'var(--sub-color)'}`, position: 'relative',
            }}
              onMouseEnter={e => { e.currentTarget.style.transform = 'translateY(-2px)'; e.currentTarget.style.boxShadow = '0 4px 16px rgba(0,0,0,0.2)' }}
              onMouseLeave={e => { e.currentTarget.style.transform = ''; e.currentTarget.style.boxShadow = '' }}
            >
              <div onClick={() => showVersions(f)}>
                <Fa icon={fileIconClass(f.mime_type, f.file_name)} style={{ fontSize: 24, color: '#1a1a1a', marginBottom: 8 }} />
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)', marginBottom: 4, wordBreak: 'break-word' }}>{f.file_name}</div>
                <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>{f.subject || 'General'} · v{f.version}</div>
                <div style={{ fontSize: 13, color: 'var(--sub-color)', marginTop: 4 }}>{formatBytes(f.file_size)} · {displayName(f)}</div>
              </div>
              <div style={{ display: 'flex', gap: 3, marginTop: 10 }}>
                <button onClick={() => openPreview(f)} style={{ ...btnBase, flex: 1, justifyContent: 'center', padding: '4px 0' }}><Fa icon={icons.preview} style={{ fontSize: 10 }} /> view</button>
                <button onClick={() => downloadFile(f)} style={{ ...btnBase, flex: 1, justifyContent: 'center', padding: '4px 0' }}><Fa icon={icons.download} style={{ fontSize: 10 }} /> get</button>
                <button onClick={() => confirmDelete(f)} style={{ ...btnDanger, padding: '4px 6px' }}><Fa icon={icons.delete} style={{ fontSize: 10 }} /></button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Version History Modal */}
      {selectedFile && createPortal(
        <div onClick={() => setSelectedFile(null)} style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 99999 }}>
          <div onClick={e => e.stopPropagation()} style={{ background: '#fff', borderRadius: '12px', padding: 32, maxWidth: 500, width: '90vw', maxHeight: '80vh', overflow: 'auto', border: '1.5px solid #ccc', boxShadow: '0 25px 60px rgba(0,0,0,0.15), 0 8px 20px rgba(0,0,0,0.1)' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h3 style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 8 }}>
                <Fa icon={fileIconClass(selectedFile.mime_type, selectedFile.file_name)} style={{ fontSize: 14 }} /> {selectedFile.file_name}
              </h3>
              <button onClick={() => setSelectedFile(null)} style={{ ...btnBase, borderRadius: '50%', width: 28, height: 28, padding: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Fa icon={icons.close} style={{ fontSize: 10 }} /></button>
            </div>
            <div style={{ fontSize: 13, color: 'var(--sub-color)', marginBottom: 12 }}>
              subject: <strong style={{ color: 'var(--text-color)' }}>{selectedFile.subject || 'General'}</strong> · size: <strong style={{ color: 'var(--text-color)' }}>{formatBytes(selectedFile.file_size)}</strong> · current: <strong style={{ color: 'var(--text-color)' }}>v{selectedFile.version}</strong>
            </div>
            <div style={{ display: 'flex', gap: 6, marginBottom: 16 }}>
              <button onClick={() => { setSelectedFile(null); openPreview(selectedFile) }} style={btnBase} onMouseEnter={e=>{e.currentTarget.style.borderColor='#999';e.currentTarget.style.boxShadow='0 3px 10px rgba(0,0,0,0.1)'}} onMouseLeave={e=>{e.currentTarget.style.borderColor='';e.currentTarget.style.boxShadow=''}}><Fa icon={icons.preview} style={{ fontSize: 10 }} /> preview</button>
              <button onClick={() => downloadFile(selectedFile)} style={btnBase} onMouseEnter={e=>{e.currentTarget.style.borderColor='#999';e.currentTarget.style.boxShadow='0 3px 10px rgba(0,0,0,0.1)'}} onMouseLeave={e=>{e.currentTarget.style.borderColor='';e.currentTarget.style.boxShadow=''}}><Fa icon={icons.download} style={{ fontSize: 10 }} /> download</button>
              <button onClick={() => { setSelectedFile(null); confirmDelete(selectedFile) }} style={btnDanger}><Fa icon={icons.delete} style={{ fontSize: 10 }} /> delete</button>
            </div>
            <h4 style={{ fontSize: 12, fontWeight: 400, marginBottom: 10, color: 'var(--main-color)' }}>version history</h4>
            {versions.length === 0 ? <p style={{ fontSize: 13, color: 'var(--sub-color)' }}>single version</p> :
              versions.map((v, i) => (
                <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '8px 0', borderBottom: '1px solid var(--card-border)' }}>
                  <div style={{ width: 8, height: 8, borderRadius: '50%', background: i === 0 ? 'var(--reyna-accent)' : 'var(--sub-color)' }} />
                  <div style={{ flex: 1 }}>
                    <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)' }}>version {v.version}</div>
                    <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>{formatBytes(v.file_size)} · {timeAgo(v.created_at)}</div>
                  </div>
                  {i === 0 && <span style={{ fontSize: 10, fontWeight: 500, color: 'var(--reyna-accent)', background: 'var(--reyna-accent-dim)', padding: '2px 8px', borderRadius: 'var(--roundness)' }}>latest</span>}
                </div>
              ))}
          </div>
        </div>, document.body
      )}

      {/* File Preview */}
      {previewFile && createPortal(
        <div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.8)', display: 'flex', flexDirection: 'column', zIndex: 99999 }}>
          <div style={{ padding: '10px 20px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: '#fff', borderBottom: '1px solid #e5e0d8', flexShrink: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <Fa icon={fileIconClass(previewFile.mime_type, previewFile.file_name)} style={{ fontSize: 18, color: 'var(--main-color)' }} />
              <div>
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)' }}>{previewFile.file_name}</div>
                <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>{formatBytes(previewFile.file_size)} · {previewFile.subject || 'General'} · by {displayName(previewFile)}</div>
              </div>
            </div>
            <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
              {previewData?.driveUrl && <a href={previewData.driveUrl} target="_blank" rel="noreferrer" style={{ ...btnBase, textDecoration: 'none', color: 'var(--sub-color)' }}><Fa icon={icons.link} style={{ fontSize: 9 }} /> open in drive</a>}
              <button onClick={() => downloadFile(previewFile)} style={btnBase} onMouseEnter={e=>{e.currentTarget.style.borderColor='#999';e.currentTarget.style.boxShadow='0 3px 10px rgba(0,0,0,0.1)'}} onMouseLeave={e=>{e.currentTarget.style.borderColor='';e.currentTarget.style.boxShadow=''}}><Fa icon={icons.download} style={{ fontSize: 10 }} /> download</button>
              <button onClick={() => { setPreviewFile(null); setPreviewData(null) }} style={{ ...btnBase, borderRadius: '50%', width: 32, height: 32, padding: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Fa icon={icons.close} style={{ fontSize: 12 }} /></button>
            </div>
          </div>
          <div style={{ flex: 1, overflow: 'auto', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            {previewLoading ? <div style={{ color: 'var(--sub-color)', fontSize: 14 }}><Fa icon={icons.loading} spin /> loading preview...</div>
            : !previewData ? <div style={{ color: 'var(--sub-color)', fontSize: 14 }}>preview not available</div>
            : previewData.type === 'drive' ? <iframe src={previewData.url} style={{ width: '100%', height: '100%', border: 'none' }} title="Preview" allow="autoplay" />
            : previewData.type === 'image' ? <img src={previewData.url} alt={previewFile.file_name} style={{ maxWidth: '95%', maxHeight: '95%', borderRadius: 'var(--roundness)', objectFit: 'contain' }} />
            : previewData.type === 'pdf' ? <iframe src={previewData.url} style={{ width: '100%', height: '100%', border: 'none' }} title="PDF" />
            : previewData.type === 'text' ? (
              <div style={{ width: '100%', height: '100%', overflow: 'auto', padding: '20px 28px', background: 'var(--card-bg)' }}>
                <pre style={{ fontFamily: 'var(--font-mono)', fontSize: 12, lineHeight: 1.7, whiteSpace: 'pre-wrap', wordBreak: 'break-word', color: 'var(--text-color)', margin: 0 }}>{previewData.content}</pre>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 16, color: 'var(--sub-color)' }}>
                <Fa icon={fileIconClass(previewFile.mime_type, previewFile.file_name)} style={{ fontSize: 48, color: 'var(--main-color)' }} />
                <p style={{ fontSize: 14, fontWeight: 500 }}>no local preview available</p>
                <button onClick={() => downloadFile(previewFile)} style={btnPrimary}><Fa icon={icons.download} style={{ fontSize: 10 }} /> download file</button>
              </div>
            )}
          </div>
        </div>, document.body
      )}

      {/* Delete Confirmation */}
      {deleteTarget && createPortal(
        <div onClick={() => setDeleteTarget(null)} style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 99999 }}>
          <div onClick={e => e.stopPropagation()} style={{ background: '#fff', borderRadius: '12px', padding: 32, maxWidth: 420, width: '90vw', border: '1.5px solid #ccc', boxShadow: '0 25px 60px rgba(0,0,0,0.15), 0 8px 20px rgba(0,0,0,0.1)' }}>
            <h3 style={{ fontSize: 20, fontWeight: 700, letterSpacing: -0.3, color: 'var(--error-color)', marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.warning} style={{ fontSize: 14 }} /> Delete File?
            </h3>
            <p style={{ fontSize: 14, color: 'var(--text-secondary, #555)', lineHeight: 1.6, marginBottom: 8 }}>delete <strong style={{ color: 'var(--text-color)' }}>{deleteTarget.file_name}</strong>?</p>
            <p style={{ fontSize: 14, color: 'var(--text-secondary, #555)', lineHeight: 1.6, marginBottom: 24 }}>Moves to Drive trash (30 day recovery).</p>
            <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
              <button onClick={() => setDeleteTarget(null)} style={btnBase} onMouseEnter={e=>{e.currentTarget.style.borderColor='#999';e.currentTarget.style.boxShadow='0 3px 10px rgba(0,0,0,0.1)'}} onMouseLeave={e=>{e.currentTarget.style.borderColor='';e.currentTarget.style.boxShadow=''}}>cancel</button>
              <button onClick={doDelete} disabled={deleting} style={{ ...btnDanger, opacity: deleting ? 0.6 : 1 }}>{deleting ? 'deleting...' : 'delete'}</button>
            </div>
          </div>
        </div>, document.body
      )}

      {/* Batch Delete */}
      {showBatchDelete && createPortal(
        <div onClick={() => setShowBatchDelete(false)} style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 99999 }}>
          <div onClick={e => e.stopPropagation()} style={{ background: '#fff', borderRadius: '12px', padding: 32, maxWidth: 420, width: '90vw', border: '1.5px solid #ccc', boxShadow: '0 25px 60px rgba(0,0,0,0.15), 0 8px 20px rgba(0,0,0,0.1)' }}>
            <h3 style={{ fontSize: 20, fontWeight: 700, letterSpacing: -0.3, color: 'var(--error-color)', marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.warning} style={{ fontSize: 14 }} /> Delete {selected.size} file(s)?
            </h3>
            <p style={{ fontSize: 15, color: 'var(--text-secondary, #555)', lineHeight: 1.6, marginBottom: 24 }}>this will remove {selected.size} file(s) from reyna and move to drive trash.</p>
            <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
              <button onClick={() => setShowBatchDelete(false)} style={btnBase} onMouseEnter={e=>{e.currentTarget.style.borderColor='#999';e.currentTarget.style.boxShadow='0 3px 10px rgba(0,0,0,0.1)'}} onMouseLeave={e=>{e.currentTarget.style.borderColor='';e.currentTarget.style.boxShadow=''}}>cancel</button>
              <button onClick={doBatchDelete} disabled={deleting} style={{ ...btnDanger, opacity: deleting ? 0.6 : 1 }}>{deleting ? 'deleting...' : `delete ${selected.size} files`}</button>
            </div>
          </div>
        </div>, document.body
      )}
    </div>
  )
}
