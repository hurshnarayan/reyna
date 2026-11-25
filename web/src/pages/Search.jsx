import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { createPortal } from 'react-dom'
import { api, getToken } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa, icons, fileIconClass, IconBox } from '../components/icons'

function formatBytes(b) {
  if (b < 1024) return b + ' B'
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'
  return (b / 1048576).toFixed(1) + ' MB'
}

function timeAgo(d) {
  if (!d) return ''
  const s = Math.floor((Date.now() - new Date(d).getTime()) / 1000)
  if (isNaN(s)) return ''
  if (s < 60) return 'just now'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

function formatClockTime(ts) {
  const d = ts ? new Date(ts) : new Date()
  return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
}

function cleanReply(s) {
  if (!s) return ''
  let t = String(s).trim()
  if (t.startsWith('```')) {
    t = t.replace(/^```(?:json|md|markdown)?/i, '').replace(/```$/, '').trim()
  }
  if (t.startsWith('{') && t.endsWith('}')) {
    try {
      const obj = JSON.parse(t)
      for (const k of ['answer', 'reply', 'response', 'text', 'output', 'result', 'message']) {
        if (typeof obj[k] === 'string' && obj[k].trim()) return obj[k].trim()
      }
    } catch {}
  }
  return t
}

function renderMarkdown(text) {
  if (!text) return null
  const lines = text.split('\n')
  const blocks = []
  let listBuf = []
  const flushList = () => {
    if (listBuf.length > 0) {
      blocks.push(
        <ul key={'ul' + blocks.length} style={{ margin: '6px 0 6px 18px', padding: 0 }}>
          {listBuf.map((item, i) => (
            <li key={i} style={{ marginBottom: 4 }}>
              {renderInline(item)}
            </li>
          ))}
        </ul>
      )
      listBuf = []
    }
  }
  lines.forEach((rawLine, i) => {
    const line = rawLine.replace(/\r$/, '')
    if (/^\s*[-•*]\s+/.test(line)) {
      listBuf.push(line.replace(/^\s*[-•*]\s+/, ''))
      return
    }
    flushList()
    if (line.trim() === '') {
      blocks.push(<div key={'br' + i} style={{ height: 6 }} />)
      return
    }
    if (line.startsWith('> ')) {
      blocks.push(
        <blockquote
          key={'q' + i}
          style={{
            margin: '6px 0',
            padding: '6px 12px',
            borderLeft: '3px solid var(--reyna-accent)',
            background: 'rgba(37,211,102,0.06)',
            fontStyle: 'italic',
            borderRadius: 4,
          }}
        >
          {renderInline(line.slice(2))}
        </blockquote>
      )
      return
    }
    blocks.push(
      <p key={'p' + i} style={{ margin: '4px 0', lineHeight: 1.65 }}>
        {renderInline(line)}
      </p>
    )
  })
  flushList()
  return blocks
}

function renderInline(text) {
  const parts = []
  const re = /(\*\*[^*]+\*\*|`[^`]+`)/g
  let lastIdx = 0
  let m
  let key = 0
  while ((m = re.exec(text)) !== null) {
    if (m.index > lastIdx) parts.push(text.slice(lastIdx, m.index))
    const tok = m[0]
    if (tok.startsWith('**')) {
      parts.push(
        <strong key={'b' + key++} style={{ fontWeight: 700 }}>
          {tok.slice(2, -2)}
        </strong>
      )
    } else if (tok.startsWith('`')) {
      parts.push(
        <code
          key={'c' + key++}
          style={{
            background: 'rgba(0,0,0,0.06)',
            padding: '1px 6px',
            borderRadius: 4,
            fontSize: '0.92em',
            fontFamily: 'var(--font-mono)',
          }}
        >
          {tok.slice(1, -1)}
        </code>
      )
    }
    lastIdx = m.index + tok.length
  }
  if (lastIdx < text.length) parts.push(text.slice(lastIdx))
  return parts
}

