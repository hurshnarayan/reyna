import { useState, useEffect, useRef, useCallback } from 'react'
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
      blocks.push(<div key={'br' + i} style={{ height: 8 }} />)
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
      <p key={'p' + i} style={{ margin: '6px 0', lineHeight: 1.7 }}>
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
            fontFamily: 'monospace',
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

const btnBase = {
  background: '#fff',
  border: '1.5px solid #ccc',
  borderRadius: 'var(--roundness)',
  padding: '8px 16px',
  fontSize: 14,
  fontWeight: 600,
  cursor: 'pointer',
  color: 'var(--text-secondary, #555)',
  transition: 'all 0.15s',
  display: 'inline-flex',
  alignItems: 'center',
  gap: 6,
}
const btnPrimary = {
  ...btnBase,
  background: 'var(--reyna-accent)',
  color: '#fff',
  border: '1.5px solid var(--reyna-accent)',
  fontWeight: 700,
}
const btnDanger = {
  ...btnBase,
  background: '#fef2f2',
  color: 'var(--error-color)',
  border: '1.5px solid rgba(220,38,38,0.15)',
}
const cardStyle = {
  background: 'var(--card-bg)',
  border: '1.5px solid #ccc',
  borderRadius: 'var(--roundness)',
  boxShadow: 'var(--card-shadow)',
}

