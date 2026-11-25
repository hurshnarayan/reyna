import { useState, useEffect, useRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { api, getToken, getUser } from '../lib/api'
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
  const currentUser = getUser()
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

  // Conversational Chat & Retrieval Thread
  const [chatConversation, setChatConversation] = useState([])
  const [llmStatus, setLlmStatus] = useState(null)
  const [showPromptsMenu, setShowPromptsMenu] = useState(false)

  const chatEndRef = useRef(null)
  const inputRef = useRef(null)
  const debounceRef = useRef(null)

  useEffect(() => {
    if (chatEndRef.current) {
      chatEndRef.current.scrollIntoView({ behavior: 'smooth', block: 'end' })
    }
  }, [chatConversation])

  useEffect(() => {
    api.llmStatus().then(s => setLlmStatus(s))
  }, [])

  // Live Instant Search (Mode: search)
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

  // Send Conversational Chat Query (Mode: chat)
  const sendChat = async (promptText, fileIds = []) => {
    const q = promptText.trim()
    if (!q) return
    setLoading(true)
    notify.showLoader()
    setShowPromptsMenu(false)

    const turnId = Date.now()
    const userTurn = {
      id: turnId,
      question: q,
      reply: '',
      files: [],
      citations: [],
      candidates: [],
      notice: '',
      parsed_query: null,
      status: 'answered',
      pending: true,
      showSources: false,
      ts: turnId,
    }

    setChatConversation(prev => [...prev, userTurn])
    setQuery('')

    // Reset textarea height
    if (inputRef.current) {
      inputRef.current.style.height = 'auto'
    }

    // Build chat history for multi-turn threading
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
            parsed_query: resp?.parsed_query || null,
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
            reply: 'Sorry, I encountered an issue processing that query. Please try again.',
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
      setShowPromptsMenu(false)
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
    'show me raspberry pi documents',
    'what travel tickets are booked for us?',
  ]

  const followUpSuggestions = [
    'tell me more',
    'in simpler words',
    'fetch it',
    'who sent this?',
    'hindi me samjhao',
  ]

  const userDisplayName = currentUser?.name || 'You'
  const userInitials = userDisplayName.slice(0, 2).toUpperCase()

  return (
    <div
      style={{
        height: 'calc(100vh - 36px)',
        margin: '18px 24px',
        display: 'flex',
        flexDirection: 'column',
      }}
      className="fade-in"
    >
      {/* ── Main Glassmorphic Panel (Matching Screenshot) ── */}
      <div
        className="glass-container"
        style={{
          flex: 1,
          borderRadius: 24,
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
          position: 'relative',
        }}
      >
        {/* ── Top Bar ── */}
        <div
          style={{
            height: 64,
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid rgba(255, 255, 255, 0.4)',
            background: 'rgba(255, 255, 255, 0.25)',
            backdropFilter: 'blur(10px)',
            flexShrink: 0,
            zIndex: 10,
          }}
        >
          {/* Left: Avatar & Identity */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{ position: 'relative' }}>
              <img
                src="/reyna-badge.png"
                alt="Reyna"
                style={{
                  width: 38,
                  height: 38,
                  borderRadius: '50%',
                  objectFit: 'cover',
                  border: '2px solid rgba(255, 255, 255, 0.8)',
                  boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                }}
              />
              <span
                style={{
                  position: 'absolute',
                  bottom: -1,
                  right: -1,
                  width: 10,
                  height: 10,
                  borderRadius: '50%',
                  background: '#25D366',
                  border: '2px solid #fff',
                  boxShadow: '0 0 6px rgba(37,211,102,0.6)',
                }}
              />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <span style={{ fontSize: 16, fontWeight: 800, letterSpacing: -0.3, color: 'var(--text-color)' }}>
                  Reyna
                </span>
                <span
                  style={{
                    fontSize: 9,
                    fontWeight: 700,
                    textTransform: 'uppercase',
                    letterSpacing: 0.5,
                    padding: '1px 6px',
                    borderRadius: 10,
                    background: 'rgba(37,211,102,0.15)',
                    color: '#0e7a3d',
                  }}
                >
                  AI
                </span>
              </div>
              <div style={{ fontSize: 11, color: 'var(--sub-color)', fontWeight: 500 }}>
                {llmStatus?.enabled ? 'connected · multilingual assistant' : 'online · document capture'}
              </div>
            </div>
          </div>

          {/* Right: Mode Switch & Actions */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <div
              style={{
                display: 'flex',
                background: 'rgba(0, 0, 0, 0.05)',
                borderRadius: 9999,
                padding: 3,
                border: '1px solid rgba(255, 255, 255, 0.4)',
              }}
            >
              <button
                onClick={() => setMode('chat')}
                style={{
                  padding: '6px 14px',
                  borderRadius: 9999,
                  border: 'none',
                  fontSize: 12,
                  fontWeight: 600,
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  background: mode === 'chat' ? '#fff' : 'transparent',
                  color: mode === 'chat' ? 'var(--text-color)' : 'var(--sub-color)',
                  boxShadow: mode === 'chat' ? '0 2px 6px rgba(0,0,0,0.08)' : 'none',
                  transition: 'all 0.15s ease',
                }}
              >
                <Fa icon="fa-comments" style={{ fontSize: 11, color: mode === 'chat' ? 'var(--reyna-accent)' : 'inherit' }} />
                <span>Chat</span>
              </button>
              <button
                onClick={() => setMode('search')}
                style={{
                  padding: '6px 14px',
                  borderRadius: 9999,
                  border: 'none',
                  fontSize: 12,
                  fontWeight: 600,
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  background: mode === 'search' ? '#fff' : 'transparent',
                  color: mode === 'search' ? 'var(--text-color)' : 'var(--sub-color)',
                  boxShadow: mode === 'search' ? '0 2px 6px rgba(0,0,0,0.08)' : 'none',
                  transition: 'all 0.15s ease',
                }}
              >
                <Fa icon={icons.search} style={{ fontSize: 11 }} />
                <span>Files</span>
              </button>
            </div>

            {mode === 'chat' && chatConversation.length > 0 && (
              <button
                onClick={clearChat}
                title="New conversation"
                style={{
                  width: 32,
                  height: 32,
                  borderRadius: '50%',
                  border: '1px solid rgba(255,255,255,0.6)',
                  background: 'rgba(255,255,255,0.6)',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: 'var(--sub-color)',
                  transition: 'all 0.15s ease',
                }}
                onMouseEnter={e => {
                  e.currentTarget.style.background = '#fff'
                  e.currentTarget.style.color = 'var(--text-color)'
                }}
                onMouseLeave={e => {
                  e.currentTarget.style.background = 'rgba(255,255,255,0.6)'
                  e.currentTarget.style.color = 'var(--sub-color)'
                }}
              >
                <Fa icon="fa-arrow-rotate-left" style={{ fontSize: 11 }} />
              </button>
            )}
          </div>
        </div>

        {/* ── Chat Stream (Mode: chat) ── */}
        {mode === 'chat' && (
          <div
            className="glass-scroll"
            style={{
              flex: 1,
              overflowY: 'auto',
              padding: '24px 32px 12px',
              display: 'flex',
              flexDirection: 'column',
              justifyContent: chatConversation.length === 0 ? 'center' : 'flex-start',
            }}
          >
            {/* Empty Hero State (Matching screenshot with two central avatars) */}
            {chatConversation.length === 0 ? (
              <div
                style={{
                  textAlign: 'center',
                  maxWidth: 520,
                  margin: '0 auto',
                  padding: '32px 20px',
                }}
              >
                {/* Two Floating Avatars connected by dots */}
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: 16,
                    marginBottom: 20,
                  }}
                >
                  {/* User Avatar Circle */}
                  <div
                    style={{
                      width: 58,
                      height: 58,
                      borderRadius: '50%',
                      background: 'linear-gradient(135deg, #f59e0b, #d97706)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      color: '#fff',
                      fontSize: 20,
                      fontWeight: 800,
                      border: '3px solid rgba(255,255,255,0.9)',
                      boxShadow: '0 8px 24px rgba(217,119,6,0.25)',
                    }}
                  >
                    {userInitials}
                  </div>

                  {/* Connecting Dotted Line */}
                  <div
                    style={{
                      display: 'flex',
                      gap: 4,
                      color: 'rgba(0,0,0,0.25)',
                      fontSize: 14,
                    }}
                  >
                    <span>·</span>
                    <span>·</span>
                    <span>·</span>
                  </div>

                  {/* Reyna Avatar Circle */}
                  <div
                    style={{
                      width: 58,
                      height: 58,
                      borderRadius: '50%',
                      padding: 2,
                      background: 'linear-gradient(135deg, #25D366, #10b981)',
                      border: '3px solid rgba(255,255,255,0.9)',
                      boxShadow: '0 8px 24px rgba(37,211,102,0.3)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    <img
                      src="/reyna-badge.png"
                      alt="Reyna"
                      style={{
                        width: '100%',
                        height: '100%',
                        borderRadius: '50%',
                        objectFit: 'cover',
                      }}
                    />
                  </div>
                </div>

                <h2 style={{ fontSize: 24, fontWeight: 800, letterSpacing: -0.5, margin: '0 0 6px', color: 'var(--text-color)' }}>
                  Reyna
                </h2>
                <div style={{ fontSize: 13, color: '#25D366', fontWeight: 600, marginBottom: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6 }}>
                  <span style={{ width: 6, height: 6, borderRadius: '50%', background: '#25D366' }} />
                  Multilingual Document Intelligence
                </div>
                <p style={{ fontSize: 14, color: 'var(--sub-color)', margin: '0 auto 28px', lineHeight: 1.5, maxWidth: 420 }}>
                  Ask questions, retrieve shared files, or follow up in English, Hindi, or Hinglish.
                </p>

                {/* Glassy Quick Prompt Capsules */}
                <div
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 8,
                    textAlign: 'left',
                  }}
                >
                  {samplePrompts.map(p => (
                    <button
                      key={p}
                      onClick={() => sendChat(p)}
                      className="glass-pill"
                      style={{
                        padding: '12px 18px',
                        borderRadius: 14,
                        fontSize: 13,
                        fontWeight: 500,
                        cursor: 'pointer',
                        color: 'var(--text-color)',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                      }}
                    >
                      <span>{p}</span>
                      <Fa icon="fa-arrow-right" style={{ fontSize: 11, color: 'var(--reyna-accent)' }} />
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              /* Message Stream */
              <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
                {chatConversation.map(turn => (
                  <div key={turn.id} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                    {/* User Message Row */}
                    <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start' }}>
                      <div
                        style={{
                          width: 38,
                          height: 38,
                          borderRadius: '50%',
                          background: 'linear-gradient(135deg, #f59e0b, #d97706)',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          color: '#fff',
                          fontSize: 14,
                          fontWeight: 700,
                          flexShrink: 0,
                          marginTop: 2,
                          boxShadow: '0 2px 6px rgba(0,0,0,0.1)',
                        }}
                      >
                        {userInitials}
                      </div>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, marginBottom: 4 }}>
                          <span style={{ fontSize: 14, fontWeight: 700, color: 'var(--text-color)' }}>
                            {userDisplayName}
                          </span>
                          <span style={{ fontSize: 11, color: 'var(--sub-color)' }}>
                            Today at {formatClockTime(turn.ts)}
                          </span>
                        </div>
                        <div
                          style={{
                            fontSize: 14.5,
                            color: 'var(--text-color)',
                            lineHeight: 1.6,
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-word',
                          }}
                        >
                          {turn.question}
                        </div>
                      </div>
                    </div>

                    {/* Reyna Response Row */}
                    <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start' }}>
                      <div style={{ position: 'relative', flexShrink: 0, marginTop: 2 }}>
                        <img
                          src="/reyna-badge.png"
                          alt="Reyna"
                          style={{
                            width: 38,
                            height: 38,
                            borderRadius: '50%',
                            objectFit: 'cover',
                            border: '1.5px solid rgba(255,255,255,0.8)',
                            boxShadow: '0 2px 8px rgba(37,211,102,0.2)',
                          }}
                        />
                      </div>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, marginBottom: 4 }}>
                          <span style={{ fontSize: 14, fontWeight: 700, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 6 }}>
                            Reyna
                            <span
                              style={{
                                fontSize: 9,
                                fontWeight: 700,
                                textTransform: 'uppercase',
                                padding: '1px 5px',
                                borderRadius: 8,
                                background: 'rgba(37,211,102,0.15)',
                                color: '#0e7a3d',
                              }}
                            >
                              AI
                            </span>
                          </span>
                          <span style={{ fontSize: 11, color: 'var(--sub-color)' }}>
                            Today at {formatClockTime(turn.ts)}
                          </span>
                        </div>

                        {turn.pending ? (
                          <div
                            style={{
                              display: 'inline-flex',
                              alignItems: 'center',
                              gap: 8,
                              padding: '8px 14px',
                              borderRadius: 14,
                              background: 'rgba(255,255,255,0.6)',
                              border: '1px solid rgba(255,255,255,0.7)',
                              fontSize: 13,
                              color: 'var(--sub-color)',
                              marginTop: 4,
                            }}
                          >
                            <Fa icon={icons.loading} spin style={{ fontSize: 12, color: 'var(--reyna-accent)' }} />
                            <span>searching documents & drafting answer…</span>
                          </div>
                        ) : (
                          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                            {/* Markdown text reply */}
                            <div
                              style={{
                                fontSize: 14.5,
                                color: 'var(--text-color)',
                                lineHeight: 1.65,
                                wordBreak: 'break-word',
                              }}
                            >
                              {renderMarkdown(turn.reply)}
                            </div>

                            {/* Disambiguation Candidate Choices */}
                            {turn.candidates && turn.candidates.length > 0 && (
                              <div
                                className="glass-card"
                                style={{
                                  padding: '12px 16px',
                                  borderRadius: 14,
                                  marginTop: 4,
                                }}
                              >
                                <div style={{ fontSize: 12, fontWeight: 700, color: '#b45309', marginBottom: 8 }}>
                                  Which document did you mean?
                                </div>
                                <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                                  {turn.candidates.map(c => (
                                    <button
                                      key={c.file_id}
                                      onClick={() => handleChooseCandidate(c, turn.question)}
                                      className="glass-pill"
                                      style={{
                                        padding: '8px 12px',
                                        borderRadius: 8,
                                        fontSize: 12,
                                        textAlign: 'left',
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'space-between',
                                        cursor: 'pointer',
                                      }}
                                    >
                                      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                        <Fa icon={icons.file} style={{ color: 'var(--reyna-accent)', fontSize: 12 }} />
                                        <span style={{ fontWeight: 600 }}>{c.file_name}</span>
                                        {c.folder && (
                                          <span style={{ fontSize: 10, padding: '1px 6px', borderRadius: 4, background: 'rgba(0,0,0,0.06)' }}>
                                            {c.folder}
                                          </span>
                                        )}
                                      </div>
                                      <span style={{ fontSize: 11, color: 'var(--sub-color)' }}>
                                        {c.sender ? `by ${c.sender}` : ''}
                                      </span>
                                    </button>
                                  ))}
                                </div>
                              </div>
                            )}

                            {/* Attached File Cards */}
                            {turn.files && turn.files.length > 0 && (
                              <div
                                style={{
                                  display: 'flex',
                                  flexDirection: 'column',
                                  gap: 6,
                                  marginTop: 4,
                                  maxWidth: 580,
                                }}
                              >
                                {turn.files.map(f => (
                                  <div
                                    key={f.id}
                                    className="glass-card"
                                    style={{
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
                                        color="var(--main-color)"
                                        bg="rgba(37,211,102,0.12)"
                                        size={32}
                                        iconSize={13}
                                      />
                                      <div style={{ minWidth: 0 }}>
                                        <div
                                          style={{
                                            fontWeight: 600,
                                            fontSize: 13,
                                            color: 'var(--text-color)',
                                            whiteSpace: 'nowrap',
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis',
                                            maxWidth: 320,
                                          }}
                                          title={f.file_name}
                                        >
                                          {f.file_name}
                                        </div>
                                        <div style={{ fontSize: 11, color: 'var(--sub-color)' }}>
                                          {f.subject && <span style={{ fontWeight: 600, color: '#333' }}>{f.subject} · </span>}
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
                                          borderRadius: '50%',
                                          border: '1px solid rgba(0,0,0,0.1)',
                                          background: 'rgba(255,255,255,0.7)',
                                          cursor: 'pointer',
                                          display: 'flex',
                                          alignItems: 'center',
                                          justifyContent: 'center',
                                          color: 'var(--text-color)',
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
                                          borderRadius: '50%',
                                          border: '1px solid rgba(0,0,0,0.1)',
                                          background: 'rgba(255,255,255,0.7)',
                                          cursor: 'pointer',
                                          display: 'flex',
                                          alignItems: 'center',
                                          justifyContent: 'center',
                                          color: 'var(--text-color)',
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
                                              borderRadius: '50%',
                                              border: '1px solid rgba(0,0,0,0.1)',
                                              background: 'rgba(255,255,255,0.7)',
                                              display: 'flex',
                                              alignItems: 'center',
                                              justifyContent: 'center',
                                              color: 'var(--text-color)',
                                              textDecoration: 'none',
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
                                      maxWidth: 580,
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
                ))}

                {/* Follow-up Quick Prompts Floating Row */}
                {chatConversation.length > 0 &&
                  !chatConversation[chatConversation.length - 1].pending && (
                    <div
                      style={{
                        display: 'flex',
                        gap: 6,
                        flexWrap: 'wrap',
                        alignItems: 'center',
                        marginLeft: 52,
                        marginTop: 4,
                      }}
                    >
                      {followUpSuggestions.map(s => (
                        <button
                          key={s}
                          onClick={() => sendChat(s)}
                          className="glass-pill"
                          style={{
                            padding: '4px 12px',
                            borderRadius: 9999,
                            fontSize: 11,
                            fontWeight: 500,
                            cursor: 'pointer',
                            color: 'var(--text-color)',
                          }}
                        >
                          {s}
                        </button>
                      ))}
                    </div>
                  )}

                <div ref={chatEndRef} style={{ height: 10 }} />
              </div>
            )}
          </div>
        )}

        {/* ── Files Table Mode (Mode: search) ── */}
        {mode === 'search' && (
          <div
            className="glass-scroll"
            style={{
              flex: 1,
              overflowY: 'auto',
              padding: '24px 32px',
            }}
          >
            {/* Search Bar Input */}
            <div style={{ position: 'relative', marginBottom: 20 }}>
              <Fa
                icon={icons.search}
                style={{
                  position: 'absolute',
                  left: 16,
                  top: 15,
                  fontSize: 14,
                  color: 'var(--sub-color)',
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
                className="glass-card"
                style={{
                  width: '100%',
                  padding: '13px 16px 13px 44px',
                  borderRadius: 14,
                  fontSize: 14,
                  color: 'var(--text-color)',
                  outline: 'none',
                  fontFamily: 'inherit',
                }}
              />

              {showSuggestions && suggestions.length > 0 && (
                <div
                  className="glass-card"
                  style={{
                    position: 'absolute',
                    top: '100%',
                    left: 0,
                    right: 0,
                    marginTop: 6,
                    borderRadius: 12,
                    zIndex: 50,
                    overflow: 'hidden',
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
                        background: i === selectedSuggestion ? 'rgba(0,0,0,0.06)' : 'transparent',
                        borderBottom: i < suggestions.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                        display: 'flex',
                        alignItems: 'center',
                        gap: 8,
                      }}
                    >
                      <Fa icon={icons.file} style={{ fontSize: 12, color: 'var(--sub-color)' }} />
                      <span style={{ fontWeight: 500 }}>{name}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Results or Empty */}
            {results === null ? (
              <div style={{ textAlign: 'center', padding: '40px 20px', color: 'var(--sub-color)' }}>
                <Fa icon={icons.search} style={{ fontSize: 36, opacity: 0.3, marginBottom: 12 }} />
                <p style={{ fontSize: 14, margin: '0 0 10px' }}>Instant keyword search across your documents</p>
                <div style={{ display: 'flex', gap: 6, justifyContent: 'center', flexWrap: 'wrap' }}>
                  {['linear queue', 'notes', 'assignment', 'bayes', 'lab'].map(q => (
                    <button
                      key={q}
                      onClick={() => {
                        setQuery(q)
                        doLiveSearch(q)
                      }}
                      className="glass-pill"
                      style={{
                        padding: '4px 12px',
                        borderRadius: 9999,
                        fontSize: 11,
                        cursor: 'pointer',
                      }}
                    >
                      {q}
                    </button>
                  ))}
                </div>
              </div>
            ) : results.length === 0 ? (
              <div style={{ textAlign: 'center', padding: '40px 20px', color: 'var(--sub-color)' }}>
                <Fa icon="fa-inbox" style={{ fontSize: 32, opacity: 0.3, marginBottom: 8 }} />
                <p style={{ fontSize: 13 }}>No files found for "{query}"</p>
              </div>
            ) : (
              <div>
                <p style={{ fontSize: 12, color: 'var(--sub-color)', marginBottom: 12 }}>
                  {results.length} result{results.length !== 1 ? 's' : ''} for "{query}"
                </p>
                <div className="glass-card" style={{ borderRadius: 14, overflow: 'hidden' }}>
                  {results.map((f, i) => (
                    <div
                      key={f.id}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 12,
                        padding: '10px 14px',
                        borderBottom: i < results.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                        transition: 'background 0.15s',
                      }}
                    >
                      <IconBox
                        icon={fileIconClass(f.mime_type, f.file_name)}
                        color="var(--main-color)"
                        bg="rgba(37,211,102,0.12)"
                        size={32}
                        iconSize={13}
                      />
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-color)' }}>
                          {f.file_name}
                        </div>
                        <div style={{ fontSize: 11, color: 'var(--sub-color)' }}>
                          {f.subject && <span style={{ fontWeight: 600, color: '#333' }}>{f.subject} · </span>}
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
                            borderRadius: '50%',
                            border: '1px solid rgba(0,0,0,0.1)',
                            background: 'rgba(255,255,255,0.7)',
                            cursor: 'pointer',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
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
                            borderRadius: '50%',
                            border: '1px solid rgba(0,0,0,0.1)',
                            background: 'rgba(255,255,255,0.7)',
                            cursor: 'pointer',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
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
                            borderRadius: '50%',
                            border: '1px solid rgba(220,38,38,0.2)',
                            background: 'rgba(254,242,242,0.7)',
                            color: 'var(--error-color)',
                            cursor: 'pointer',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
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

        {/* ── Floating Frosted Capsule Composer (Matching Screenshot Bottom Bar) ── */}
        {mode === 'chat' && (
          <div
            style={{
              padding: '10px 24px 18px',
              position: 'relative',
              zIndex: 10,
            }}
          >
            {/* Quick Prompts Popover when `+` is tapped */}
            {showPromptsMenu && (
              <div
                className="glass-card"
                style={{
                  position: 'absolute',
                  bottom: '100%',
                  left: 32,
                  marginBottom: 10,
                  width: 300,
                  padding: 10,
                  borderRadius: 16,
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 6,
                  zIndex: 20,
                  boxShadow: '0 12px 30px rgba(0,0,0,0.12)',
                }}
              >
                <div style={{ fontSize: 11, fontWeight: 700, color: 'var(--sub-color)', padding: '2px 8px', textTransform: 'uppercase' }}>
                  Quick Ideas
                </div>
                {samplePrompts.map(p => (
                  <button
                    key={p}
                    onClick={() => sendChat(p)}
                    style={{
                      background: 'none',
                      border: 'none',
                      padding: '8px 10px',
                      borderRadius: 8,
                      textAlign: 'left',
                      fontSize: 12,
                      color: 'var(--text-color)',
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      transition: 'background 0.15s',
                    }}
                    onMouseEnter={e => e.currentTarget.style.background = 'rgba(0,0,0,0.06)'}
                    onMouseLeave={e => e.currentTarget.style.background = 'none'}
                  >
                    <Fa icon="fa-comment-dots" style={{ fontSize: 11, color: 'var(--reyna-accent)' }} />
                    <span style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{p}</span>
                  </button>
                ))}
              </div>
            )}

            {/* Pill Capsule Container */}
            <div
              className="glass-composer"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                borderRadius: 9999,
                padding: '6px 8px 6px 14px',
                transition: 'all 0.2s cubic-bezier(0.16, 1, 0.3, 1)',
              }}
            >
              {/* Left `+` button */}
              <button
                onClick={() => setShowPromptsMenu(p => !p)}
                title="Ideas & prompts"
                style={{
                  width: 32,
                  height: 32,
                  borderRadius: '50%',
                  border: 'none',
                  background: showPromptsMenu ? 'var(--reyna-accent)' : 'rgba(0,0,0,0.06)',
                  color: showPromptsMenu ? '#fff' : 'var(--text-color)',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                  transition: 'all 0.15s ease',
                }}
              >
                <Fa icon={showPromptsMenu ? icons.close : icons.add} style={{ fontSize: 12 }} />
              </button>

              {/* Main Textarea */}
              <textarea
                ref={inputRef}
                value={query}
                onChange={handleChange}
                onKeyDown={handleKeyDown}
                onInput={e => {
                  e.target.style.height = 'auto'
                  e.target.style.height = Math.min(e.target.scrollHeight, 140) + 'px'
                }}
                rows={1}
                placeholder="Message @reyna in English, Hindi, or Hinglish..."
                style={{
                  flex: 1,
                  minWidth: 0,
                  padding: '6px 0',
                  fontSize: 14.5,
                  background: 'transparent',
                  border: 'none',
                  color: 'var(--text-color)',
                  outline: 'none',
                  resize: 'none',
                  overflowY: 'hidden',
                  minHeight: 22,
                  maxHeight: 140,
                  fontFamily: 'inherit',
                  lineHeight: 1.5,
                }}
              />

              {/* Right Send Button */}
              <button
                onClick={() => sendChat(query)}
                disabled={!query.trim() || loading}
                title="Send"
                style={{
                  width: 36,
                  height: 36,
                  borderRadius: '50%',
                  border: 'none',
                  background: query.trim() ? '#25D366' : 'rgba(0,0,0,0.08)',
                  color: query.trim() ? '#fff' : 'rgba(0,0,0,0.3)',
                  cursor: query.trim() && !loading ? 'pointer' : 'default',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                  boxShadow: query.trim() ? '0 4px 12px rgba(37,211,102,0.4)' : 'none',
                  transform: query.trim() ? 'scale(1.05)' : 'scale(1)',
                  transition: 'all 0.2s cubic-bezier(0.16, 1, 0.3, 1)',
                }}
              >
                <Fa icon="fa-paper-plane" style={{ fontSize: 13 }} />
              </button>
            </div>
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