export default function Search() {
  const navigate = useNavigate()
  const [mode, setMode] = useState('chat') // 'chat' | 'search'
  const [query, setQuery] = useState('')
  const [results, setResults] = useState(null)
  const [loading, setLoading] = useState(false)
  const [suggestions, setSuggestions] = useState([])
  const [showSuggestions, setShowSuggestions] = useState(false)
  const [selectedSuggestion, setSelectedSuggestion] = useState(-1)
  const [previewFile, setPreviewFile] = useState(null)
  const [previewData, setPreviewData] = useState(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [deleting, setDeleting] = useState(false)

  // Conversational Chat Thread
  const [chatConversation, setChatConversation] = useState([])
  const chatEndRef = useRef(null)
  const inputRef = useRef(null)
  const debounceRef = useRef(null)

  useEffect(() => {
    if (chatEndRef.current) {
      chatEndRef.current.scrollIntoView({ behavior: 'smooth', block: 'end' })
    }
  }, [chatConversation, loading])

  // Instant File Search (Mode: search)
  const doLiveSearch = useCallback(async (q) => {
    if (!q.trim() || q.trim().length < 2) {
      setResults(null)
      return
    }
    setLoading(true)
    notify.showLoader()
    const files = await api.search(q.trim())
    setResults(files || [])
    setLoading(false)
    notify.hideLoader()
  }, [])

  // Send Conversational Query (Mode: chat)
  const sendChat = async (promptText, fileIds = []) => {
    const q = promptText.trim()
    if (!q) return
    setLoading(true)
    notify.showLoader()

    const turnId = Date.now()
    const userTurn = {
      id: turnId,
      question: q,
      reply: '',
      files: [],
      citations: [],
      candidates: [],
      notice: '',
      status: 'answered',
      pending: true,
      showSources: false,
      ts: turnId,
    }

    setChatConversation(prev => [...prev, userTurn])
    setQuery('')

    if (inputRef.current) {
      inputRef.current.style.height = 'auto'
    }

    // Build chat history for conversational follow-ups
    const history = chatConversation
      .filter(t => !t.pending)
      .flatMap(t => [
        { role: 'user', text: t.question },
        {
          role: 'assistant',
          text: t.reply,
          file_names: (t.files || []).map(f => f.file_name),
          file_ids: (t.files || []).map(f => f.id),
        },
      ])

    try {
      const resp = await api.nlpRetrieve(q, '', history, fileIds)
      setChatConversation(prev => {
        const copy = [...prev]
        const idx = copy.findIndex(t => t.id === turnId)
        if (idx !== -1) {
          copy[idx] = {
            ...copy[idx],
            reply: cleanReply(resp?.reply || 'I could not find anything matching that in your documents.'),
            files: resp?.files || [],
            citations: resp?.citations || [],
            candidates: resp?.candidates || [],
            notice: resp?.notice || '',
            status: resp?.status || 'answered',
            pending: false,
          }
        }
        return copy
      })
    } catch (err) {
      setChatConversation(prev => {
        const copy = [...prev]
        const idx = copy.findIndex(t => t.id === turnId)
        if (idx !== -1) {
          copy[idx] = {
            ...copy[idx],
            reply: 'Sorry, I encountered an error while processing your request. Please try again.',
            pending: false,
          }
        }
        return copy
      })
    } finally {
      setLoading(false)
      notify.hideLoader()
    }
  }

  const handleChooseCandidate = (candidate, originalQuestion) => {
    sendChat(originalQuestion || candidate.file_name, [candidate.file_id])
  }

  const toggleSources = (turnId) => {
    setChatConversation(prev =>
      prev.map(t => (t.id === turnId ? { ...t, showSources: !t.showSources } : t))
    )
  }

  const clearChat = () => {
    setChatConversation([])
    setQuery('')
  }

  const fetchSuggestions = useCallback(async (q) => {
    if (!q.trim() || q.startsWith('/content:')) {
      setSuggestions([])
      return
    }
    const names = await api.suggest(q.trim())
    setSuggestions(names || [])
  }, [])

  const handleChange = (e) => {
    const val = e.target.value
    setQuery(val)
    setSelectedSuggestion(-1)
    if (mode === 'search') {
      if (debounceRef.current) clearTimeout(debounceRef.current)
      debounceRef.current = setTimeout(() => {
        if (val.trim().length >= 2) {
          fetchSuggestions(val.trim())
          setShowSuggestions(true)
          doLiveSearch(val)
        } else {
          setSuggestions([])
          setShowSuggestions(false)
          if (val.trim().length === 0) setResults(null)
        }
      }, 300)
    }
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Tab' && suggestions.length > 0 && mode === 'search') {
      e.preventDefault()
      const idx = selectedSuggestion >= 0 ? selectedSuggestion : 0
      if (suggestions[idx]) {
        setQuery(suggestions[idx])
        setShowSuggestions(false)
        doLiveSearch(suggestions[idx])
      }
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelectedSuggestion(prev => Math.min(prev + 1, suggestions.length - 1))
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelectedSuggestion(prev => Math.max(prev - 1, -1))
      return
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      setShowSuggestions(false)
      if (mode === 'search') {
        if (selectedSuggestion >= 0 && suggestions[selectedSuggestion]) {
          setQuery(suggestions[selectedSuggestion])
          doLiveSearch(suggestions[selectedSuggestion])
        } else {
          doLiveSearch(query)
        }
      } else {
        sendChat(query)
      }
      return
    }
    if (e.key === 'Escape') {
      setShowSuggestions(false)
    }
  }

  const selectSuggestion = (name) => {
    setQuery(name)
    setShowSuggestions(false)
    doLiveSearch(name)
  }

  const openPreview = async (f) => {
    setPreviewFile(f)
    setPreviewLoading(true)
    setPreviewData(null)
    if (f.drive_file_id && !f.drive_file_id.startsWith('local_') && !f.drive_file_id.startsWith('meta_')) {
      setPreviewData({
        type: 'drive',
        url: `https://drive.google.com/file/d/${f.drive_file_id}/preview`,
        driveUrl: `https://drive.google.com/file/d/${f.drive_file_id}/view`,
      })
      setPreviewLoading(false)
      return
    }
    try {
      const resp = await fetch(api.downloadUrl(f.id), {
        headers: { Authorization: `Bearer ${getToken()}` },
      })
      const ct = resp.headers.get('content-type') || ''
      if (ct.includes('json')) {
        const info = await resp.json()
        setPreviewData(
          info.preview_url
            ? { type: 'drive', url: info.preview_url, driveUrl: info.drive_url }
            : { type: 'not_available' }
        )
      } else if (ct.includes('image')) {
        setPreviewData({ type: 'image', url: URL.createObjectURL(await resp.blob()) })
      } else if (ct.includes('text')) {
        setPreviewData({ type: 'text', content: await resp.text() })
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
    fetch(api.downloadUrl(f.id), {
      headers: { Authorization: `Bearer ${getToken()}` },
    })
      .then(r => r.blob())
      .then(blob => {
        const u = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = u
        a.download = f.file_name
        a.click()
        URL.revokeObjectURL(u)
      })
  }

  const confirmDelete = (f) => setDeleteTarget(f)
  const doDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    await api.deleteFile(deleteTarget.id)
    notify.success(`Deleted ${deleteTarget.file_name}`)
    setDeleteTarget(null)
    setDeleting(false)
    doLiveSearch(query)
  }

  const samplePrompts = [
    'bhai harsh ne kya bheja hai?',
    'do we have any notes on linear queue?',
    'what travel tickets are booked for us?',
    'show me raspberry pi documents',
  ]

  const followUpSuggestions = [
    'tell me more',
    'in simpler words',
    'fetch it',
    'who sent this?',
    'hindi me samjhao',
  ]

  return (
    <div style={{ padding: '40px 48px 60px', maxWidth: 1100 }} className="fade-in">
      {/* ── Breadcrumb (identical to Files and Groups) ── */}
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
        <span style={{ color: '#111827', fontWeight: 600 }}>chat</span>
      </div>

      {/* ── Main Chat Workspace Card ── */}
      <div style={{
        background: '#ffffff',
        border: '1px solid #e5e7eb',
        borderRadius: 18,
        padding: '22px 26px',
        boxShadow: '0 4px 20px -2px rgba(0, 0, 0, 0.04), 0 1px 3px rgba(0, 0, 0, 0.02)',
        display: 'flex',
        flexDirection: 'column',
        height: 'calc(100vh - 175px)',
        minHeight: 580,
        boxSizing: 'border-box',
      }}>
        {/* Controls Bar */}
        <div style={{
          display: 'flex',
          flexWrap: 'wrap',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 14,
          marginBottom: 16,
          paddingBottom: 14,
          borderBottom: '1px solid #f3f4f6',
          flexShrink: 0,
        }}>
          {/* Left Title & Status Badge */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <h2 style={{ fontSize: 18, fontWeight: 700, color: '#111827', margin: 0, textTransform: 'lowercase', letterSpacing: '-0.01em' }}>
              chat
            </h2>
            <span style={{
              fontSize: 11,
              fontWeight: 600,
              color: '#15803d',
              background: '#dcfce7',
              padding: '2px 8px',
              borderRadius: 9999,
              display: 'inline-flex',
              alignItems: 'center',
              gap: 5,
            }}>
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: '#22c55e', display: 'inline-block' }} />
              gemini 2.5
            </span>
          </div>

          {/* Right Mode Switcher & Clear Button */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{
              display: 'flex',
              gap: 4,
              background: '#f9fafb',
              padding: 3,
              borderRadius: 10,
              border: '1px solid #f3f4f6',
            }}>
              {[
                { key: 'chat', label: 'assistant', icon: 'fa-comments' },
                { key: 'search', label: 'instant files', icon: icons.search },
              ].map(tab => {
                const isSelected = mode === tab.key
                return (
                  <button
                    key={tab.key}
                    onClick={() => { setMode(tab.key); setResults(null) }}
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
                      display: 'flex',
                      alignItems: 'center',
                      gap: 6,
                      boxShadow: isSelected ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
                      transition: 'all 0.15s ease',
                    }}
                  >
                    <Fa icon={tab.icon} style={{ fontSize: 11, color: isSelected ? '#15803d' : '#9ca3af' }} />
                    {tab.label}
                  </button>
                )
              })}
            </div>

            {mode === 'chat' && chatConversation.length > 0 && (
              <button
                onClick={clearChat}
                title="Start new conversation"
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 8,
                  padding: '5px 11px',
                  fontSize: 12,
                  fontWeight: 500,
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 5,
                  color: '#6b7280',
                  transition: 'all 0.15s',
                }}
                onMouseEnter={e => { e.currentTarget.style.color = '#111827'; e.currentTarget.style.borderColor = '#d1d5db' }}
                onMouseLeave={e => { e.currentTarget.style.color = '#6b7280'; e.currentTarget.style.borderColor = '#e5e7eb' }}
              >
                <Fa icon="fa-arrow-rotate-left" style={{ fontSize: 10 }} />
                clear
              </button>
            )}
          </div>
        </div>

      {/* ── CHAT MODE (Clean Conversation Canvas) ── */}
      {mode === 'chat' && (
        <div
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            minHeight: 0,
            position: 'relative',
          }}
        >
          {/* Scrollable Conversation Thread */}
          <div
            className="glass-scroll"
            style={{
              flex: 1,
              overflowY: 'auto',
              paddingRight: 6,
              display: 'flex',
              flexDirection: 'column',
              gap: 16,
              paddingBottom: 16,
            }}
          >
            {/* Minimal Clean Empty State */}
            {chatConversation.length === 0 ? (
              <div
                style={{
                  margin: 'auto 0',
                  padding: '24px 0',
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                  textAlign: 'center',
                }}
              >
                <div
                  style={{
                    width: 44,
                    height: 44,
                    borderRadius: 12,
                    background: '#f0fdf4',
                    border: '1px solid #dcfce7',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    marginBottom: 14,
                  }}
                >
                  <Fa icon="fa-sparkles" style={{ fontSize: 18, color: '#15803d' }} />
                </div>
                <div
                  style={{
                    fontSize: 16,
                    fontWeight: 700,
                    color: '#111827',
                    letterSpacing: '-0.01em',
                    marginBottom: 6,
                    textTransform: 'lowercase',
                  }}
                >
                  how can i help you find or understand your documents?
                </div>
                <div style={{ fontSize: 13, color: '#6b7280', marginBottom: 22, maxWidth: 460, lineHeight: 1.5, textTransform: 'lowercase' }}>
                  ask anything about what was shared in your chats in english, hindi, or hinglish.
                </div>

                {/* Clean Prompt Starter Pills */}
                <div
                  style={{
                    display: 'flex',
                    flexWrap: 'wrap',
                    gap: 8,
                    justifyContent: 'center',
                    maxWidth: 620,
                  }}
                >
                  {samplePrompts.map(p => (
                    <button
                      key={p}
                      onClick={() => sendChat(p)}
                      style={{
                        background: '#f9fafb',
                        border: '1px solid #e5e7eb',
                        borderRadius: 9999,
                        padding: '8px 15px',
                        fontSize: 12.5,
                        fontWeight: 500,
                        color: '#374151',
                        cursor: 'pointer',
                        display: 'flex',
                        alignItems: 'center',
                        gap: 8,
                        transition: 'all 0.15s ease',
                      }}
                      onMouseEnter={e => {
                        e.currentTarget.style.background = '#f0fdf4'
                        e.currentTarget.style.borderColor = '#86efac'
                        e.currentTarget.style.color = '#15803d'
                      }}
                      onMouseLeave={e => {
                        e.currentTarget.style.background = '#f9fafb'
                        e.currentTarget.style.borderColor = '#e5e7eb'
                        e.currentTarget.style.color = '#374151'
                      }}
                    >
                      <span>{p}</span>
                      <Fa icon="fa-arrow-right" style={{ fontSize: 9, opacity: 0.6 }} />
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              /* Conversation Messages */
              chatConversation.map(turn => (
                <div
                  key={turn.id}
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 12,
                    width: '100%',
                  }}
                >
                  {/* ── USER CHAT BUBBLE (RIGHT HAND SIDE) ── */}
                  <div
                    style={{
                      alignSelf: 'flex-end',
                      maxWidth: '75%',
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'flex-end',
                    }}
                  >
                    <div
                      style={{
                        background: '#f0fdf4',
                        border: '1px solid #dcfce7',
                        borderRadius: '16px 16px 4px 16px',
                        padding: '11px 16px',
                        fontSize: 14,
                        color: '#14532d',
                        lineHeight: 1.55,
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                      }}
                    >
                      {turn.question}
                    </div>
                    <span
                      style={{
                        fontSize: 10.5,
                        color: '#9ca3af',
                        marginTop: 4,
                        marginRight: 4,
                      }}
                    >
                      {formatClockTime(turn.ts)}
                    </span>
                  </div>

                  {/* ── REYNA REPLY BUBBLE (LEFT HAND SIDE) ── */}
                  <div
                    style={{
                      alignSelf: 'flex-start',
                      maxWidth: '82%',
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'flex-start',
                      width: '100%',
                    }}
                  >
                    <div
                      style={{
                        background: '#f9fafb',
                        border: '1px solid #e5e7eb',
                        borderRadius: '16px 16px 16px 4px',
                        padding: '15px 18px',
                        fontSize: 14,
                        color: '#1f2937',
                        lineHeight: 1.65,
                        wordBreak: 'break-word',
                        width: '100%',
                        boxSizing: 'border-box',
                      }}
                    >
                      {/* Sub-header */}
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 6,
                          marginBottom: 8,
                          fontSize: 11.5,
                          fontWeight: 600,
                          color: '#111827',
                          textTransform: 'lowercase',
                        }}
                      >
                        <span style={{ width: 6, height: 6, borderRadius: '50%', background: '#22c55e', display: 'inline-block' }} />
                        <span>reyna</span>
                        <span style={{ color: '#9ca3af', fontWeight: 400 }}>
                          · {formatClockTime(turn.ts)}
                        </span>
                      </div>

                      {/* Thinking State */}
                      {turn.pending ? (
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 8,
                            padding: '6px 0',
                            fontSize: 13,
                            color: 'var(--sub-color)',
                          }}
                        >
                          <Fa icon={icons.loading} spin style={{ fontSize: 12, color: 'var(--reyna-accent)' }} />
                          <span>reading documents…</span>
                        </div>
                      ) : (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                          {/* Notice banner if allowance reached */}
                          {turn.notice && (
                            <div
                              style={{
                                background: 'rgba(245, 158, 11, 0.1)',
                                border: '1px solid rgba(245, 158, 11, 0.25)',
                                borderRadius: 10,
                                padding: '8px 12px',
                                fontSize: 12.5,
                                color: '#b45309',
                                display: 'flex',
                                alignItems: 'center',
                                gap: 8,
                              }}
                            >
                              <Fa icon={icons.warning} style={{ fontSize: 12 }} />
                              <span>{turn.reply}</span>
                            </div>
                          )}

                          {/* Markdown formatted response */}
                          {!turn.notice && <div>{renderMarkdown(turn.reply)}</div>}

                          {/* Disambiguation: Which document did you mean? */}
                          {turn.candidates && turn.candidates.length > 0 && (
                            <div
                              style={{
                                marginTop: 6,
                                paddingTop: 10,
                                borderTop: '1px solid rgba(0,0,0,0.06)',
                              }}
                            >
                              <div style={{ fontSize: 12, fontWeight: 700, color: '#b45309', marginBottom: 8 }}>
                                which document did you mean?
                              </div>
                              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                                {turn.candidates.map(c => (
                                  <button
                                    key={c.file_id}
                                    onClick={() => handleChooseCandidate(c, turn.question)}
                                    style={{
                                      background: '#ffffff',
                                      border: '1px solid #e5e7eb',
                                      padding: '8px 12px',
                                      borderRadius: 10,
                                      fontSize: 12,
                                      textAlign: 'left',
                                      display: 'flex',
                                      alignItems: 'center',
                                      justifyContent: 'space-between',
                                      cursor: 'pointer',
                                      transition: 'all 0.15s ease',
                                    }}
                                    onMouseEnter={e => {
                                      e.currentTarget.style.borderColor = '#86efac'
                                      e.currentTarget.style.background = '#f0fdf4'
                                    }}
                                    onMouseLeave={e => {
                                      e.currentTarget.style.borderColor = '#e5e7eb'
                                      e.currentTarget.style.background = '#ffffff'
                                    }}
                                  >
                                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                      <Fa icon={icons.file} style={{ color: '#15803d', fontSize: 11 }} />
                                      <span style={{ fontWeight: 600, color: '#111827' }}>{c.file_name}</span>
                                      {c.folder && (
                                        <span style={{ fontSize: 10, padding: '1px 6px', borderRadius: 4, background: '#f3f4f6', color: '#6b7280' }}>
                                          {c.folder}
                                        </span>
                                      )}
                                    </div>
                                    <span style={{ fontSize: 11, color: '#6b7280' }}>
                                      {c.sender ? `by ${c.sender}` : ''}
                                    </span>
                                  </button>
                                ))}
                              </div>
                            </div>
                          )}

                          {/* Attached Document Capsules */}
                          {turn.files && turn.files.length > 0 && (
                            <div
                              style={{
                                display: 'flex',
                                flexDirection: 'column',
                                gap: 6,
                                marginTop: 6,
                              }}
                            >
                              {turn.files.map(f => (
                                <div
                                  key={f.id}
                                  style={{
                                    background: '#ffffff',
                                    border: '1px solid #e5e7eb',
                                    padding: '8px 12px',
                                    borderRadius: 12,
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'space-between',
                                    gap: 10,
                                  }}
                                >
                                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
                                    <IconBox
                                      icon={fileIconClass(f.mime_type, f.file_name)}
                                      color="#111827"
                                      bg="rgba(37,211,102,0.12)"
                                      size={30}
                                      iconSize={12}
                                    />
                                    <div style={{ minWidth: 0 }}>
                                      <div
                                        style={{
                                          fontWeight: 600,
                                          fontSize: 13,
                                          color: '#111827',
                                          whiteSpace: 'nowrap',
                                          overflow: 'hidden',
                                          textOverflow: 'ellipsis',
                                          maxWidth: 340,
                                        }}
                                        title={f.file_name}
                                      >
                                        {f.file_name}
                                      </div>
                                      <div style={{ fontSize: 11, color: '#6b7280' }}>
                                        {f.subject && <span style={{ fontWeight: 600, color: '#111827' }}>{f.subject} · </span>}
                                        {f.shared_by_name ? `by ${f.shared_by_name}` : 'shared in chat'}
                                        {f.posted_at || f.created_at ? ` · ${timeAgo(f.posted_at || f.created_at)}` : ''}
                                      </div>
                                    </div>
                                  </div>

                                  <div style={{ display: 'flex', gap: 4, flexShrink: 0 }}>
                                    <button
                                      onClick={() => openPreview(f)}
                                      title="Preview"
                                      style={{
                                        width: 28,
                                        height: 28,
                                        borderRadius: 8,
                                        border: '1px solid #e5e7eb',
                                        background: '#f9fafb',
                                        cursor: 'pointer',
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        padding: 0,
                                        color: '#374151',
                                      }}
                                    >
                                      <Fa icon={icons.preview} style={{ fontSize: 11 }} />
                                    </button>
                                    <button
                                      onClick={() => downloadFile(f)}
                                      title="Download"
                                      style={{
                                        width: 28,
                                        height: 28,
                                        borderRadius: 8,
                                        border: '1px solid #e5e7eb',
                                        background: '#f9fafb',
                                        cursor: 'pointer',
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        padding: 0,
                                        color: '#374151',
                                      }}
                                    >
                                      <Fa icon={icons.download} style={{ fontSize: 11 }} />
                                    </button>
                                    {f.drive_file_id &&
                                      !f.drive_file_id.startsWith('local_') &&
                                      !f.drive_file_id.startsWith('meta_') && (
                                        <a
                                          href={`https://drive.google.com/file/d/${f.drive_file_id}/view`}
                                          target="_blank"
                                          rel="noreferrer"
                                          title="Open in Drive"
                                          style={{
                                            width: 28,
                                            height: 28,
                                            borderRadius: 8,
                                            border: '1px solid #e5e7eb',
                                            background: '#f9fafb',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            color: '#374151',
                                            textDecoration: 'none',
                                            padding: 0,
                                          }}
                                        >
                                          <Fa icon={icons.link} style={{ fontSize: 10 }} />
                                        </a>
                                      )}
                                  </div>
                                </div>
                              ))}
                            </div>
                          )}

                          {/* Verified Citations Accordion */}
                          {turn.citations && turn.citations.length > 0 && (
                            <div>
                              <button
                                onClick={() => toggleSources(turn.id)}
                                className="glass-pill"
                                style={{
                                  padding: '4px 10px',
                                  borderRadius: 9999,
                                  fontSize: 11,
                                  fontWeight: 600,
                                  color: 'var(--sub-color)',
                                  cursor: 'pointer',
                                  display: 'inline-flex',
                                  alignItems: 'center',
                                  gap: 6,
                                }}
                              >
                                <Fa icon={icons.file} style={{ fontSize: 9, color: 'var(--reyna-accent)' }} />
                                <span>
                                  {turn.citations.length} verified passage{turn.citations.length > 1 ? 's' : ''}
                                </span>
                                <Fa icon={turn.showSources ? 'fa-chevron-up' : 'fa-chevron-down'} style={{ fontSize: 8 }} />
                              </button>

                              {turn.showSources && (
                                <div
                                  className="glass-card"
                                  style={{
                                    marginTop: 8,
                                    padding: 10,
                                    borderRadius: 12,
                                    display: 'flex',
                                    flexDirection: 'column',
                                    gap: 6,
                                  }}
                                >
                                  {turn.citations.map((c, ci) => (
                                    <div
                                      key={ci}
                                      style={{
                                        padding: '8px 10px',
                                        background: 'rgba(255,255,255,0.7)',
                                        borderRadius: 8,
                                        borderLeft: '3px solid var(--reyna-accent)',
                                        fontSize: 12,
                                      }}
                                    >
                                      <div style={{ fontStyle: 'italic', marginBottom: 4, lineHeight: 1.5 }}>
                                        "{c.quote}"
                                      </div>
                                      <div style={{ fontSize: 10, color: 'var(--sub-color)', display: 'flex', gap: 8 }}>
                                        <span>
                                          <strong>{c.file_name}</strong> (Page {c.page || 1})
                                        </span>
                                        {c.sender && <span>by {c.sender}</span>}
                                      </div>
                                    </div>
                                  ))}
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              ))
            )}
            <div ref={chatEndRef} style={{ height: 8 }} />
          </div>

          {/* ── Follow-Up Suggestion Pills ── */}
          {chatConversation.length > 0 &&
            !chatConversation[chatConversation.length - 1].pending && (
              <div
                style={{
                  display: 'flex',
                  gap: 6,
                  flexWrap: 'wrap',
                  marginBottom: 10,
                  paddingLeft: 2,
                }}
              >
                {followUpSuggestions.map(s => (
                  <button
                    key={s}
                    onClick={() => sendChat(s)}
                    style={{
                      background: '#ffffff',
                      border: '1px solid #e5e7eb',
                      borderRadius: 9999,
                      padding: '4px 11px',
                      fontSize: 11.5,
                      fontWeight: 500,
                      cursor: 'pointer',
                      color: '#4b5563',
                      transition: 'all 0.15s ease',
                    }}
                    onMouseEnter={e => {
                      e.currentTarget.style.borderColor = '#86efac'
                      e.currentTarget.style.color = '#15803d'
                      e.currentTarget.style.background = '#f0fdf4'
                    }}
                    onMouseLeave={e => {
                      e.currentTarget.style.borderColor = '#e5e7eb'
                      e.currentTarget.style.color = '#4b5563'
                      e.currentTarget.style.background = '#ffffff'
                    }}
                  >
                    {s}
                  </button>
                ))}
              </div>
            )}

          {/* ── Bottom Integrated Composer ── */}
          <div
            style={{
              borderTop: '1px solid #f3f4f6',
              paddingTop: 12,
              marginTop: 'auto',
              flexShrink: 0,
            }}
          >
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                background: '#f9fafb',
                border: '1px solid #e5e7eb',
                borderRadius: 14,
                padding: '7px 10px 7px 14px',
                transition: 'border-color 0.15s ease, box-shadow 0.15s ease, background 0.15s ease',
              }}
              onFocusCapture={e => {
                e.currentTarget.style.borderColor = '#22c55e'
                e.currentTarget.style.boxShadow = '0 0 0 3px rgba(34, 197, 94, 0.12)'
                e.currentTarget.style.background = '#ffffff'
              }}
              onBlurCapture={e => {
                e.currentTarget.style.borderColor = '#e5e7eb'
                e.currentTarget.style.boxShadow = 'none'
                e.currentTarget.style.background = '#f9fafb'
              }}
            >
              <textarea
                ref={inputRef}
                value={query}
                onChange={handleChange}
                onKeyDown={handleKeyDown}
                onInput={e => {
                  e.target.style.height = 'auto'
                  e.target.style.height = Math.min(e.target.scrollHeight, 120) + 'px'
                }}
                rows={1}
                placeholder="ask reyna in english, hindi, or hinglish..."
                style={{
                  flex: 1,
                  minWidth: 0,
                  padding: '6px 0',
                  fontSize: 14,
                  background: 'transparent',
                  border: 'none',
                  color: '#111827',
                  outline: 'none',
                  resize: 'none',
                  overflowY: 'hidden',
                  minHeight: 22,
                  maxHeight: 120,
                  fontFamily: 'inherit',
                  lineHeight: 1.5,
                }}
              />

              <button
                onClick={() => sendChat(query)}
                disabled={!query.trim() || loading}
                title="Send message"
                style={{
                  width: 34,
                  height: 34,
                  borderRadius: 10,
                  border: 'none',
                  background: query.trim() ? '#0d3824' : '#e5e7eb',
                  color: query.trim() ? '#ffffff' : '#9ca3af',
                  cursor: query.trim() && !loading ? 'pointer' : 'default',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                  transition: 'all 0.15s ease',
                }}
              >
                <Fa icon="fa-paper-plane" style={{ fontSize: 12 }} />
              </button>
            </div>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                padding: '5px 4px 0',
                fontSize: 11,
                color: '#9ca3af',
              }}
            >
              <span>enter to send · shift + enter for new line</span>
              <span>supports english, hindi, and hinglish</span>
            </div>
          </div>
        </div>
      )}

      {/* ── FILES SEARCH MODE (Instant File Keyword Table) ── */}
      {mode === 'search' && (
        <div
          className="glass-scroll"
          style={{
            flex: 1,
            overflowY: 'auto',
            paddingRight: 6,
          }}
        >
          {/* Search Bar Input */}
          <div style={{ position: 'relative', marginBottom: 16 }}>
            <Fa
              icon={icons.search}
              style={{
                position: 'absolute',
                left: 14,
                top: 13,
                fontSize: 13,
                color: '#9ca3af',
                pointerEvents: 'none',
              }}
            />
            <input
              value={query}
              onChange={handleChange}
              onKeyDown={handleKeyDown}
              onFocus={() => suggestions.length > 0 && setShowSuggestions(true)}
              onBlur={() => setTimeout(() => setShowSuggestions(false), 200)}
              placeholder="search files by name or subject tag..."
              style={{
                width: '100%',
                padding: '10px 14px 10px 38px',
                borderRadius: 12,
                border: '1px solid #e5e7eb',
                background: '#f9fafb',
                fontSize: 13.5,
                color: '#111827',
                outline: 'none',
                fontFamily: 'inherit',
                boxSizing: 'border-box',
                transition: 'border-color 0.15s ease',
              }}
              onFocusCapture={e => e.currentTarget.style.borderColor = '#22c55e'}
              onBlurCapture={e => e.currentTarget.style.borderColor = '#e5e7eb'}
            />

            {showSuggestions && suggestions.length > 0 && (
              <div
                style={{
                  position: 'absolute',
                  top: '100%',
                  left: 0,
                  right: 0,
                  marginTop: 6,
                  borderRadius: 12,
                  zIndex: 50,
                  overflow: 'hidden',
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  boxShadow: '0 8px 24px rgba(0,0,0,0.06)',
                }}
              >
                {suggestions.slice(0, 6).map((name, i) => (
                  <div
                    key={i}
                    onClick={() => selectSuggestion(name)}
                    style={{
                      padding: '8px 14px',
                      cursor: 'pointer',
                      fontSize: 12,
                      color: '#111827',
                      background: i === selectedSuggestion ? '#f0fdf4' : 'transparent',
                      borderBottom: i < suggestions.length - 1 ? '1px solid #f3f4f6' : 'none',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                    }}
                  >
                    <Fa icon={icons.file} style={{ fontSize: 12, color: '#9ca3af' }} />
                    <span style={{ fontWeight: 500 }}>{name}</span>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Results or Empty */}
          {results === null ? (
            <div style={{ textAlign: 'center', padding: '40px 20px', color: '#6b7280' }}>
              <Fa icon={icons.search} style={{ fontSize: 32, opacity: 0.3, marginBottom: 12 }} />
              <p style={{ fontSize: 14, margin: '0 0 12px', fontWeight: 600, color: '#111827', textTransform: 'lowercase' }}>
                instant keyword search across your documents
              </p>
              <div style={{ display: 'flex', gap: 6, justifyContent: 'center', flexWrap: 'wrap' }}>
                {['linear queue', 'notes', 'assignment', 'bayes', 'lab'].map(q => (
                  <button
                    key={q}
                    onClick={() => {
                      setQuery(q)
                      doLiveSearch(q)
                    }}
                    style={{
                      padding: '5px 13px',
                      borderRadius: 9999,
                      border: '1px solid #e5e7eb',
                      background: '#f9fafb',
                      fontSize: 12,
                      fontWeight: 500,
                      color: '#374151',
                      cursor: 'pointer',
                      transition: 'all 0.15s ease',
                    }}
                    onMouseEnter={e => {
                      e.currentTarget.style.borderColor = '#86efac'
                      e.currentTarget.style.color = '#15803d'
                      e.currentTarget.style.background = '#f0fdf4'
                    }}
                    onMouseLeave={e => {
                      e.currentTarget.style.borderColor = '#e5e7eb'
                      e.currentTarget.style.color = '#374151'
                      e.currentTarget.style.background = '#f9fafb'
                    }}
                  >
                    {q}
                  </button>
                ))}
              </div>
            </div>
          ) : results.length === 0 ? (
            <div style={{ textAlign: 'center', padding: '40px 20px', color: '#6b7280' }}>
              <Fa icon="fa-inbox" style={{ fontSize: 32, opacity: 0.3, marginBottom: 8 }} />
              <p style={{ fontSize: 13 }}>No files found for "{query}"</p>
            </div>
          ) : (
            <div>
              <p style={{ fontSize: 12, color: '#6b7280', marginBottom: 10 }}>
                {results.length} result{results.length !== 1 ? 's' : ''} for "{query}"
              </p>
              <div style={{ borderRadius: 12, overflow: 'hidden', border: '1px solid #e5e7eb', background: '#ffffff' }}>
                {results.map((f, i) => (
                  <div
                    key={f.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 12,
                      padding: '10px 14px',
                      borderBottom: i < results.length - 1 ? '1px solid #f3f4f6' : 'none',
                      transition: 'background 0.15s',
                    }}
                  >
                    <IconBox
                      icon={fileIconClass(f.mime_type, f.file_name)}
                      color="#111827"
                      bg="rgba(37,211,102,0.12)"
                      size={32}
                      iconSize={13}
                    />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontSize: 13, fontWeight: 600, color: '#111827' }}>
                        {f.file_name}
                      </div>
                      <div style={{ fontSize: 11, color: '#6b7280' }}>
                        {f.subject && <span style={{ fontWeight: 600, color: '#111827' }}>{f.subject} · </span>}
                        {formatBytes(f.file_size)} · by {f.shared_by_name || 'unknown'} · {timeAgo(f.posted_at || f.created_at)}
                      </div>
                    </div>

                    <div style={{ display: 'flex', gap: 6 }}>
                      <button
                        onClick={() => openPreview(f)}
                        title="Preview"
                        style={{
                          width: 28,
                          height: 28,
                          borderRadius: 8,
                          border: '1px solid #e5e7eb',
                          background: '#f9fafb',
                          cursor: 'pointer',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          padding: 0,
                          color: '#374151',
                        }}
                      >
                        <Fa icon={icons.preview} style={{ fontSize: 11 }} />
                      </button>
                      <button
                        onClick={() => downloadFile(f)}
                        title="Download"
                        style={{
                          width: 28,
                          height: 28,
                          borderRadius: 8,
                          border: '1px solid #e5e7eb',
                          background: '#f9fafb',
                          cursor: 'pointer',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          padding: 0,
                          color: '#374151',
                        }}
                      >
                        <Fa icon={icons.download} style={{ fontSize: 11 }} />
                      </button>
                      <button
                        onClick={() => confirmDelete(f)}
                        title="Delete"
                        style={{
                          width: 28,
                          height: 28,
                          borderRadius: 8,
                          border: '1px solid #fecaca',
                          background: '#fef2f2',
                          color: '#dc2626',
                          cursor: 'pointer',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          padding: 0,
                        }}
                      >
                        <Fa icon={icons.delete} style={{ fontSize: 11 }} />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      </div>

      {/* ── Document Preview Modal ── */}
      {previewFile &&
        createPortal(
          <div
            style={{
              position: 'fixed',
              top: 0,
              left: 0,
              width: '100vw',
              height: '100vh',
              background: 'rgba(0,0,0,0.75)',
              backdropFilter: 'blur(10px)',
              display: 'flex',
              flexDirection: 'column',
              zIndex: 99999,
            }}
          >
            <div
              style={{
                padding: '12px 24px',
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                background: 'rgba(255,255,255,0.9)',
                backdropFilter: 'blur(20px)',
                borderBottom: '1px solid rgba(0,0,0,0.08)',
                flexShrink: 0,
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <Fa
                  icon={fileIconClass(previewFile.mime_type, previewFile.file_name)}
                  style={{ fontSize: 20, color: 'var(--reyna-accent)' }}
                />
                <div>
                  <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--text-color)' }}>
                    {previewFile.file_name}
                  </div>
                  <div style={{ fontSize: 12, color: 'var(--sub-color)' }}>
                    {formatBytes(previewFile.file_size)} · by {previewFile.shared_by_name || 'unknown'}
                  </div>
                </div>
              </div>
              <div style={{ display: 'flex', gap: 8 }}>
                {previewData?.driveUrl && (
                  <a
                    href={previewData.driveUrl}
                    target="_blank"
                    rel="noreferrer"
                    style={{
                      padding: '6px 12px',
                      borderRadius: 8,
                      border: '1px solid #ccc',
                      background: '#fff',
                      color: 'var(--text-color)',
                      fontSize: 12,
                      fontWeight: 600,
                      textDecoration: 'none',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 6,
                    }}
                  >
                    <Fa icon={icons.link} style={{ fontSize: 10 }} /> Drive
                  </a>
                )}
                <button
                  onClick={() => downloadFile(previewFile)}
                  style={{
                    padding: '6px 12px',
                    borderRadius: 8,
                    border: '1px solid #ccc',
                    background: '#fff',
                    color: 'var(--text-color)',
                    fontSize: 12,
                    fontWeight: 600,
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                  }}
                >
                  <Fa icon={icons.download} style={{ fontSize: 10 }} /> Download
                </button>
                <button
                  onClick={() => {
                    setPreviewFile(null)
                    setPreviewData(null)
                  }}
                  style={{
                    width: 32,
                    height: 32,
                    borderRadius: '50%',
                    border: 'none',
                    background: 'rgba(0,0,0,0.06)',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <Fa icon={icons.close} style={{ fontSize: 12 }} />
                </button>
              </div>
            </div>

            <div
              style={{
                flex: 1,
                overflow: 'auto',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: 24,
              }}
            >
              {previewLoading ? (
                <div style={{ color: '#fff', display: 'flex', alignItems: 'center', gap: 8 }}>
                  <Fa icon={icons.loading} spin /> loading preview...
                </div>
              ) : previewData?.type === 'drive' ? (
                <iframe
                  src={previewData.url}
                  style={{ width: '90%', height: '90%', border: 'none', borderRadius: 12 }}
                  title="Preview"
                />
              ) : previewData?.type === 'image' ? (
                <img
                  src={previewData.url}
                  alt=""
                  style={{
                    maxWidth: '90%',
                    maxHeight: '90%',
                    borderRadius: 12,
                    objectFit: 'contain',
                    boxShadow: '0 20px 50px rgba(0,0,0,0.5)',
                  }}
                />
              ) : previewData?.type === 'text' ? (
                <div
                  style={{
                    width: '80%',
                    height: '80%',
                    overflow: 'auto',
                    padding: 24,
                    background: '#fff',
                    borderRadius: 12,
                  }}
                >
                  <pre
                    style={{
                      fontFamily: 'var(--font-mono)',
                      fontSize: 12,
                      lineHeight: 1.7,
                      whiteSpace: 'pre-wrap',
                      margin: 0,
                    }}
                  >
                    {previewData.content}
                  </pre>
                </div>
              ) : (
                <div style={{ color: '#fff', textAlign: 'center' }}>
                  <Fa icon={icons.file} style={{ fontSize: 48, marginBottom: 12, opacity: 0.6 }} />
                  <p>No preview available for this file type</p>
                  <button
                    onClick={() => downloadFile(previewFile)}
                    style={{
                      padding: '8px 16px',
                      borderRadius: 8,
                      border: 'none',
                      background: '#25D366',
                      color: '#fff',
                      fontWeight: 700,
                      cursor: 'pointer',
                      marginTop: 12,
                    }}
                  >
                    Download File
                  </button>
                </div>
              )}
            </div>
          </div>,
          document.body
        )}

      {/* ── Delete Confirmation Modal ── */}
      {deleteTarget &&
        createPortal(
          <div
            onClick={() => setDeleteTarget(null)}
            style={{
              position: 'fixed',
              top: 0,
              left: 0,
              width: '100vw',
              height: '100vh',
              background: 'rgba(0,0,0,0.4)',
              backdropFilter: 'blur(6px)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              zIndex: 99999,
            }}
          >
            <div
              onClick={e => e.stopPropagation()}
              className="glass-card"
              style={{
                borderRadius: 20,
                padding: 28,
                maxWidth: 400,
                width: '90vw',
                background: 'rgba(255,255,255,0.95)',
                boxShadow: '0 25px 60px rgba(0,0,0,0.2)',
              }}
            >
              <h3
                style={{
                  fontSize: 18,
                  fontWeight: 800,
                  color: 'var(--error-color)',
                  marginBottom: 10,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                }}
              >
                <Fa icon={icons.warning} style={{ fontSize: 14 }} /> Delete File?
              </h3>
              <p
                style={{
                  fontSize: 13,
                  color: 'var(--text-secondary, #555)',
                  lineHeight: 1.5,
                  marginBottom: 20,
                }}
              >
                Delete <strong>{deleteTarget.file_name}</strong>? It will be moved to your Drive trash.
              </p>
              <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
                <button
                  onClick={() => setDeleteTarget(null)}
                  style={{
                    padding: '8px 16px',
                    borderRadius: 10,
                    border: '1px solid #ccc',
                    background: '#fff',
                    cursor: 'pointer',
                    fontSize: 13,
                    fontWeight: 600,
                  }}
                >
                  Cancel
                </button>
                <button
                  onClick={doDelete}
                  disabled={deleting}
                  style={{
                    padding: '8px 16px',
                    borderRadius: 10,
                    border: 'none',
                    background: 'var(--error-color)',
                    color: '#fff',
                    cursor: 'pointer',
                    fontSize: 13,
                    fontWeight: 700,
                    opacity: deleting ? 0.6 : 1,
                  }}
                >
                  {deleting ? 'Deleting...' : 'Delete'}
                </button>
              </div>
            </div>
          </div>,
          document.body
        )}
    </div>
  )
}