export default function Search() {
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
  const [isContentSearch, setIsContentSearch] = useState(false)

  // Conversational Chat & Retrieval Thread
  const [chatConversation, setChatConversation] = useState([])
  const [llmStatus, setLlmStatus] = useState(null)
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
            reply: cleanReply(resp?.reply || 'I could not find anything matching that.'),
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
    setIsContentSearch(val.startsWith('/content:'))
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

  const switchMode = (m) => {
    setMode(m)
    setResults(null)
    setQuery('')
    setSuggestions([])
    setShowSuggestions(false)
  }

  const placeholders = {
    chat: 'ask in english, hindi, or hinglish (e.g. "bhai harsh ne kya bheja hai?")...',
    search: 'search files by name or tag...',
  }

  const tagColors = { WHO: '#D85A30', WHAT: '#7F77DD', WHEN: '#1D9E75', WHY: '#BA7517' }

  return (
    <div style={{ padding: '32px 40px', maxWidth: 840 }} className="fade-in">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 6 }}>
        <h1
          style={{
            fontSize: 40,
            fontWeight: 900,
            letterSpacing: -0.5,
            color: 'var(--text-color)',
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            margin: 0,
          }}
        >
          <Fa icon={mode === 'chat' ? 'fa-comments' : icons.search} style={{ fontSize: 30, color: 'var(--reyna-accent)' }} />
          {mode === 'chat' ? 'chat & retrieval' : 'search'}
        </h1>
        {mode === 'chat' && chatConversation.length > 0 && (
          <button
            onClick={clearChat}
            style={{
              ...btnBase,
              fontSize: 12,
              padding: '6px 14px',
              color: 'var(--sub-color)',
            }}
          >
            <Fa icon="fa-arrow-rotate-left" style={{ fontSize: 10 }} /> new conversation
          </button>
        )}
      </div>

      <p style={{ fontSize: 14, color: '#888', marginBottom: 20 }}>
        {mode === 'chat'
          ? 'ask questions, retrieve documents, and follow up in any language or phrasing.'
          : 'instant keyword matching across file names and subjects.'}
      </p>

      {/* Mode tabs */}
      <div
        style={{
          display: 'flex',
          gap: 4,
          marginBottom: 18,
          background: '#f5f5f5',
          borderRadius: 'var(--roundness)',
          padding: 3,
        }}
      >
        {[
          { id: 'chat', icon: 'fa-comments', label: 'Chat & Retrieval' },
          { id: 'search', icon: icons.search, label: 'Instant Search' },
        ].map(tab => (
          <button
            key={tab.id}
            onClick={() => switchMode(tab.id)}
            style={{
              flex: 1,
              padding: '8px 12px',
              border: 'none',
              borderRadius: 'var(--roundness)',
              fontSize: 12,
              fontWeight: 600,
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 6,
              background: mode === tab.id ? '#fff' : 'transparent',
              color: mode === tab.id ? 'var(--text-color)' : 'var(--sub-color)',
              boxShadow: mode === tab.id ? '0 1px 3px rgba(0,0,0,0.08)' : 'none',
            }}
          >
            <Fa icon={tab.icon} style={{ fontSize: 11 }} /> {tab.label}
          </button>
        ))}
      </div>

      {/* LLM status badge */}
      {mode === 'chat' && llmStatus && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            marginBottom: 16,
            fontSize: 11,
            color: llmStatus.enabled ? '#0F6E56' : '#BA7517',
          }}
        >
          <span
            style={{
              width: 6,
              height: 6,
              borderRadius: '50%',
              background: llmStatus.enabled ? '#25D366' : '#f59e0b',
            }}
          />
          {llmStatus.enabled
            ? `Multilingual AI Retrieval powered by ${llmStatus.provider}`
            : 'AI not configured — keyword fallback only'}
        </div>
      )}

      {/* ── CONVERSATIONAL CHAT MODE ── */}
      {mode === 'chat' && (
        <div style={{ marginBottom: 24 }}>
          {/* Empty Chat State */}
          {chatConversation.length === 0 ? (
            <div
              style={{
                textAlign: 'center',
                padding: '48px 24px',
                background: '#fafafa',
                border: '1.5px dashed #e0e0e0',
                borderRadius: 'var(--roundness)',
                marginBottom: 20,
              }}
            >
              <div
                style={{
                  width: 52,
                  height: 52,
                  borderRadius: '50%',
                  background: 'rgba(37,211,102,0.12)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  margin: '0 auto 14px',
                  color: 'var(--reyna-accent)',
                  fontSize: 22,
                }}
              >
                <Fa icon="fa-comments" />
              </div>
              <h3 style={{ fontSize: 18, fontWeight: 700, margin: '0 0 6px', color: 'var(--text-color)' }}>
                Ask anything in English, Hindi, or Hinglish
              </h3>
              <p style={{ fontSize: 13, color: '#777', maxWidth: 440, margin: '0 auto 18px', lineHeight: 1.5 }}>
                Reyna extracts text from shared documents, links who sent what, and answers factual questions with verified citations.
              </p>

              <div
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 8,
                  maxWidth: 420,
                  margin: '0 auto',
                  textAlign: 'left',
                }}
              >
                {[
                  'bhai harsh ne kya bheja hai?',
                  'do we have any notes on linear queue?',
                  'show me raspberry pi documents',
                  'what travel tickets are booked for us?',
                ].map(q => (
                  <button
                    key={q}
                    onClick={() => sendChat(q)}
                    style={{
                      padding: '9px 14px',
                      background: '#fff',
                      border: '1px solid #ddd',
                      borderRadius: 10,
                      fontSize: 13,
                      cursor: 'pointer',
                      color: 'var(--text-secondary, #444)',
                      textAlign: 'left',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      transition: 'all 0.15s',
                    }}
                    onMouseEnter={e => {
                      e.currentTarget.style.borderColor = 'var(--reyna-accent)'
                      e.currentTarget.style.background = 'rgba(37,211,102,0.04)'
                    }}
                    onMouseLeave={e => {
                      e.currentTarget.style.borderColor = '#ddd'
                      e.currentTarget.style.background = '#fff'
                    }}
                  >
                    <span>{q}</span>
                    <Fa icon="fa-arrow-right" style={{ fontSize: 10, color: 'var(--sub-color)' }} />
                  </button>
                ))}
              </div>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16, marginBottom: 20 }}>
              {chatConversation.map(turn => (
                <div key={turn.id} style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {/* User Bubble */}
                  <div
                    style={{
                      background: 'rgba(37,211,102,0.12)',
                      border: '1px solid rgba(37,211,102,0.3)',
                      borderRadius: '16px 16px 4px 16px',
                      padding: '10px 16px',
                      maxWidth: '82%',
                      alignSelf: 'flex-end',
                      fontSize: 14,
                      color: '#1a1a1a',
                      lineHeight: 1.55,
                      whiteSpace: 'pre-wrap',
                      wordBreak: 'break-word',
                    }}
                  >
                    {turn.question}
                  </div>

                  {/* Reyna Assistant Bubble */}
                  <div
                    style={{
                      ...cardStyle,
                      padding: '16px 20px',
                      maxWidth: '92%',
                      alignSelf: 'flex-start',
                      borderRadius: '16px 16px 16px 4px',
                    }}
                  >
                    <div
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        marginBottom: 8,
                      }}
                    >
                      <div
                        style={{
                          fontSize: 12,
                          fontWeight: 700,
                          color: 'var(--reyna-accent)',
                          display: 'flex',
                          alignItems: 'center',
                          gap: 6,
                        }}
                      >
                        <Fa icon="fa-crown" style={{ fontSize: 11 }} /> Reyna
                      </div>

                      {turn.parsed_query && (
                        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                          {[
                            { k: 'WHO', v: turn.parsed_query.who },
                            { k: 'WHAT', v: turn.parsed_query.what },
                            { k: 'WHEN', v: turn.parsed_query.when },
                          ]
                            .filter(t => t.v)
                            .map(t => (
                              <span
                                key={t.k}
                                style={{
                                  padding: '2px 8px',
                                  borderRadius: 12,
                                  fontSize: 10,
                                  fontWeight: 600,
                                  background: tagColors[t.k] + '15',
                                  color: tagColors[t.k],
                                }}
                              >
                                {t.k}: {t.v}
                              </span>
                            ))}
                        </div>
                      )}
                    </div>

                    {turn.pending ? (
                      <div
                        style={{
                          fontSize: 13,
                          color: 'var(--sub-color)',
                          display: 'flex',
                          alignItems: 'center',
                          gap: 8,
                          padding: '6px 0',
                        }}
                      >
                        <Fa icon={icons.loading} spin style={{ fontSize: 13 }} />
                        <span>thinking & searching documents…</span>
                      </div>
                    ) : (
                      <>
                        {/* Notice Banner (e.g. out of allowance) */}
                        {turn.notice && (
                          <div
                            style={{
                              background: '#fffbeb',
                              border: '1px solid #fde68a',
                              borderRadius: 8,
                              padding: '8px 12px',
                              marginBottom: 10,
                              fontSize: 12,
                              color: '#92400e',
                              display: 'flex',
                              alignItems: 'center',
                              gap: 8,
                            }}
                          >
                            <Fa icon={icons.warning} style={{ fontSize: 12 }} />
                            <span>{turn.reply}</span>
                          </div>
                        )}

                        {/* Conversational Markdown Answer */}
                        {!turn.notice && (
                          <div style={{ fontSize: 14, color: 'var(--text-color)', lineHeight: 1.65 }}>
                            {renderMarkdown(turn.reply)}
                          </div>
                        )}

                        {/* Ambiguity Candidate Selector */}
                        {turn.candidates && turn.candidates.length > 0 && (
                          <div style={{ marginTop: 14, paddingTop: 12, borderTop: '1px solid var(--card-border)' }}>
                            <div style={{ fontSize: 12, fontWeight: 700, color: '#b45309', marginBottom: 8 }}>
                              Which document did you mean?
                            </div>
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                              {turn.candidates.map(c => (
                                <button
                                  key={c.file_id}
                                  onClick={() => handleChooseCandidate(c, turn.question)}
                                  style={{
                                    ...btnBase,
                                    padding: '8px 12px',
                                    fontSize: 12,
                                    textAlign: 'left',
                                    justifyContent: 'space-between',
                                    background: '#fff',
                                    borderColor: '#e5e7eb',
                                  }}
                                  onMouseEnter={e => {
                                    e.currentTarget.style.borderColor = 'var(--reyna-accent)'
                                    e.currentTarget.style.background = 'rgba(37,211,102,0.04)'
                                  }}
                                  onMouseLeave={e => {
                                    e.currentTarget.style.borderColor = '#e5e7eb'
                                    e.currentTarget.style.background = '#fff'
                                  }}
                                >
                                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                    <Fa icon={icons.file} style={{ color: 'var(--reyna-accent)' }} />
                                    <span style={{ fontWeight: 600 }}>{c.file_name}</span>
                                    {c.folder && (
                                      <span
                                        style={{
                                          fontSize: 10,
                                          padding: '1px 6px',
                                          borderRadius: 4,
                                          background: '#f3f4f6',
                                          color: '#4b5563',
                                        }}
                                      >
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

                        {/* Attached Files Cards */}
                        {turn.files && turn.files.length > 0 && (
                          <div style={{ marginTop: 14, paddingTop: 12, borderTop: '1px solid var(--card-border)' }}>
                            <div
                              style={{
                                fontSize: 11,
                                fontWeight: 700,
                                color: 'var(--sub-color)',
                                letterSpacing: 0.5,
                                textTransform: 'uppercase',
                                marginBottom: 8,
                                display: 'flex',
                                alignItems: 'center',
                                gap: 6,
                              }}
                            >
                              <Fa icon={icons.files} style={{ fontSize: 10 }} />
                              attached files ({turn.files.length})
                            </div>
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                              {turn.files.map(f => (
                                <div
                                  key={f.id}
                                  style={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'space-between',
                                    padding: '8px 12px',
                                    background: '#f9fafb',
                                    border: '1px solid #e5e7eb',
                                    borderRadius: 8,
                                    fontSize: 13,
                                  }}
                                >
                                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
                                    <IconBox
                                      icon={fileIconClass(f.mime_type, f.file_name)}
                                      color="var(--main-color)"
                                      bg="rgba(37,211,102,0.1)"
                                      size={28}
                                      iconSize={12}
                                    />
                                    <div style={{ minWidth: 0 }}>
                                      <div
                                        style={{
                                          fontWeight: 600,
                                          color: 'var(--text-color)',
                                          whiteSpace: 'nowrap',
                                          overflow: 'hidden',
                                          textOverflow: 'ellipsis',
                                          maxWidth: 360,
                                        }}
                                        title={f.file_name}
                                      >
                                        {f.file_name}
                                      </div>
                                      <div style={{ fontSize: 11, color: 'var(--sub-color)' }}>
                                        {f.subject && (
                                          <span style={{ marginRight: 6, fontWeight: 500, color: '#4b5563' }}>
                                            {f.subject} ·
                                          </span>
                                        )}
                                        {f.shared_by_name ? `by ${f.shared_by_name}` : 'shared in chat'}
                                        {f.posted_at || f.created_at
                                          ? ` · ${timeAgo(f.posted_at || f.created_at)}`
                                          : ''}
                                      </div>
                                    </div>
                                  </div>

                                  <div style={{ display: 'flex', gap: 4, flexShrink: 0 }}>
                                    <button
                                      onClick={() => openPreview(f)}
                                      title="Preview"
                                      style={{ ...btnBase, padding: '4px 8px', fontSize: 11 }}
                                    >
                                      <Fa icon={icons.preview} />
                                    </button>
                                    <button
                                      onClick={() => downloadFile(f)}
                                      title="Download"
                                      style={{ ...btnBase, padding: '4px 8px', fontSize: 11 }}
                                    >
                                      <Fa icon={icons.download} />
                                    </button>
                                    {f.drive_file_id &&
                                      !f.drive_file_id.startsWith('local_') &&
                                      !f.drive_file_id.startsWith('meta_') && (
                                        <a
                                          href={`https://drive.google.com/file/d/${f.drive_file_id}/view`}
                                          target="_blank"
                                          rel="noreferrer"
                                          title="Open in Drive"
                                          style={{ ...btnBase, padding: '4px 8px', fontSize: 11, textDecoration: 'none' }}
                                        >
                                          <Fa icon={icons.link} />
                                        </a>
                                      )}
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                        {/* Verified Citations Accordion */}
                        {turn.citations && turn.citations.length > 0 && (
                          <div style={{ marginTop: 12 }}>
                            <button
                              onClick={() => toggleSources(turn.id)}
                              style={{
                                ...btnBase,
                                padding: '4px 10px',
                                fontSize: 11,
                                color: 'var(--sub-color)',
                                border: '1px solid #e5e7eb',
                                background: turn.showSources ? 'rgba(37,211,102,0.06)' : '#fff',
                              }}
                            >
                              <Fa icon={icons.file} style={{ fontSize: 10, color: 'var(--reyna-accent)' }} />
                              {turn.citations.length} verified passage{turn.citations.length > 1 ? 's' : ''}
                              <Fa icon={turn.showSources ? 'fa-chevron-up' : 'fa-chevron-down'} style={{ fontSize: 8 }} />
                            </button>

                            {turn.showSources && (
                              <div
                                style={{
                                  marginTop: 8,
                                  display: 'flex',
                                  flexDirection: 'column',
                                  gap: 6,
                                  background: '#f9fafb',
                                  padding: 10,
                                  borderRadius: 8,
                                  border: '1px solid #e5e7eb',
                                }}
                              >
                                {turn.citations.map((c, ci) => (
                                  <div
                                    key={ci}
                                    style={{
                                      fontSize: 12,
                                      color: 'var(--text-color)',
                                      padding: '6px 8px',
                                      background: '#fff',
                                      borderRadius: 6,
                                      borderLeft: '3px solid var(--reyna-accent)',
                                    }}
                                  >
                                    <div style={{ fontStyle: 'italic', marginBottom: 4, lineHeight: 1.5 }}>
                                      "{c.quote}"
                                    </div>
                                    <div
                                      style={{
                                        fontSize: 10,
                                        color: 'var(--sub-color)',
                                        display: 'flex',
                                        gap: 8,
                                        alignItems: 'center',
                                      }}
                                    >
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
                      </>
                    )}
                  </div>
                </div>
              ))}

              {/* Follow-up Quick Prompts */}
              {chatConversation.length > 0 &&
                !chatConversation[chatConversation.length - 1].pending && (
                  <div
                    style={{
                      display: 'flex',
                      gap: 8,
                      marginTop: 6,
                      flexWrap: 'wrap',
                      alignItems: 'center',
                    }}
                  >
                    <span style={{ fontSize: 11, color: 'var(--sub-color)', marginRight: 4 }}>
                      follow-up:
                    </span>
                    {[
                      'tell me more',
                      'in simpler words',
                      'fetch it',
                      'who sent this?',
                      'summarize it',
                      'hindi me samjhao',
                    ].map(s => (
                      <button
                        key={s}
                        onClick={() => {
                          setQuery(s)
                          inputRef.current?.focus()
                        }}
                        style={{
                          padding: '4px 10px',
                          background: '#fff',
                          border: '1px solid #ddd',
                          borderRadius: 14,
                          fontSize: 11,
                          cursor: 'pointer',
                          color: '#555',
                          fontWeight: 500,
                          transition: 'all 0.15s',
                        }}
                        onMouseEnter={e => {
                          e.currentTarget.style.borderColor = 'var(--reyna-accent)'
                          e.currentTarget.style.color = 'var(--reyna-accent)'
                        }}
                        onMouseLeave={e => {
                          e.currentTarget.style.borderColor = '#ddd'
                          e.currentTarget.style.color = '#555'
                        }}
                      >
                        {s}
                      </button>
                    ))}
                  </div>
                )}
              <div ref={chatEndRef} />
            </div>
          )}

          {/* Chat Composer Input */}
          <div
            style={{
              position: 'relative',
              display: 'flex',
              alignItems: 'flex-end',
              gap: 10,
              background: '#fff',
              border: '1.5px solid #ccc',
              borderRadius: 14,
              padding: '12px 14px 12px 18px',
              transition: 'border-color 0.15s',
            }}
            onFocusCapture={e => (e.currentTarget.style.borderColor = '#2563eb')}
            onBlurCapture={e => (e.currentTarget.style.borderColor = '#ccc')}
          >
            <Fa
              icon="fa-comments"
              style={{ fontSize: 16, color: '#555', marginBottom: 8, flexShrink: 0 }}
            />
            <textarea
              ref={inputRef}
              value={query}
              onChange={handleChange}
              onKeyDown={handleKeyDown}
              onInput={e => {
                e.target.style.height = 'auto'
                e.target.style.height = Math.min(e.target.scrollHeight, 280) + 'px'
              }}
              rows={1}
              placeholder={placeholders.chat}
              style={{
                flex: 1,
                minWidth: 0,
                padding: '6px 4px',
                fontSize: 15,
                background: 'transparent',
                border: 'none',
                color: '#1a1a1a',
                outline: 'none',
                resize: 'none',
                overflowY: 'hidden',
                minHeight: 24,
                maxHeight: 280,
                fontFamily: 'inherit',
                lineHeight: 1.55,
              }}
            />
            <button
              onClick={() => sendChat(query)}
              disabled={!query.trim() || loading}
              style={{
                ...btnPrimary,
                padding: '8px 16px',
                fontSize: 13,
                opacity: !query.trim() || loading ? 0.5 : 1,
                cursor: !query.trim() || loading ? 'not-allowed' : 'pointer',
                flexShrink: 0,
                alignSelf: 'flex-end',
              }}
            >
              <Fa icon="fa-paper-plane" style={{ fontSize: 11 }} /> Send
            </button>
          </div>
        </div>
      )}

      {/* ── INSTANT SEARCH MODE ── */}
      {mode === 'search' && (
        <div style={{ position: 'relative', marginBottom: 28 }}>
          <div style={{ position: 'relative' }}>
            <Fa
              icon={icons.search}
              style={{
                position: 'absolute',
                left: 18,
                top: 18,
                fontSize: 15,
                color: '#666',
                pointerEvents: 'none',
                zIndex: 1,
              }}
            />
            <input
              ref={inputRef}
              value={query}
              onChange={handleChange}
              onKeyDown={handleKeyDown}
              onFocus={() => suggestions.length > 0 && setShowSuggestions(true)}
              onBlur={() => setTimeout(() => setShowSuggestions(false), 200)}
              placeholder={placeholders.search}
              style={{
                width: '100%',
                padding: '16px 18px 16px 46px',
                fontSize: 15,
                background: '#fff',
                border: '1.5px solid #ccc',
                borderRadius: 'var(--roundness)',
                color: '#1a1a1a',
                outline: 'none',
                transition: 'border-color 0.15s',
                fontFamily: 'inherit',
              }}
              onFocusCapture={e => (e.target.style.borderColor = '#2563eb')}
              onBlurCapture={e => (e.target.style.borderColor = '#ccc')}
            />

            {/* Autocomplete */}
            {showSuggestions && suggestions.length > 0 && (
              <div
                style={{
                  position: 'absolute',
                  top: '100%',
                  left: 0,
                  right: 0,
                  ...cardStyle,
                  marginTop: 4,
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
                      background: i === selectedSuggestion ? 'var(--card-hover)' : 'var(--card-bg)',
                      borderBottom: i < suggestions.length - 1 ? '1px solid var(--card-border)' : 'none',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      color: 'var(--text-color)',
                    }}
                    onMouseEnter={e => (e.currentTarget.style.background = 'var(--card-hover)')}
                    onMouseLeave={e =>
                      (e.currentTarget.style.background =
                        i === selectedSuggestion ? 'var(--card-hover)' : 'var(--card-bg)')
                    }
                  >
                    <Fa icon={icons.file} style={{ fontSize: 13, color: 'var(--sub-color)' }} />
                    <span style={{ fontWeight: 500 }}>{name}</span>
                    {i === 0 && (
                      <span
                        style={{
                          marginLeft: 'auto',
                          fontSize: 9,
                          color: 'var(--sub-color)',
                          background: 'var(--bg-color)',
                          padding: '2px 6px',
                          borderRadius: 'var(--roundness)',
                        }}
                      >
                        tab
                      </span>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
          {loading && (
            <Fa
              icon={icons.loading}
              spin
              style={{
                position: 'absolute',
                right: 14,
                top: 18,
                fontSize: 13,
                color: 'var(--sub-color)',
              }}
            />
          )}

          {/* Search Empty / Results */}
          {results === null ? (
            <div style={{ textAlign: 'center', padding: 60, color: 'var(--sub-color)' }}>
              <Fa icon={icons.search} style={{ fontSize: 48, marginBottom: 14, opacity: 0.3 }} />
              <p style={{ fontSize: 14 }}>search for files across all your groups</p>
              <p style={{ fontSize: 11, marginTop: 6, color: 'var(--sub-color)' }}>
                start typing to search by filename. results update live.
              </p>
              <div
                style={{
                  marginTop: 16,
                  display: 'flex',
                  gap: 6,
                  justifyContent: 'center',
                  flexWrap: 'wrap',
                }}
              >
                {['DSA', 'notes', 'queue', 'assignment', 'lab'].map(q => (
                  <button
                    key={q}
                    onClick={() => {
                      setQuery(q)
                      doLiveSearch(q)
                      fetchSuggestions(q)
                    }}
                    style={{
                      padding: '5px 12px',
                      background: 'var(--reyna-accent-dim)',
                      border: '1px solid rgba(37,211,102,0.2)',
                      borderRadius: 'var(--roundness)',
                      fontSize: 11,
                      cursor: 'pointer',
                      color: 'var(--reyna-accent)',
                      fontWeight: 400,
                    }}
                  >
                    {q}
                  </button>
                ))}
              </div>
            </div>
          ) : results.length === 0 ? (
            <div style={{ textAlign: 'center', padding: 60, color: 'var(--sub-color)' }}>
              <Fa icon="fa-inbox" style={{ fontSize: 40, marginBottom: 12, opacity: 0.3 }} />
              <p style={{ fontSize: 14 }}>nothing found for "{query}"</p>
            </div>
          ) : (
            <div style={{ marginTop: 24 }}>
              <p style={{ fontSize: 13, color: 'var(--sub-color)', marginBottom: 14 }}>
                {results.length} result{results.length !== 1 ? 's' : ''} for "{query}"
                {isContentSearch && <span style={{ color: '#8b5cf6', fontWeight: 500 }}> (content search)</span>}
              </p>
              <div style={{ ...cardStyle, overflow: 'hidden' }}>
                {results.map((f, i) => (
                  <div
                    key={f.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 14,
                      padding: '12px 16px',
                      borderBottom: i < results.length - 1 ? '1px solid var(--card-border)' : 'none',
                      transition: 'background 0.15s',
                    }}
                    onMouseEnter={e => (e.currentTarget.style.background = 'var(--card-hover)')}
                    onMouseLeave={e => (e.currentTarget.style.background = 'var(--card-bg)')}
                  >
                    <IconBox
                      icon={fileIconClass(f.mime_type, f.file_name)}
                      color="var(--main-color)"
                      bg="rgba(37,211,102,0.08)"
                      size={36}
                      iconSize={14}
                    />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)' }}>
                        {f.file_name}
                      </div>
                      <div style={{ fontSize: 12, color: 'var(--sub-color)', marginTop: 2 }}>
                        {f.subject && <span style={{ fontWeight: 500, color: '#4b5563' }}>{f.subject} · </span>}
                        {formatBytes(f.file_size)} · by {f.shared_by_name || 'unknown'} · {timeAgo(f.posted_at || f.created_at)}
                      </div>
                    </div>

                    <div style={{ display: 'flex', gap: 6 }}>
                      <button
                        onClick={() => openPreview(f)}
                        title="Preview"
                        style={{ ...btnBase, padding: '5px 8px' }}
                      >
                        <Fa icon={icons.preview} style={{ fontSize: 11 }} />
                      </button>
                      <button
                        onClick={() => downloadFile(f)}
                        title="Download"
                        style={{ ...btnBase, padding: '5px 8px' }}
                      >
                        <Fa icon={icons.download} style={{ fontSize: 11 }} />
                      </button>
                      <button
                        onClick={() => confirmDelete(f)}
                        title="Delete"
                        style={{ ...btnDanger, padding: '5px 8px' }}
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

      {/* Preview Modal */}
      {previewFile &&
        createPortal(
          <div
            style={{
              position: 'fixed',
              top: 0,
              left: 0,
              width: '100vw',
              height: '100vh',
              background: 'rgba(0,0,0,0.8)',
              display: 'flex',
              flexDirection: 'column',
              zIndex: 99999,
            }}
          >
            <div
              style={{
                padding: '10px 20px',
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                background: '#fff',
                borderBottom: '1px solid var(--card-border)',
                flexShrink: 0,
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <Fa
                  icon={fileIconClass(previewFile.mime_type, previewFile.file_name)}
                  style={{ fontSize: 18, color: 'var(--main-color)' }}
                />
                <div>
                  <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)' }}>
                    {previewFile.file_name}
                  </div>
                  <div style={{ fontSize: 13, color: 'var(--sub-color)' }}>
                    {formatBytes(previewFile.file_size)} · by {previewFile.shared_by_name || 'unknown'}
                  </div>
                </div>
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                {previewData?.driveUrl && (
                  <a
                    href={previewData.driveUrl}
                    target="_blank"
                    rel="noreferrer"
                    style={{ ...btnBase, textDecoration: 'none' }}
                  >
                    <Fa icon={icons.link} style={{ fontSize: 9 }} /> open in drive
                  </a>
                )}
                <button onClick={() => downloadFile(previewFile)} style={btnBase}>
                  <Fa icon={icons.download} style={{ fontSize: 10 }} /> download
                </button>
                <button
                  onClick={() => {
                    setPreviewFile(null)
                    setPreviewData(null)
                  }}
                  style={{
                    ...btnBase,
                    borderRadius: '50%',
                    width: 32,
                    height: 32,
                    padding: 0,
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
              }}
            >
              {previewLoading ? (
                <div style={{ color: 'var(--sub-color)' }}>
                  <Fa icon={icons.loading} spin /> loading...
                </div>
              ) : previewData?.type === 'drive' ? (
                <iframe
                  src={previewData.url}
                  style={{ width: '100%', height: '100%', border: 'none' }}
                  title="Preview"
                />
              ) : previewData?.type === 'image' ? (
                <img
                  src={previewData.url}
                  alt=""
                  style={{
                    maxWidth: '95%',
                    maxHeight: '95%',
                    borderRadius: 'var(--roundness)',
                    objectFit: 'contain',
                  }}
                />
              ) : previewData?.type === 'text' ? (
                <div
                  style={{
                    width: '100%',
                    height: '100%',
                    overflow: 'auto',
                    padding: '20px 28px',
                    background: '#fff',
                  }}
                >
                  <pre
                    style={{
                      fontFamily: 'var(--font-mono)',
                      fontSize: 12,
                      lineHeight: 1.7,
                      whiteSpace: 'pre-wrap',
                      color: 'var(--text-color)',
                      margin: 0,
                    }}
                  >
                    {previewData.content}
                  </pre>
                </div>
              ) : (
                <div style={{ color: 'var(--sub-color)', textAlign: 'center' }}>
                  <Fa icon={icons.file} style={{ fontSize: 48, color: '#1a1a1a', marginBottom: 12 }} />
                  <p>no preview available</p>
                  <button
                    onClick={() => downloadFile(previewFile)}
                    style={{ ...btnPrimary, marginTop: 12 }}
                  >
                    <Fa icon={icons.download} /> download
                  </button>
                </div>
              )}
            </div>
          </div>,
          document.body
        )}

      {/* Delete Confirmation Modal */}
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
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              zIndex: 99999,
            }}
          >
            <div
              onClick={e => e.stopPropagation()}
              style={{
                background: '#fff',
                borderRadius: 'var(--roundness)',
                padding: 28,
                maxWidth: 420,
                width: '90vw',
                border: '1.5px solid #ccc',
                boxShadow:
                  '0 25px 60px rgba(0,0,0,0.15), 0 8px 20px rgba(0,0,0,0.1)',
              }}
            >
              <h3
                style={{
                  fontSize: 20,
                  fontWeight: 700,
                  letterSpacing: -0.3,
                  color: 'var(--error-color)',
                  marginBottom: 12,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                }}
              >
                <Fa icon={icons.warning} style={{ fontSize: 14 }} /> Delete File?
              </h3>
              <p
                style={{
                  fontSize: 14,
                  color: 'var(--text-secondary, #555)',
                  lineHeight: 1.6,
                  marginBottom: 24,
                }}
              >
                delete <strong style={{ color: 'var(--text-color)' }}>{deleteTarget.file_name}</strong>? moves to drive trash (30 day recovery).
              </p>
              <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
                <button onClick={() => setDeleteTarget(null)} style={btnBase}>
                  cancel
                </button>
                <button
                  onClick={doDelete}
                  disabled={deleting}
                  style={{ ...btnDanger, opacity: deleting ? 0.6 : 1 }}
                >
                  {deleting ? 'deleting...' : 'delete'}
                </button>
              </div>
            </div>
          </div>,
          document.body
        )}
    </div>
  )
}
