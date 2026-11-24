import { useState, useEffect, useRef } from 'react'
import { api } from '../lib/api'
import { Fa, icons } from './icons'
import { notify } from './Notifications'

export default function Spotlight({ isOpen, onClose }) {
  const [query, setQuery] = useState('')
  const [files, setFiles] = useState([])
  const [selectedIndex, setSelectedIndex] = useState(0)
  const [loading, setLoading] = useState(false)
  const inputRef = useRef(null)
  const listRef = useRef(null)

  useEffect(() => {
    if (isOpen) {
      loadFiles()
      setSelectedIndex(0)
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [isOpen])

  const loadFiles = async () => {
    setLoading(true)
    try {
      const res = await api.files('', 150)
      if (Array.isArray(res)) {
        setFiles(res)
      } else if (res?.files) {
        setFiles(res.files)
      }
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }

  // Filter files based on query
  const filtered = files.filter(f => {
    if (!query.trim()) return true
    const q = query.toLowerCase()
    return (
      (f.file_name && f.file_name.toLowerCase().includes(q)) ||
      (f.subject && f.subject.toLowerCase().includes(q)) ||
      (f.shared_by_name && f.shared_by_name.toLowerCase().includes(q)) ||
      (f.content_summary && f.content_summary.toLowerCase().includes(q))
    )
  })

  // Keep selected index within bounds
  useEffect(() => {
    if (selectedIndex >= filtered.length) {
      setSelectedIndex(Math.max(0, filtered.length - 1))
    }
  }, [filtered.length, selectedIndex])

  // Scroll active item into view
  useEffect(() => {
    if (listRef.current) {
      const activeEl = listRef.current.children[selectedIndex]
      if (activeEl) {
        activeEl.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
      }
    }
  }, [selectedIndex])

  // Handle global key events when modal is open
  useEffect(() => {
    if (!isOpen) return

    const handleKeyDown = (e) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSelectedIndex(prev => (prev + 1 < filtered.length ? prev + 1 : 0))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSelectedIndex(prev => (prev - 1 >= 0 ? prev - 1 : filtered.length - 1))
      } else if (e.key === 'Enter') {
        e.preventDefault()
        const selected = filtered[selectedIndex]
        if (selected) copyWindowsPath(selected)
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'd') {
        e.preventDefault()
        const selected = filtered[selectedIndex]
        if (selected) copyDriveLink(selected)
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
        e.preventDefault()
        const selected = filtered[selectedIndex]
        if (selected) copySummary(selected)
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'o') {
        e.preventDefault()
        const selected = filtered[selectedIndex]
        if (selected) openInDrive(selected)
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, filtered, selectedIndex])

  if (!isOpen) return null

  const selected = filtered[selectedIndex]

  const formatSize = (bytes) => {
    if (!bytes) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB']
    let b = bytes
    for (const u of units) {
      if (b < 1024) return `${b.toFixed(1)} ${u}`
      b /= 1024
    }
    return `${b.toFixed(1)} TB`
  }

  const getWindowsPath = (file) => {
    return `\\\\wsl.localhost\\Ubuntu\\home\\bro\\code\\reyna\\drive_storage\\files\\${file.file_name}`
  }

  const getDriveUrl = (file) => {
    if (file.drive_file_id && !file.drive_file_id.startsWith('local_')) {
      return `https://drive.google.com/file/d/${file.drive_file_id}/view`
    }
    return ''
  }

  const copyWindowsPath = (file) => {
    const path = getWindowsPath(file)
    navigator.clipboard.writeText(path)
    notify.success(`copied file path to clipboard: ready to paste in any file upload or email!`)
  }

  const copyDriveLink = (file) => {
    const url = getDriveUrl(file)
    if (url) {
      navigator.clipboard.writeText(url)
      notify.success('copied google drive link to clipboard!')
    } else {
      notify.error('file has not been pushed to google drive yet')
    }
  }

  const copySummary = (file) => {
    if (file.content_summary) {
      navigator.clipboard.writeText(file.content_summary)
      notify.success('copied ai summary to clipboard!')
    } else {
      notify.error('no ai summary generated yet for this file')
    }
  }

  const openInDrive = (file) => {
    const url = getDriveUrl(file)
    if (url) {
      window.open(url, '_blank')
    } else {
      notify.error('file is stored locally on machine')
    }
  }

  const openLocalDownload = (file) => {
    window.open(api.downloadUrl(file.id), '_blank')
  }

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 9999,
        background: 'rgba(15, 23, 42, 0.65)',
        backdropFilter: 'blur(16px)',
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'center',
        paddingTop: '8vh',
        animation: 'fadeIn 0.15s ease-out',
      }}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: '92%',
          maxWidth: 920,
          maxHeight: '82vh',
          background: '#ffffff',
          borderRadius: 20,
          boxShadow: '0 25px 60px -15px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(0, 0, 0, 0.08)',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
          fontFamily: 'inherit',
          animation: 'slideDown 0.18s cubic-bezier(0.16, 1, 0.3, 1)',
        }}
      >
        {/* ── Top Search Input Header ── */}
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: 14,
          padding: '18px 24px',
          borderBottom: '1px solid #f1f5f9',
          background: '#ffffff',
        }}>
          <Fa icon="fa-bolt" style={{ color: '#22c55e', fontSize: 18 }} />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={e => {
              setQuery(e.target.value)
              setSelectedIndex(0)
            }}
            placeholder="type to retrieve any document across all groups... (e.g. physics, receipt, harsh)"
            style={{
              flex: 1,
              border: 'none',
              outline: 'none',
              fontSize: 16,
              color: '#0f172a',
              background: 'transparent',
              fontFamily: 'inherit',
              textTransform: 'lowercase',
            }}
          />
          {query && (
            <button
              onClick={() => setQuery('')}
              style={{
                background: '#f1f5f9',
                border: 'none',
                borderRadius: '50%',
                width: 24,
                height: 24,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#64748b',
                fontSize: 11,
              }}
            >
              <Fa icon={icons.close} />
            </button>
          )}
          <span style={{
            fontSize: 11,
            color: '#94a3b8',
            background: '#f8fafc',
            border: '1px solid #e2e8f0',
            padding: '3px 8px',
            borderRadius: 6,
            fontWeight: 500,
            textTransform: 'uppercase',
          }}>
            esc
          </span>
        </div>

        {/* ── Dual Pane Layout: Left List, Right Inspector ── */}
        <div style={{ display: 'flex', flex: 1, overflow: 'hidden', minHeight: 460 }}>
          {/* Left Column: Result List */}
          <div
            ref={listRef}
            style={{
              width: '45%',
              borderRight: '1px solid #f1f5f9',
              overflowY: 'auto',
              padding: '10px 10px',
              display: 'flex',
              flexDirection: 'column',
              gap: 4,
              background: '#fafafa',
            }}
          >
            {loading ? (
              <div style={{ padding: 30, textAlign: 'center', color: '#94a3b8', fontSize: 13 }}>
                <Fa icon={icons.loading} spin style={{ marginRight: 8 }} /> loading indexed files...
              </div>
            ) : filtered.length === 0 ? (
              <div style={{ padding: '40px 20px', textAlign: 'center', color: '#94a3b8', fontSize: 13 }}>
                no documents matching "{query}"
              </div>
            ) : (
              filtered.map((file, idx) => {
                const isSelected = idx === selectedIndex
                return (
                  <div
                    key={file.id}
                    onClick={() => setSelectedIndex(idx)}
                    onDoubleClick={() => copyWindowsPath(file)}
                    style={{
                      padding: '10px 12px',
                      borderRadius: 12,
                      cursor: 'pointer',
                      background: isSelected ? '#111827' : '#ffffff',
                      color: isSelected ? '#ffffff' : '#1e293b',
                      border: isSelected ? '1px solid #111827' : '1px solid #f1f5f9',
                      transition: 'all 0.1s ease',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 4,
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <span style={{
                        fontSize: 13,
                        fontWeight: 600,
                        whiteSpace: 'nowrap',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        color: isSelected ? '#ffffff' : '#0f172a',
                        textTransform: 'lowercase',
                      }}>
                        {file.file_name}
                      </span>
                      <span style={{
                        fontSize: 10,
                        fontWeight: 600,
                        padding: '2px 7px',
                        borderRadius: 9999,
                        background: isSelected ? 'rgba(255,255,255,0.2)' : '#e2e8f0',
                        color: isSelected ? '#ffffff' : '#475569',
                        textTransform: 'lowercase',
                        flexShrink: 0,
                      }}>
                        {file.subject || 'general'}
                      </span>
                    </div>

                    <div style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      fontSize: 11,
                      color: isSelected ? '#94a3b8' : '#64748b',
                      textTransform: 'lowercase',
                    }}>
                      <span>{file.shared_by_name ? `by ${file.shared_by_name}` : 'captured'}</span>
                      <span>{formatSize(file.file_size)}</span>
                    </div>
                  </div>
                )
              })
            )}
          </div>

          {/* Right Column: Rich Inspector */}
          <div style={{
            flex: 1,
            padding: '24px 28px',
            overflowY: 'auto',
            background: '#ffffff',
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
          }}>
            {selected ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
                {/* Header */}
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
                    <span style={{
                      fontSize: 11,
                      fontWeight: 600,
                      padding: '3px 10px',
                      borderRadius: 9999,
                      background: '#dcfce7',
                      color: '#15803d',
                      textTransform: 'lowercase',
                    }}>
                      {selected.subject || 'uncategorized'}
                    </span>
                    <span style={{
                      fontSize: 11,
                      color: '#64748b',
                      padding: '3px 8px',
                      borderRadius: 6,
                      background: '#f1f5f9',
                    }}>
                      {selected.mime_type || 'document'}
                    </span>
                    <span style={{ fontSize: 11, color: '#94a3b8' }}>
                      {formatSize(selected.file_size)}
                    </span>
                  </div>

                  <h3 style={{
                    fontSize: 18,
                    fontWeight: 700,
                    color: '#0f172a',
                    margin: 0,
                    textTransform: 'lowercase',
                    lineHeight: 1.3,
                  }}>
                    {selected.file_name}
                  </h3>
                </div>

                {/* Metadata Row */}
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 1fr',
                  gap: 12,
                  padding: '12px 16px',
                  background: '#f8fafc',
                  borderRadius: 12,
                  border: '1px solid #f1f5f9',
                  fontSize: 12,
                }}>
                  <div>
                    <span style={{ color: '#94a3b8', display: 'block', marginBottom: 2 }}>shared by:</span>
                    <span style={{ fontWeight: 600, color: '#1e293b' }}>
                      {selected.shared_by_name || selected.shared_by_phone || 'group member'}
                    </span>
                  </div>
                  <div>
                    <span style={{ color: '#94a3b8', display: 'block', marginBottom: 2 }}>captured:</span>
                    <span style={{ fontWeight: 600, color: '#1e293b' }}>
                      {selected.created_at ? new Date(selected.created_at).toLocaleDateString() : 'recent'}
                    </span>
                  </div>
                </div>

                {/* Google Drive Status */}
                <div style={{ fontSize: 12 }}>
                  <span style={{ color: '#94a3b8', display: 'block', marginBottom: 4 }}>cloud storage:</span>
                  {selected.drive_file_id && !selected.drive_file_id.startsWith('local_') ? (
                    <a
                      href={getDriveUrl(selected)}
                      target="_blank"
                      rel="noreferrer"
                      style={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: 6,
                        color: '#0284c7',
                        fontWeight: 600,
                        textDecoration: 'none',
                        wordBreak: 'break-all',
                      }}
                    >
                      <Fa icon="fa-cloud" style={{ color: '#22c55e' }} /> synced to google drive
                      <Fa icon="fa-arrow-up-right-from-square" style={{ fontSize: 10 }} />
                    </a>
                  ) : (
                    <span style={{ color: '#64748b' }}>
                      <Fa icon="fa-hard-drive" style={{ marginRight: 6 }} /> saved locally on machine
                    </span>
                  )}
                </div>

                {/* AI Summary */}
                <div style={{
                  padding: '14px 16px',
                  background: '#f8fafc',
                  borderRadius: 14,
                  border: '1px solid #e2e8f0',
                }}>
                  <div style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                    fontSize: 11,
                    fontWeight: 700,
                    color: '#0f172a',
                    textTransform: 'uppercase',
                    letterSpacing: '0.04em',
                    marginBottom: 8,
                  }}>
                    <Fa icon="fa-sparkles" style={{ color: '#eab308' }} /> ai content summary
                  </div>
                  <div style={{
                    fontSize: 13,
                    color: '#334155',
                    lineHeight: 1.6,
                    whiteSpace: 'pre-line',
                    maxHeight: 140,
                    overflowY: 'auto',
                  }}>
                    {selected.content_summary || 'no ai summary generated yet for this document.'}
                  </div>
                </div>
              </div>
            ) : (
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#94a3b8' }}>
                select a document to view details
              </div>
            )}

            {/* Action Buttons Bar */}
            {selected && (
              <div style={{
                display: 'flex',
                flexWrap: 'wrap',
                gap: 8,
                paddingTop: 16,
                borderTop: '1px solid #f1f5f9',
                marginTop: 18,
              }}>
                <button
                  onClick={() => copyWindowsPath(selected)}
                  style={{
                    background: '#111827',
                    color: '#ffffff',
                    border: 'none',
                    borderRadius: 10,
                    padding: '8px 16px',
                    fontSize: 12,
                    fontWeight: 600,
                    cursor: 'pointer',
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    fontFamily: 'inherit',
                    textTransform: 'lowercase',
                  }}
                >
                  <Fa icon="fa-copy" /> copy path (ctrl+v)
                </button>

                <button
                  onClick={() => openLocalDownload(selected)}
                  style={{
                    background: '#f1f5f9',
                    color: '#0f172a',
                    border: '1px solid #e2e8f0',
                    borderRadius: 10,
                    padding: '8px 14px',
                    fontSize: 12,
                    fontWeight: 600,
                    cursor: 'pointer',
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    fontFamily: 'inherit',
                    textTransform: 'lowercase',
                  }}
                >
                  <Fa icon="fa-arrow-down" /> download / open
                </button>

                {selected.drive_file_id && !selected.drive_file_id.startsWith('local_') && (
                  <button
                    onClick={() => copyDriveLink(selected)}
                    style={{
                      background: '#ffffff',
                      color: '#0284c7',
                      border: '1px solid #bae6fd',
                      borderRadius: 10,
                      padding: '8px 14px',
                      fontSize: 12,
                      fontWeight: 600,
                      cursor: 'pointer',
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 6,
                      fontFamily: 'inherit',
                      textTransform: 'lowercase',
                    }}
                  >
                    <Fa icon="fa-cloud" /> copy drive link
                  </button>
                )}

                {selected.content_summary && (
                  <button
                    onClick={() => copySummary(selected)}
                    style={{
                      background: '#ffffff',
                      color: '#854d0e',
                      border: '1px solid #fef08a',
                      borderRadius: 10,
                      padding: '8px 14px',
                      fontSize: 12,
                      fontWeight: 600,
                      cursor: 'pointer',
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 6,
                      fontFamily: 'inherit',
                      textTransform: 'lowercase',
                    }}
                  >
                    <Fa icon="fa-sparkles" /> copy summary
                  </button>
                )}
              </div>
            )}
          </div>
        </div>

        {/* ── Footer Keyboard Legend ── */}
        <div style={{
          padding: '10px 24px',
          background: '#f8fafc',
          borderTop: '1px solid #f1f5f9',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: 11,
          color: '#64748b',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <span><strong style={{ color: '#0f172a' }}>↵</strong> copy path & open</span>
            <span><strong style={{ color: '#0f172a' }}>↑↓</strong> navigate</span>
            <span><strong style={{ color: '#0f172a' }}>ctrl+d</strong> copy drive link</span>
            <span><strong style={{ color: '#0f172a' }}>ctrl+s</strong> copy summary</span>
          </div>
          <div>
            <span>press <strong style={{ color: '#0f172a' }}>ctrl+k</strong> anywhere in reyna</span>
          </div>
        </div>
      </div>
    </div>
  )
}
