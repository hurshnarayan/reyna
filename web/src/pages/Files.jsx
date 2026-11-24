import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { createPortal } from 'react-dom'
import { api, getToken } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa, icons, fileIconClass } from '../components/icons'

function formatBytes(b) {
  if (b < 1024) return b + ' b'
  if (b < 1048576) return (b / 1024).toFixed(1) + ' kb'
  return (b / 1048576).toFixed(1) + ' mb'
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

function canPreview(mimeType, fileName) {
  const m = (mimeType || '').toLowerCase()
  const ext = (fileName || '').split('.').pop()?.toLowerCase()
  if (m.includes('pdf') || ext === 'pdf') return 'pdf'
  if (m.includes('image') || ['png','jpg','jpeg','gif','webp','svg'].includes(ext)) return 'image'
  if (m.includes('text') || ['txt','md','csv','json','js','py','go','html','css'].includes(ext)) return 'text'
  return null
}

export default function Files() {
  const navigate = useNavigate()
  const [files, setFiles] = useState([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('all')
  const [searchQuery, setSearchQuery] = useState('')
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

  const fetchFiles = () => {
    return api.files('', 200, sortBy, sortOrder).then(f => {
      setFiles(f || [])
      setLoading(false)
      notify.hideLoader()
    })
  }

  useEffect(() => {
    notify.showLoader()
    fetchFiles()
    const interval = setInterval(fetchFiles, 5000)
    return () => clearInterval(interval)
  }, [sortBy, sortOrder])

  const subjects = ['all', ...Array.from(new Set(files.map(f => f.subject || 'General').filter(Boolean)))]

  const filtered = files.filter(f => {
    const matchesSubject = filter === 'all' || (f.subject || 'General').toLowerCase() === filter.toLowerCase()
    const matchesSearch = !searchQuery.trim() ||
      f.file_name?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (f.subject || 'General').toLowerCase().includes(searchQuery.toLowerCase()) ||
      displayName(f).toLowerCase().includes(searchQuery.toLowerCase())
    return matchesSubject && matchesSearch
  })

  const toggleSort = (col) => {
    if (sortBy === col) {
      setSortOrder(o => o === 'asc' ? 'desc' : 'asc')
    } else {
      setSortBy(col)
      setSortOrder(col === 'name' ? 'asc' : 'desc')
    }
  }

  const showVersions = async (f) => {
    setSelectedFile(f)
    const v = await api.versions(f.id)
    setVersions(v || [])
  }

  const openPreview = async (f) => {
    setPreviewFile(f)
    setPreviewLoading(true)
    setPreviewData(null)
    const type = canPreview(f.mime_type, f.file_name)
    const hasDriveId = f.drive_file_id && !f.drive_file_id.startsWith('local_') && !f.drive_file_id.startsWith('meta_')
    if (hasDriveId) {
      setPreviewData({
        type: 'drive',
        url: `https://drive.google.com/file/d/${f.drive_file_id}/preview`,
        driveUrl: `https://drive.google.com/file/d/${f.drive_file_id}/view`,
      })
      setPreviewLoading(false)
      return
    }
    try {
      const resp = await fetch(api.downloadUrl(f.id), { headers: { 'Authorization': `Bearer ${getToken()}` } })
      const contentType = resp.headers.get('content-type') || ''
      if (contentType.includes('json')) {
        const info = await resp.json()
        if (info.preview_url || info.drive_url) {
          setPreviewData({ type: 'drive', url: info.preview_url || info.drive_url, driveUrl: info.drive_url })
        } else {
          setPreviewData({ type: 'not_available' })
        }
      } else if (type === 'image') {
        const blob = await resp.blob()
        setPreviewData({ type: 'image', url: URL.createObjectURL(blob) })
      } else if (type === 'text') {
        const text = await resp.text()
        setPreviewData({ type: 'text', content: text })
      } else if (type === 'pdf') {
        const blob = await resp.blob()
        setPreviewData({ type: 'pdf', url: URL.createObjectURL(blob) })
      } else {
        setPreviewData({ type: 'not_available' })
      }
    } catch {
      setPreviewData({ type: 'not_available' })
    }
    setPreviewLoading(false)
  }

  const downloadFile = (f) => {
    if (f.drive_file_id && !f.drive_file_id.startsWith('local_') && !f.drive_file_id.startsWith('meta_')) {
      window.open(`https://drive.google.com/file/d/${f.drive_file_id}/view`, '_blank')
      return
    }
    fetch(api.downloadUrl(f.id), { headers: { 'Authorization': `Bearer ${getToken()}` } })
      .then(r => r.blob())
      .then(blob => {
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = f.file_name
        a.click()
        URL.revokeObjectURL(url)
      })
  }

  const confirmDelete = (f) => setDeleteTarget(f)
  const doDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    await api.deleteFile(deleteTarget.id)
    setFiles(prev => prev.filter(f => f.id !== deleteTarget.id))
    notify.success(`deleted ${deleteTarget.file_name.toLowerCase()}`)
    setDeleteTarget(null)
    setDeleting(false)
  }

  const toggleSelect = (id) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const selectAll = () => {
    if (selected.size === filtered.length && filtered.length > 0) {
      setSelected(new Set())
    } else {
      setSelected(new Set(filtered.map(f => f.id)))
    }
  }

  const doBatchDelete = async () => {
    if (selected.size === 0) return
    const count = selected.size
    setDeleting(true)
    await api.deleteFiles([...selected])
    setFiles(prev => prev.filter(f => !selected.has(f.id)))
    notify.success(`${count} file(s) deleted`)
    setSelected(new Set())
    setShowBatchDelete(false)
    setDeleting(false)
  }

  return (
    <div style={{ padding: '40px 48px 60px', maxWidth: 1100 }} className="fade-in">
      {/* ── Breadcrumb ── */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#6b7280', marginBottom: 20 }}>
        <span
          style={{ cursor: 'pointer', transition: 'color 0.15s' }}
          onClick={() => navigate('/dashboard')}
          onMouseEnter={e => e.currentTarget.style.color = '#111827'}
          onMouseLeave={e => e.currentTarget.style.color = '#6b7280'}
        >
          reyna
        </span>
        <span style={{ color: '#d1d5db' }}>/</span>
        <span style={{ color: '#111827', fontWeight: 600 }}>all files</span>
      </div>

      {/* ── Main All Files Section ── */}
      <div style={{
        background: '#ffffff',
        border: '1px solid #e5e7eb',
        borderRadius: 18,
        padding: '22px 26px',
        boxShadow: '0 4px 20px -2px rgba(0, 0, 0, 0.04), 0 1px 3px rgba(0, 0, 0, 0.02)',
      }}>
        {/* Controls Bar */}
        <div style={{
          display: 'flex',
          flexWrap: 'wrap',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 14,
          marginBottom: 18,
          paddingBottom: 16,
          borderBottom: '1px solid #f3f4f6',
        }}>
          {/* Left Title & Segmented Subject Filters */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <h2 style={{ fontSize: 18, fontWeight: 700, color: '#111827', margin: 0, textTransform: 'lowercase', letterSpacing: '-0.01em' }}>
                all files
              </h2>
              <span style={{
                fontSize: 11,
                fontWeight: 600,
                color: '#15803d',
                background: '#dcfce7',
                padding: '2px 8px',
                borderRadius: 9999,
              }}>
                {filtered.length}
              </span>
            </div>

            {/* Segmented Subject Filter Track */}
            {subjects.length > 1 && (
              <div style={{
                display: 'flex',
                gap: 4,
                background: '#f9fafb',
                padding: 3,
                borderRadius: 10,
                border: '1px solid #f3f4f6',
                flexWrap: 'wrap',
              }}>
                {subjects.map(s => {
                  const isSelected = filter === s
                  return (
                    <button
                      key={s}
                      onClick={() => setFilter(s)}
                      style={{
                        background: isSelected ? '#ffffff' : 'transparent',
                        color: isSelected ? '#111827' : '#6b7280',
                        border: isSelected ? '1px solid rgba(0,0,0,0.06)' : '1px solid transparent',
                        borderRadius: 7,
                        padding: '5px 12px',
                        fontSize: 12,
                        fontWeight: isSelected ? 600 : 500,
                        cursor: 'pointer',
                        fontFamily: 'inherit',
                        textTransform: 'lowercase',
                        boxShadow: isSelected ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
                        transition: 'all 0.15s ease',
                      }}
                    >
                      {s === 'all' ? 'view all' : s.toLowerCase()}
                    </button>
                  )
                })}
              </div>
            )}
          </div>

          {/* Right Search Input, Refresh, View Toggle & Batch Delete */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            {selected.size > 0 && (
              <button
                onClick={() => setShowBatchDelete(true)}
                style={{
                  background: '#fef2f2',
                  border: '1px solid #fecaca',
                  borderRadius: 9999,
                  padding: '7px 14px',
                  fontSize: 12,
                  fontWeight: 600,
                  color: '#dc2626',
                  cursor: 'pointer',
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 6,
                  textTransform: 'lowercase',
                  transition: 'all 0.15s',
                }}
              >
                <Fa icon={icons.delete} style={{ fontSize: 10 }} />
                delete {selected.size}
              </button>
            )}

            <div style={{
              position: 'relative',
              display: 'flex',
              alignItems: 'center',
            }}>
              <Fa icon="fa-search" style={{ position: 'absolute', left: 12, fontSize: 11, color: '#9ca3af' }} />
              <input
                type="text"
                placeholder="search files..."
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                style={{
                  padding: '7px 12px 7px 32px',
                  fontSize: 12,
                  borderRadius: 9999,
                  border: '1px solid #e5e7eb',
                  background: '#f9fafb',
                  color: '#111827',
                  outline: 'none',
                  width: 190,
                  fontFamily: 'inherit',
                  textTransform: 'lowercase',
                  transition: 'border-color 0.15s, box-shadow 0.15s',
                }}
                onFocus={e => {
                  e.currentTarget.style.borderColor = '#111827'
                  e.currentTarget.style.background = '#ffffff'
                }}
                onBlur={e => {
                  e.currentTarget.style.borderColor = '#e5e7eb'
                  e.currentTarget.style.background = '#f9fafb'
                }}
              />
            </div>

            <button
              onClick={fetchFiles}
              style={{
                background: '#ffffff',
                border: '1px solid #e5e7eb',
                borderRadius: 9999,
                padding: '7px 14px',
                fontSize: 12,
                fontWeight: 500,
                color: '#374151',
                cursor: 'pointer',
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                textTransform: 'lowercase',
                transition: 'all 0.15s',
              }}
              onMouseEnter={e => {
                e.currentTarget.style.borderColor = '#9ca3af'
                e.currentTarget.style.background = '#f9fafb'
              }}
              onMouseLeave={e => {
                e.currentTarget.style.borderColor = '#e5e7eb'
                e.currentTarget.style.background = '#ffffff'
              }}
            >
              <Fa icon={icons.refresh} style={{ fontSize: 10 }} />
              refresh
            </button>

            {/* List / Grid Toggle */}
            <div style={{
              display: 'flex',
              background: '#f9fafb',
              border: '1px solid #e5e7eb',
              borderRadius: 9999,
              padding: 2,
            }}>
              <button
                onClick={() => setViewMode('list')}
                title="list view"
                style={{
                  padding: '5px 10px',
                  border: 'none',
                  borderRadius: 9999,
                  fontSize: 11,
                  cursor: 'pointer',
                  background: viewMode === 'list' ? '#ffffff' : 'transparent',
                  color: viewMode === 'list' ? '#111827' : '#9ca3af',
                  boxShadow: viewMode === 'list' ? '0 1px 2px rgba(0,0,0,0.05)' : 'none',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  transition: 'all 0.15s',
                }}
              >
                <Fa icon="fa-list" />
              </button>
              <button
                onClick={() => setViewMode('grid')}
                title="grid view"
                style={{
                  padding: '5px 10px',
                  border: 'none',
                  borderRadius: 9999,
                  fontSize: 11,
                  cursor: 'pointer',
                  background: viewMode === 'grid' ? '#ffffff' : 'transparent',
                  color: viewMode === 'grid' ? '#111827' : '#9ca3af',
                  boxShadow: viewMode === 'grid' ? '0 1px 2px rgba(0,0,0,0.05)' : 'none',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  transition: 'all 0.15s',
                }}
              >
                <Fa icon="fa-th" />
              </button>
            </div>
          </div>
        </div>

        {/* ── Files Table or Grid ── */}
        {loading ? (
          <div style={{ padding: '48px 0', textAlign: 'center', color: '#9ca3af', fontSize: 14 }}>
            <Fa icon={icons.loading} spin style={{ fontSize: 16, marginRight: 8 }} />
            loading files...
          </div>
        ) : filtered.length === 0 ? (
          <div style={{ padding: '56px 16px', textAlign: 'center' }}>
            <div style={{
              width: 44,
              height: 44,
              borderRadius: '50%',
              background: '#f3f4f6',
              color: '#9ca3af',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 18,
              marginBottom: 12,
            }}>
              <Fa icon={icons.files} />
            </div>
            <div style={{ fontSize: 15, fontWeight: 600, color: '#111827', marginBottom: 4, textTransform: 'lowercase' }}>
              {searchQuery || filter !== 'all' ? 'no files match your filter' : 'no files found'}
            </div>
            <p style={{ fontSize: 13, color: '#6b7280', margin: 0, textTransform: 'lowercase' }}>
              {searchQuery || filter !== 'all' ? 'try adjusting your search or subject filters' : 'files shared in whatsapp groups where reyna is added will appear here'}
            </p>
          </div>
        ) : viewMode === 'list' ? (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #f3f4f6' }}>
                  <th style={{ width: 40, padding: '10px 12px' }}>
                    <input
                      type="checkbox"
                      checked={selected.size > 0 && selected.size === filtered.length}
                      onChange={selectAll}
                      style={{ cursor: 'pointer' }}
                    />
                  </th>
                  <th
                    onClick={() => toggleSort('name')}
                    style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase', cursor: 'pointer', userSelect: 'none' }}
                  >
                    file name {sortBy === 'name' && <Fa icon={sortOrder === 'asc' ? 'fa-arrow-up' : 'fa-arrow-down'} style={{ fontSize: 9, marginLeft: 3 }} />}
                  </th>
                  <th
                    onClick={() => toggleSort('subject')}
                    style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase', cursor: 'pointer', userSelect: 'none' }}
                  >
                    subject {sortBy === 'subject' && <Fa icon={sortOrder === 'asc' ? 'fa-arrow-up' : 'fa-arrow-down'} style={{ fontSize: 9, marginLeft: 3 }} />}
                  </th>
                  <th
                    onClick={() => toggleSort('size')}
                    style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase', cursor: 'pointer', userSelect: 'none' }}
                  >
                    size {sortBy === 'size' && <Fa icon={sortOrder === 'asc' ? 'fa-arrow-up' : 'fa-arrow-down'} style={{ fontSize: 9, marginLeft: 3 }} />}
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    shared by
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    version
                  </th>
                  <th style={{ width: 100, padding: '10px 14px', textAlign: 'right', fontSize: 12, fontWeight: 600, color: '#6b7280' }}>
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((f) => {
                  const isChecked = selected.has(f.id)

                  return (
                    <tr
                      key={f.id}
                      style={{
                        borderBottom: '1px solid #f9fafb',
                        background: isChecked ? '#f9fafb' : '#ffffff',
                        transition: 'background 0.1s',
                      }}
                      onMouseEnter={e => e.currentTarget.style.background = '#f9fafb'}
                      onMouseLeave={e => e.currentTarget.style.background = isChecked ? '#f9fafb' : '#ffffff'}
                    >
                      {/* Checkbox */}
                      <td style={{ padding: '14px 12px' }}>
                        <input
                          type="checkbox"
                          checked={isChecked}
                          onChange={() => toggleSelect(f.id)}
                          style={{ cursor: 'pointer' }}
                        />
                      </td>

                      {/* File Name & Time */}
                      <td style={{ padding: '14px 14px' }}>
                        <div
                          onClick={() => showVersions(f)}
                          style={{ display: 'flex', alignItems: 'center', gap: 12, cursor: 'pointer' }}
                        >
                          <div style={{
                            width: 36,
                            height: 36,
                            borderRadius: 10,
                            background: '#f9fafb',
                            border: '1px solid #e5e7eb',
                            color: '#374151',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            fontSize: 14,
                            flexShrink: 0,
                          }}>
                            <Fa icon={fileIconClass(f.mime_type, f.file_name)} />
                          </div>
                          <div>
                            <div style={{
                              fontSize: 14,
                              fontWeight: 600,
                              color: '#111827',
                              textTransform: 'lowercase',
                              letterSpacing: '-0.01em',
                            }}>
                              {f.file_name.toLowerCase()}
                            </div>
                            <div style={{ fontSize: 12, color: '#9ca3af' }}>
                              {timeAgo(f.created_at)}
                            </div>
                          </div>
                        </div>
                      </td>

                      {/* Subject */}
                      <td style={{ padding: '14px 14px' }}>
                        <span style={{
                          fontSize: 11,
                          fontWeight: 500,
                          padding: '3px 10px',
                          borderRadius: 9999,
                          background: '#f3f4f6',
                          color: '#374151',
                          border: '1px solid #e5e7eb',
                          textTransform: 'lowercase',
                          display: 'inline-block',
                        }}>
                          {(f.subject || 'general').toLowerCase()}
                        </span>
                      </td>

                      {/* Size */}
                      <td style={{ padding: '14px 14px', fontSize: 13, color: '#6b7280', textTransform: 'lowercase' }}>
                        {formatBytes(f.file_size)}
                      </td>

                      {/* Shared By */}
                      <td style={{ padding: '14px 14px', fontSize: 13, color: '#374151', textTransform: 'lowercase' }}>
                        {displayName(f).toLowerCase()}
                      </td>

                      {/* Version */}
                      <td style={{ padding: '14px 14px' }}>
                        <span style={{
                          fontSize: 11,
                          fontWeight: 600,
                          color: '#15803d',
                          background: '#dcfce7',
                          padding: '2px 8px',
                          borderRadius: 9999,
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: 4,
                          lineHeight: 1.2,
                        }}>
                          v{f.version} <Fa icon={icons.check} style={{ fontSize: 8 }} />
                        </span>
                      </td>

                      {/* Actions */}
                      <td style={{ padding: '14px 14px', textAlign: 'right' }}>
                        <div style={{ display: 'inline-flex', alignItems: 'center', gap: 6, justifyContent: 'flex-end' }}>
                          <button
                            onClick={(e) => { e.stopPropagation(); openPreview(f) }}
                            title="preview"
                            style={{
                              background: '#ffffff',
                              border: '1px solid #e5e7eb',
                              borderRadius: 9999,
                              padding: '5px 9px',
                              fontSize: 11,
                              color: '#6b7280',
                              cursor: 'pointer',
                              display: 'inline-flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              transition: 'all 0.15s',
                            }}
                            onMouseEnter={e => {
                              e.currentTarget.style.borderColor = '#111827'
                              e.currentTarget.style.color = '#111827'
                            }}
                            onMouseLeave={e => {
                              e.currentTarget.style.borderColor = '#e5e7eb'
                              e.currentTarget.style.color = '#6b7280'
                            }}
                          >
                            <Fa icon={icons.preview} style={{ fontSize: 10 }} />
                          </button>
                          <button
                            onClick={(e) => { e.stopPropagation(); downloadFile(f) }}
                            title="download"
                            style={{
                              background: '#ffffff',
                              border: '1px solid #e5e7eb',
                              borderRadius: 9999,
                              padding: '5px 9px',
                              fontSize: 11,
                              color: '#6b7280',
                              cursor: 'pointer',
                              display: 'inline-flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              transition: 'all 0.15s',
                            }}
                            onMouseEnter={e => {
                              e.currentTarget.style.borderColor = '#2563eb'
                              e.currentTarget.style.color = '#2563eb'
                            }}
                            onMouseLeave={e => {
                              e.currentTarget.style.borderColor = '#e5e7eb'
                              e.currentTarget.style.color = '#6b7280'
                            }}
                          >
                            <Fa icon={icons.download} style={{ fontSize: 10 }} />
                          </button>
                          <button
                            onClick={(e) => { e.stopPropagation(); confirmDelete(f) }}
                            title="delete"
                            style={{
                              background: '#ffffff',
                              border: '1px solid #e5e7eb',
                              borderRadius: 9999,
                              padding: '5px 9px',
                              fontSize: 11,
                              color: '#6b7280',
                              cursor: 'pointer',
                              display: 'inline-flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              transition: 'all 0.15s',
                            }}
                            onMouseEnter={e => {
                              e.currentTarget.style.borderColor = '#dc2626'
                              e.currentTarget.style.color = '#dc2626'
                            }}
                            onMouseLeave={e => {
                              e.currentTarget.style.borderColor = '#e5e7eb'
                              e.currentTarget.style.color = '#6b7280'
                            }}
                          >
                            <Fa icon={icons.delete} style={{ fontSize: 10 }} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : (
          /* Grid View */
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: 14 }}>
            {filtered.map((f) => {
              const isChecked = selected.has(f.id)
              return (
                <div
                  key={f.id}
                  style={{
                    background: isChecked ? '#f9fafb' : '#ffffff',
                    border: `1px solid ${isChecked ? '#111827' : '#e5e7eb'}`,
                    borderRadius: 14,
                    padding: '16px',
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'space-between',
                    transition: 'all 0.15s',
                    position: 'relative',
                  }}
                  onMouseEnter={e => {
                    e.currentTarget.style.borderColor = '#9ca3af'
                    e.currentTarget.style.transform = 'translateY(-1px)'
                    e.currentTarget.style.boxShadow = '0 4px 12px rgba(0,0,0,0.04)'
                  }}
                  onMouseLeave={e => {
                    e.currentTarget.style.borderColor = isChecked ? '#111827' : '#e5e7eb'
                    e.currentTarget.style.transform = 'none'
                    e.currentTarget.style.boxShadow = 'none'
                  }}
                >
                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                      <div style={{
                        width: 36,
                        height: 36,
                        borderRadius: 10,
                        background: '#f9fafb',
                        border: '1px solid #e5e7eb',
                        color: '#374151',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        fontSize: 15,
                      }}>
                        <Fa icon={fileIconClass(f.mime_type, f.file_name)} />
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                        <span style={{
                          fontSize: 10,
                          fontWeight: 600,
                          color: '#15803d',
                          background: '#dcfce7',
                          padding: '2px 6px',
                          borderRadius: 9999,
                        }}>
                          v{f.version}
                        </span>
                        <input
                          type="checkbox"
                          checked={isChecked}
                          onChange={() => toggleSelect(f.id)}
                          style={{ cursor: 'pointer' }}
                        />
                      </div>
                    </div>

                    <div
                      onClick={() => showVersions(f)}
                      style={{ cursor: 'pointer', marginBottom: 8 }}
                    >
                      <div style={{
                        fontSize: 14,
                        fontWeight: 600,
                        color: '#111827',
                        textTransform: 'lowercase',
                        lineHeight: 1.3,
                        wordBreak: 'break-word',
                      }}>
                        {f.file_name.toLowerCase()}
                      </div>
                      <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>
                        {timeAgo(f.created_at)} · {formatBytes(f.file_size)}
                      </div>
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14, flexWrap: 'wrap' }}>
                      <span style={{
                        fontSize: 11,
                        fontWeight: 500,
                        padding: '2px 8px',
                        borderRadius: 9999,
                        background: '#f3f4f6',
                        color: '#374151',
                        border: '1px solid #e5e7eb',
                        textTransform: 'lowercase',
                      }}>
                        {(f.subject || 'general').toLowerCase()}
                      </span>
                    </div>
                  </div>

                  {/* Grid Card Actions */}
                  <div style={{ display: 'flex', gap: 6, paddingTop: 12, borderTop: '1px solid #f3f4f6' }}>
                    <button
                      onClick={() => openPreview(f)}
                      style={{
                        flex: 1,
                        background: '#ffffff',
                        border: '1px solid #e5e7eb',
                        borderRadius: 9999,
                        padding: '5px 0',
                        fontSize: 11,
                        fontWeight: 500,
                        color: '#374151',
                        cursor: 'pointer',
                        display: 'inline-flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        gap: 4,
                        textTransform: 'lowercase',
                      }}
                      onMouseEnter={e => e.currentTarget.style.borderColor = '#111827'}
                      onMouseLeave={e => e.currentTarget.style.borderColor = '#e5e7eb'}
                    >
                      <Fa icon={icons.preview} style={{ fontSize: 9 }} /> view
                    </button>
                    <button
                      onClick={() => downloadFile(f)}
                      style={{
                        flex: 1,
                        background: '#ffffff',
                        border: '1px solid #e5e7eb',
                        borderRadius: 9999,
                        padding: '5px 0',
                        fontSize: 11,
                        fontWeight: 500,
                        color: '#374151',
                        cursor: 'pointer',
                        display: 'inline-flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        gap: 4,
                        textTransform: 'lowercase',
                      }}
                      onMouseEnter={e => e.currentTarget.style.borderColor = '#2563eb'}
                      onMouseLeave={e => e.currentTarget.style.borderColor = '#e5e7eb'}
                    >
                      <Fa icon={icons.download} style={{ fontSize: 9 }} /> get
                    </button>
                    <button
                      onClick={() => confirmDelete(f)}
                      style={{
                        background: '#ffffff',
                        border: '1px solid #e5e7eb',
                        borderRadius: 9999,
                        padding: '5px 10px',
                        fontSize: 11,
                        color: '#6b7280',
                        cursor: 'pointer',
                        display: 'inline-flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                      }}
                      onMouseEnter={e => {
                        e.currentTarget.style.borderColor = '#dc2626'
                        e.currentTarget.style.color = '#dc2626'
                      }}
                      onMouseLeave={e => {
                        e.currentTarget.style.borderColor = '#e5e7eb'
                        e.currentTarget.style.color = '#6b7280'
                      }}
                    >
                      <Fa icon={icons.delete} style={{ fontSize: 9 }} />
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* ── Version History Modal ── */}
      {selectedFile && createPortal(
        <div
          onClick={() => setSelectedFile(null)}
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            width: '100vw',
            height: '100vh',
            background: 'rgba(0,0,0,0.4)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 99999,
            backdropFilter: 'blur(3px)',
          }}
        >
          <div
            onClick={e => e.stopPropagation()}
            style={{
              background: '#ffffff',
              borderRadius: 16,
              padding: 28,
              maxWidth: 500,
              width: '90vw',
              maxHeight: '80vh',
              overflow: 'auto',
              border: '1px solid #e5e7eb',
              boxShadow: '0 20px 40px -4px rgba(0, 0, 0, 0.12), 0 1px 3px rgba(0, 0, 0, 0.05)',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <div style={{
                  width: 32,
                  height: 32,
                  borderRadius: 8,
                  background: '#f9fafb',
                  border: '1px solid #e5e7eb',
                  color: '#374151',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 14,
                }}>
                  <Fa icon={fileIconClass(selectedFile.mime_type, selectedFile.file_name)} />
                </div>
                <h3 style={{ fontSize: 16, fontWeight: 700, color: '#111827', margin: 0, textTransform: 'lowercase', letterSpacing: '-0.01em' }}>
                  {selectedFile.file_name.toLowerCase()}
                </h3>
              </div>
              <button
                onClick={() => setSelectedFile(null)}
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 9999,
                  width: 28,
                  height: 28,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                  color: '#6b7280',
                }}
              >
                <Fa icon={icons.close} style={{ fontSize: 10 }} />
              </button>
            </div>

            <div style={{ fontSize: 13, color: '#6b7280', marginBottom: 16, textTransform: 'lowercase' }}>
              subject: <strong style={{ color: '#111827' }}>{(selectedFile.subject || 'general').toLowerCase()}</strong> · size: <strong style={{ color: '#111827' }}>{formatBytes(selectedFile.file_size)}</strong> · current: <strong style={{ color: '#111827' }}>v{selectedFile.version}</strong>
            </div>

            <div style={{ display: 'flex', gap: 8, marginBottom: 20 }}>
              <button
                onClick={() => { setSelectedFile(null); openPreview(selectedFile) }}
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 9999,
                  padding: '6px 14px',
                  fontSize: 12,
                  fontWeight: 500,
                  color: '#374151',
                  cursor: 'pointer',
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 6,
                  textTransform: 'lowercase',
                }}
              >
                <Fa icon={icons.preview} style={{ fontSize: 10 }} /> preview
              </button>
              <button
                onClick={() => downloadFile(selectedFile)}
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 9999,
                  padding: '6px 14px',
                  fontSize: 12,
                  fontWeight: 500,
                  color: '#374151',
                  cursor: 'pointer',
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 6,
                  textTransform: 'lowercase',
                }}
              >
                <Fa icon={icons.download} style={{ fontSize: 10 }} /> download
              </button>
              <button
                onClick={() => { setSelectedFile(null); confirmDelete(selectedFile) }}
                style={{
                  background: '#fef2f2',
                  border: '1px solid #fecaca',
                  borderRadius: 9999,
                  padding: '6px 14px',
                  fontSize: 12,
                  fontWeight: 600,
                  color: '#dc2626',
                  cursor: 'pointer',
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 6,
                  textTransform: 'lowercase',
                }}
              >
                <Fa icon={icons.delete} style={{ fontSize: 10 }} /> delete
              </button>
            </div>

            <div style={{ fontSize: 12, fontWeight: 600, color: '#111827', marginBottom: 10, textTransform: 'lowercase' }}>
              version history
            </div>
            {versions.length === 0 ? (
              <p style={{ fontSize: 13, color: '#9ca3af', margin: 0, textTransform: 'lowercase' }}>single version</p>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                {versions.map((v, i) => (
                  <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '8px 0', borderBottom: '1px solid #f3f4f6' }}>
                    <div style={{ width: 8, height: 8, borderRadius: '50%', background: i === 0 ? '#15803d' : '#9ca3af' }} />
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: 13, fontWeight: 600, color: '#111827', textTransform: 'lowercase' }}>version {v.version}</div>
                      <div style={{ fontSize: 12, color: '#6b7280', textTransform: 'lowercase' }}>{formatBytes(v.file_size)} · {timeAgo(v.created_at)}</div>
                    </div>
                    {i === 0 && (
                      <span style={{ fontSize: 10, fontWeight: 600, color: '#15803d', background: '#dcfce7', padding: '2px 8px', borderRadius: 9999, textTransform: 'lowercase' }}>
                        latest
                      </span>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>,
        document.body
      )}

      {/* ── File Preview Modal ── */}
      {previewFile && createPortal(
        <div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.85)', display: 'flex', flexDirection: 'column', zIndex: 99999 }}>
          <div style={{ padding: '12px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: '#ffffff', borderBottom: '1px solid #e5e7eb', flexShrink: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <div style={{
                width: 32,
                height: 32,
                borderRadius: 8,
                background: '#f9fafb',
                border: '1px solid #e5e7eb',
                color: '#374151',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 14,
              }}>
                <Fa icon={fileIconClass(previewFile.mime_type, previewFile.file_name)} />
              </div>
              <div>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#111827', textTransform: 'lowercase' }}>{previewFile.file_name.toLowerCase()}</div>
                <div style={{ fontSize: 12, color: '#6b7280', textTransform: 'lowercase' }}>
                  {formatBytes(previewFile.file_size)} · {(previewFile.subject || 'general').toLowerCase()} · by {displayName(previewFile).toLowerCase()}
                </div>
              </div>
            </div>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              {previewData?.driveUrl && (
                <a
                  href={previewData.driveUrl}
                  target="_blank"
                  rel="noreferrer"
                  style={{
                    background: '#ffffff',
                    border: '1px solid #e5e7eb',
                    borderRadius: 9999,
                    padding: '6px 14px',
                    fontSize: 12,
                    fontWeight: 500,
                    color: '#374151',
                    textDecoration: 'none',
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    textTransform: 'lowercase',
                  }}
                >
                  <Fa icon={icons.link} style={{ fontSize: 9 }} /> open in drive
                </a>
              )}
              <button
                onClick={() => downloadFile(previewFile)}
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 9999,
                  padding: '6px 14px',
                  fontSize: 12,
                  fontWeight: 500,
                  color: '#374151',
                  cursor: 'pointer',
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 6,
                  textTransform: 'lowercase',
                }}
              >
                <Fa icon={icons.download} style={{ fontSize: 10 }} /> download
              </button>
              <button
                onClick={() => { setPreviewFile(null); setPreviewData(null) }}
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 9999,
                  width: 32,
                  height: 32,
                  padding: 0,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                  color: '#6b7280',
                }}
              >
                <Fa icon={icons.close} style={{ fontSize: 12 }} />
              </button>
            </div>
          </div>
          <div style={{ flex: 1, overflow: 'auto', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            {previewLoading ? (
              <div style={{ color: '#ffffff', fontSize: 14 }}>
                <Fa icon={icons.loading} spin style={{ marginRight: 8 }} /> loading preview...
              </div>
            ) : !previewData ? (
              <div style={{ color: '#ffffff', fontSize: 14 }}>preview not available</div>
            ) : previewData.type === 'drive' ? (
              <iframe src={previewData.url} style={{ width: '100%', height: '100%', border: 'none' }} title="Preview" allow="autoplay" />
            ) : previewData.type === 'image' ? (
              <img src={previewData.url} alt={previewFile.file_name} style={{ maxWidth: '95%', maxHeight: '95%', borderRadius: 8, objectFit: 'contain' }} />
            ) : previewData.type === 'pdf' ? (
              <iframe src={previewData.url} style={{ width: '100%', height: '100%', border: 'none' }} title="PDF" />
            ) : previewData.type === 'text' ? (
              <div style={{ width: '100%', height: '100%', overflow: 'auto', padding: '24px 32px', background: '#ffffff' }}>
                <pre style={{ fontFamily: 'monospace', fontSize: 12, lineHeight: 1.7, whiteSpace: 'pre-wrap', wordBreak: 'break-word', color: '#111827', margin: 0 }}>
                  {previewData.content}
                </pre>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 14, color: '#ffffff' }}>
                <Fa icon={fileIconClass(previewFile.mime_type, previewFile.file_name)} style={{ fontSize: 44, color: '#ffffff' }} />
                <p style={{ fontSize: 14, margin: 0, textTransform: 'lowercase' }}>no local preview available</p>
                <button
                  onClick={() => downloadFile(previewFile)}
                  style={{
                    background: '#ffffff',
                    border: '1px solid #ffffff',
                    borderRadius: 9999,
                    padding: '8px 18px',
                    fontSize: 12,
                    fontWeight: 600,
                    color: '#111827',
                    cursor: 'pointer',
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    textTransform: 'lowercase',
                  }}
                >
                  <Fa icon={icons.download} style={{ fontSize: 10 }} /> download file
                </button>
              </div>
            )}
          </div>
        </div>,
        document.body
      )}

      {/* ── Single Delete Confirmation Modal ── */}
      {deleteTarget && createPortal(
        <div
          onClick={() => setDeleteTarget(null)}
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            width: '100vw',
            height: '100vh',
            background: 'rgba(0,0,0,0.4)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 99999,
            backdropFilter: 'blur(3px)',
          }}
        >
          <div
            onClick={e => e.stopPropagation()}
            style={{
              background: '#ffffff',
              borderRadius: 16,
              padding: 28,
              maxWidth: 420,
              width: '90vw',
              border: '1px solid #e5e7eb',
              boxShadow: '0 20px 40px -4px rgba(0, 0, 0, 0.12), 0 1px 3px rgba(0, 0, 0, 0.05)',
            }}
          >
            <h3 style={{ fontSize: 17, fontWeight: 700, letterSpacing: -0.2, color: '#dc2626', margin: '0 0 10px 0', textTransform: 'lowercase', display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.warning} style={{ fontSize: 14 }} /> delete file?
            </h3>
            <p style={{ fontSize: 13, color: '#4b5563', lineHeight: 1.5, margin: '0 0 6px 0', textTransform: 'lowercase' }}>
              delete <strong style={{ color: '#111827' }}>{deleteTarget.file_name.toLowerCase()}</strong>?
            </p>
            <p style={{ fontSize: 12, color: '#9ca3af', lineHeight: 1.5, margin: '0 0 20px 0', textTransform: 'lowercase' }}>
              moves to drive trash (30 day recovery).
            </p>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button
                onClick={() => setDeleteTarget(null)}
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 9999,
                  padding: '7px 16px',
                  fontSize: 12,
                  fontWeight: 500,
                  color: '#374151',
                  cursor: 'pointer',
                  textTransform: 'lowercase',
                }}
              >
                cancel
              </button>
              <button
                onClick={doDelete}
                disabled={deleting}
                style={{
                  background: '#dc2626',
                  border: '1px solid #dc2626',
                  borderRadius: 9999,
                  padding: '7px 16px',
                  fontSize: 12,
                  fontWeight: 600,
                  color: '#ffffff',
                  cursor: 'pointer',
                  opacity: deleting ? 0.6 : 1,
                  textTransform: 'lowercase',
                }}
              >
                {deleting ? 'deleting...' : 'delete'}
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}

      {/* ── Batch Delete Modal ── */}
      {showBatchDelete && createPortal(
        <div
          onClick={() => setShowBatchDelete(false)}
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            width: '100vw',
            height: '100vh',
            background: 'rgba(0,0,0,0.4)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 99999,
            backdropFilter: 'blur(3px)',
          }}
        >
          <div
            onClick={e => e.stopPropagation()}
            style={{
              background: '#ffffff',
              borderRadius: 16,
              padding: 28,
              maxWidth: 420,
              width: '90vw',
              border: '1px solid #e5e7eb',
              boxShadow: '0 20px 40px -4px rgba(0, 0, 0, 0.12), 0 1px 3px rgba(0, 0, 0, 0.05)',
            }}
          >
            <h3 style={{ fontSize: 17, fontWeight: 700, letterSpacing: -0.2, color: '#dc2626', margin: '0 0 10px 0', textTransform: 'lowercase', display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon={icons.warning} style={{ fontSize: 14 }} /> delete {selected.size} file(s)?
            </h3>
            <p style={{ fontSize: 13, color: '#4b5563', lineHeight: 1.5, margin: '0 0 20px 0', textTransform: 'lowercase' }}>
              this will remove {selected.size} file(s) from reyna and move them to drive trash.
            </p>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button
                onClick={() => setShowBatchDelete(false)}
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 9999,
                  padding: '7px 16px',
                  fontSize: 12,
                  fontWeight: 500,
                  color: '#374151',
                  cursor: 'pointer',
                  textTransform: 'lowercase',
                }}
              >
                cancel
              </button>
              <button
                onClick={doBatchDelete}
                disabled={deleting}
                style={{
                  background: '#dc2626',
                  border: '1px solid #dc2626',
                  borderRadius: 9999,
                  padding: '7px 16px',
                  fontSize: 12,
                  fontWeight: 600,
                  color: '#ffffff',
                  cursor: 'pointer',
                  opacity: deleting ? 0.6 : 1,
                  textTransform: 'lowercase',
                }}
              >
                {deleting ? 'deleting...' : `delete ${selected.size} files`}
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}
    </div>
  )
}
