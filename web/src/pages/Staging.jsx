import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa, icons, fileIconClass } from '../components/icons'

function formatBytes(b) {
  if (!b || b === 0) return '0 B'
  if (b < 1024) return b + ' B'
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB'
  return (b / 1073741824).toFixed(1) + ' GB'
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

export function StagingView({ isModal = false, onClose }) {
  const navigate = useNavigate()
  const [stagedFiles, setStagedFiles] = useState([])
  const [loading, setLoading] = useState(true)
  const [committing, setCommitting] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [activeTab, setActiveTab] = useState('all') // 'all' | 'ready' | 'classifying' | 'pdf' | 'images' | 'others'
  const [selectedIds, setSelectedIds] = useState(new Set())

  const fetchStaged = async () => {
    try {
      const files = await api.files('', 200)
      const staged = (files || []).filter(f => f.status === 'staged')
      setStagedFiles(staged)
    } catch (err) {
      notify.error('Failed to load staged files: ' + (err.message || err))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchStaged()
    const interval = setInterval(fetchStaged, 5000)
    return () => clearInterval(interval)
  }, [])

  const removeFromStaging = async (fileId) => {
    try {
      await api.removeFromStaging(fileId)
      setStagedFiles(prev => prev.filter(f => f.id !== fileId))
      setSelectedIds(prev => {
        const next = new Set(prev)
        next.delete(fileId)
        return next
      })
      notify.success('File removed from staging')
    } catch (err) {
      notify.error('Failed to remove: ' + (err.message || err))
    }
  }

  const clearAllStaged = async () => {
    if (stagedFiles.length === 0) return
    if (!window.confirm('Clear all files from staging area?')) return
    try {
      for (const f of stagedFiles) {
        await api.removeFromStaging(f.id)
      }
      setStagedFiles([])
      setSelectedIds(new Set())
      notify.success('Cleared all files from staging')
    } catch (err) {
      notify.error('Error clearing staging: ' + (err.message || err))
    }
  }

  const removeSelected = async () => {
    if (selectedIds.size === 0) return
    try {
      for (const id of selectedIds) {
        await api.removeFromStaging(id)
      }
      setStagedFiles(prev => prev.filter(f => !selectedIds.has(f.id)))
      setSelectedIds(new Set())
      notify.success('Removed selected files from staging')
    } catch (err) {
      notify.error('Error removing selected: ' + (err.message || err))
    }
  }

  const commitAllStaged = async () => {
    setCommitting(true)
    notify.info('Pushing staged files to Google Drive...')
    try {
      const res = await api.commitStaged()
      if (res?.error) {
        notify.error('Commit failed: ' + res.error)
      } else {
        notify.success(`Pushed ${res?.committed || stagedFiles.length} file(s) to Google Drive!`)
        await fetchStaged()
      }
    } catch (err) {
      notify.error('Commit failed: ' + (err.message || err))
    } finally {
      setCommitting(false)
    }
  }

  const toggleSelect = (id) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleSelectAll = () => {
    if (selectedIds.size === filteredFiles.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(filteredFiles.map(f => f.id)))
    }
  }

  // Filter & Search
  const filteredFiles = stagedFiles.filter(f => {
    const name = (f.file_name || '').toLowerCase()
    const subject = (f.subject || '').toLowerCase()
    const q = searchQuery.toLowerCase().trim()
    const matchesSearch = !q || name.includes(q) || subject.includes(q)

    if (!matchesSearch) return false
    const ext = name.split('.').pop() || ''
    const isClassifying = f.subject === 'classifying...'

    if (activeTab === 'ready') return !isClassifying
    if (activeTab === 'classifying') return isClassifying
    if (activeTab === 'pdf') return ext === 'pdf'
    if (activeTab === 'images') return ['png', 'jpg', 'jpeg', 'webp', 'gif'].includes(ext)
    if (activeTab === 'others') return ext !== 'pdf' && !['png', 'jpg', 'jpeg', 'webp', 'gif'].includes(ext)
    return true
  })

  // Stats
  const totalCount = stagedFiles.length
  const classifyingCount = stagedFiles.filter(f => f.subject === 'classifying...').length
  const readyCount = totalCount - classifyingCount

  return (
    <div style={{
      maxWidth: isModal ? '100%' : 1200,
      margin: '0 auto',
      padding: isModal ? '4px 0 20px' : '8px 4px 60px',
    }}>
      {/* ── Breadcrumb / Header Row ── */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: 16,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#6b7280' }}>
          <span
            style={{ cursor: 'pointer' }}
            onClick={() => {
              if (isModal && onClose) onClose()
              else navigate('/dashboard')
            }}
          >
            reyna
          </span>
          <span style={{ color: '#d1d5db' }}>/</span>
          <span style={{ color: '#111827', fontWeight: 600 }}>staging area</span>
        </div>

        {isModal && onClose && (
          <button
            onClick={onClose}
            style={{
              background: '#f3f4f6',
              border: 'none',
              borderRadius: '50%',
              width: 32,
              height: 32,
              cursor: 'pointer',
              color: '#4b5563',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 14,
              transition: 'all 0.15s',
            }}
            onMouseEnter={e => e.currentTarget.style.background = '#e5e7eb'}
            onMouseLeave={e => e.currentTarget.style.background = '#f3f4f6'}
          >
            <Fa icon={icons.close} />
          </button>
        )}
      </div>

      {/* ── Main Staged Files Table Container ── */}
      <div style={{
        background: '#ffffff',
        border: '1px solid #e5e7eb',
        borderRadius: 16,
        padding: '20px 22px',
        boxShadow: '0 1px 3px rgba(0,0,0,0.03)',
      }}>
        {/* Controls Bar */}
        <div style={{
          display: 'flex',
          flexWrap: 'wrap',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 14,
          marginBottom: 18,
        }}>
          {/* Left Title & Filter Tabs */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 14, flexWrap: 'wrap' }}>
            <h2 style={{ fontSize: 18, fontWeight: 700, color: '#111827', margin: 0, textTransform: 'lowercase', letterSpacing: '-0.01em' }}>
              staged files
            </h2>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
              {[
                { key: 'all', label: `all (${totalCount})` },
                { key: 'ready', label: `ready (${readyCount})` },
                { key: 'classifying', label: `classifying (${classifyingCount})` },
                { key: 'pdf', label: 'pdf' },
                { key: 'images', label: 'images' },
              ].map(tab => {
                const isSelected = activeTab === tab.key
                return (
                  <button
                    key={tab.key}
                    onClick={() => setActiveTab(tab.key)}
                    style={{
                      background: isSelected ? '#111827' : '#ffffff',
                      color: isSelected ? '#ffffff' : '#6b7280',
                      border: isSelected ? '1px solid #111827' : '1px solid #e5e7eb',
                      borderRadius: 9999,
                      padding: '5px 13px',
                      fontSize: 12,
                      fontWeight: 500,
                      cursor: 'pointer',
                      fontFamily: 'inherit',
                      textTransform: 'lowercase',
                      transition: 'all 0.15s',
                    }}
                  >
                    {tab.label}
                  </button>
                )
              })}
            </div>
          </div>

          {/* Right Search Input & Header Actions */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <div style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
              <Fa icon="fa-search" style={{ position: 'absolute', left: 12, fontSize: 12, color: '#9ca3af' }} />
              <input
                type="text"
                placeholder="search staged files..."
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
                }}
              />
            </div>

            {stagedFiles.length > 0 && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <button
                  onClick={clearAllStaged}
                  style={{
                    background: '#ffffff',
                    border: '1px solid #e5e7eb',
                    borderRadius: 10,
                    padding: '6px 14px',
                    fontSize: 12,
                    fontWeight: 600,
                    color: '#dc2626',
                    cursor: 'pointer',
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    fontFamily: 'inherit',
                    textTransform: 'lowercase',
                    transition: 'all 0.15s',
                  }}
                  onMouseEnter={e => {
                    e.currentTarget.style.borderColor = '#fca5a5'
                    e.currentTarget.style.background = '#fef2f2'
                  }}
                  onMouseLeave={e => {
                    e.currentTarget.style.borderColor = '#e5e7eb'
                    e.currentTarget.style.background = '#ffffff'
                  }}
                >
                  <Fa icon={icons.close} style={{ fontSize: 9 }} /> clear all
                </button>

                <button
                  onClick={commitAllStaged}
                  disabled={committing}
                  style={{
                    background: '#22c55e',
                    border: 'none',
                    borderRadius: 10,
                    padding: '7px 18px',
                    fontSize: 13,
                    fontWeight: 700,
                    color: '#ffffff',
                    cursor: committing ? 'not-allowed' : 'pointer',
                    opacity: committing ? 0.7 : 1,
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    fontFamily: 'inherit',
                    textTransform: 'lowercase',
                    transition: 'all 0.15s',
                  }}
                  onMouseEnter={e => { if (!committing) e.currentTarget.style.background = '#16a34a' }}
                  onMouseLeave={e => { if (!committing) e.currentTarget.style.background = '#22c55e' }}
                >
                  {committing ? (
                    <>
                      <Fa icon={icons.loading} spin style={{ fontSize: 10 }} /> pushing...
                    </>
                  ) : (
                    <>
                      push all to drive <Fa icon={icons.arrowRight} style={{ fontSize: 10 }} />
                    </>
                  )}
                </button>
              </div>
            )}
          </div>
        </div>

        {/* Bulk Selected Bar */}
        {selectedIds.size > 0 && (
          <div style={{
            background: '#f0fdf4',
            border: '1px solid #bbf7d0',
            borderRadius: 10,
            padding: '8px 14px',
            marginBottom: 14,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            fontSize: 12,
            color: '#166534',
          }}>
            <span style={{ fontWeight: 600, textTransform: 'lowercase' }}>
              {selectedIds.size} file(s) selected
            </span>
            <div style={{ display: 'flex', gap: 8 }}>
              <button
                onClick={removeSelected}
                style={{
                  background: '#ffffff',
                  border: '1px solid #fca5a5',
                  color: '#dc2626',
                  borderRadius: 6,
                  padding: '4px 10px',
                  fontSize: 11,
                  fontWeight: 600,
                  cursor: 'pointer',
                  textTransform: 'lowercase',
                }}
              >
                remove selected
              </button>
            </div>
          </div>
        )}

        {/* Table Content */}
        {loading ? (
          <div style={{ padding: '48px 0', textAlign: 'center', color: '#9ca3af', fontSize: 13 }}>
            <Fa icon={icons.loading} spin style={{ fontSize: 15, marginRight: 8 }} />
            loading staged files...
          </div>
        ) : filteredFiles.length === 0 ? (
          <div style={{ padding: '56px 16px', textAlign: 'center' }}>
            <div style={{
              width: 44,
              height: 44,
              borderRadius: '50%',
              background: '#ecfdf5',
              color: '#10b981',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 18,
              marginBottom: 12,
            }}>
              <Fa icon={icons.staging} />
            </div>
            <div style={{ fontSize: 15, fontWeight: 700, color: '#111827', marginBottom: 4, textTransform: 'lowercase' }}>
              no files in staging
            </div>
            <p style={{ fontSize: 13, color: '#6b7280', margin: 0, textTransform: 'lowercase' }}>
              {searchQuery ? 'no files match your search query' : 'files shared in your tracked whatsapp groups will appear here.'}
            </p>
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #f3f4f6' }}>
                  <th style={{ width: 40, padding: '10px 12px' }}>
                    <input
                      type="checkbox"
                      checked={selectedIds.size > 0 && selectedIds.size === filteredFiles.length}
                      onChange={toggleSelectAll}
                      style={{ cursor: 'pointer' }}
                    />
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    file name
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    target subject / folder
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    size
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    commit deadline
                  </th>
                  <th style={{ width: 60, padding: '10px 12px', textAlign: 'right', fontSize: 12, fontWeight: 600, color: '#6b7280' }}>
                  </th>
                </tr>
              </thead>
              <tbody>
                {filteredFiles.map((f) => {
                  const isChecked = selectedIds.has(f.id)
                  const isClassifying = f.subject === 'classifying...'

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

                      {/* File Icon & Name */}
                      <td style={{ padding: '14px 14px' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                          <div style={{
                            width: 36,
                            height: 36,
                            borderRadius: 10,
                            background: '#ecfdf5',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            flexShrink: 0,
                          }}>
                            <Fa icon={fileIconClass(f.mime_type, f.file_name)} style={{ fontSize: 14, color: '#111827' }} />
                          </div>
                          <div style={{ minWidth: 0 }}>
                            <div style={{
                              fontSize: 14,
                              fontWeight: 700,
                              color: '#111827',
                              textTransform: 'lowercase',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              whiteSpace: 'nowrap',
                              maxWidth: 340,
                            }}>
                              {(f.file_name || '').toLowerCase()}
                            </div>
                            <div style={{ fontSize: 12, color: '#9ca3af', textTransform: 'lowercase' }}>
                              by {(f.shared_by_name || 'unknown').toLowerCase()}
                            </div>
                          </div>
                        </div>
                      </td>

                      {/* Subject Badge */}
                      <td style={{ padding: '14px 14px' }}>
                        {isClassifying ? (
                          <span style={{
                            fontSize: 11,
                            fontWeight: 600,
                            color: '#d97706',
                            background: '#fef3c7',
                            padding: '3px 9px',
                            borderRadius: 9999,
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 5,
                            textTransform: 'lowercase',
                          }}>
                            <Fa icon={icons.loading} spin style={{ fontSize: 9 }} /> classifying...
                          </span>
                        ) : (
                          <span style={{
                            fontSize: 12,
                            fontWeight: 600,
                            color: '#15803d',
                            background: '#dcfce7',
                            padding: '3px 9px',
                            borderRadius: 9999,
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 5,
                            textTransform: 'lowercase',
                          }}>
                            <Fa icon={icons.folder} style={{ fontSize: 10 }} />
                            {(f.subject || 'general').toLowerCase()}
                          </span>
                        )}
                      </td>

                      {/* File Size */}
                      <td style={{ padding: '14px 14px', fontSize: 13, color: '#6b7280', textTransform: 'lowercase' }}>
                        {formatBytes(f.file_size)}
                      </td>

                      {/* Deadline Countdown */}
                      <td style={{ padding: '14px 14px', fontSize: 13, fontWeight: 500, color: '#111827', textTransform: 'lowercase' }}>
                        {hoursUntilCommit(f.created_at)}
                      </td>

                      {/* Actions */}
                      <td style={{ padding: '14px 12px', textAlign: 'right' }}>
                        <button
                          onClick={() => removeFromStaging(f.id)}
                          title="Remove from staging"
                          style={{
                            background: '#ffffff',
                            border: '1px solid #e5e7eb',
                            borderRadius: 9999,
                            padding: '2px 8px',
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
                          <Fa icon={icons.close} style={{ fontSize: 9 }} />
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

export default function Staging() {
  return <StagingView isModal={false} />
}
