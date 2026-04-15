import { useState, useEffect, useRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { api, getToken } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa, icons, fileIconClass, IconBox } from '../components/icons'

function formatBytes(b) { if (b < 1024) return b + ' B'; if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'; return (b / 1048576).toFixed(1) + ' MB' }
function timeAgo(d) { const s = Math.floor((Date.now() - new Date(d).getTime()) / 1000); if (s < 60) return 'just now'; if (s < 3600) return Math.floor(s / 60) + 'm ago'; if (s < 86400) return Math.floor(s / 3600) + 'h ago'; return Math.floor(s / 86400) + 'd ago' }

// cleanReply strips JSON envelopes and code fences that the LLM occasionally
// wraps free-form answers in. Defensive — backend already cleans, but we
// double-check on the client so old cached answers also render correctly.
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

// renderMarkdown does a tiny inline markdown-to-React render: **bold**,
// `code`, > blockquotes, and bullet lists. No external deps.
function renderMarkdown(text) {
  if (!text) return null
  const lines = text.split('\n')
  const blocks = []
  let listBuf = []
  const flushList = () => {
    if (listBuf.length > 0) {
      blocks.push(<ul key={'ul' + blocks.length} style={{ margin: '6px 0 6px 18px', padding: 0 }}>
        {listBuf.map((item, i) => <li key={i} style={{ marginBottom: 4 }}>{renderInline(item)}</li>)}
      </ul>)
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
      blocks.push(<blockquote key={'q' + i} style={{ margin: '6px 0', padding: '6px 12px', borderLeft: '3px solid var(--reyna-accent)', background: 'rgba(37,211,102,0.06)', fontStyle: 'italic', borderRadius: 4 }}>{renderInline(line.slice(2))}</blockquote>)
      return
    }
    blocks.push(<p key={'p' + i} style={{ margin: '6px 0', lineHeight: 1.7 }}>{renderInline(line)}</p>)
  })
  flushList()
  return blocks
}

function renderInline(text) {
  // Split by **bold** and `code` markers, preserving content.
  const parts = []
  const re = /(\*\*[^*]+\*\*|`[^`]+`)/g
  let lastIdx = 0
  let m
  let key = 0
  while ((m = re.exec(text)) !== null) {
    if (m.index > lastIdx) parts.push(text.slice(lastIdx, m.index))
    const tok = m[0]
    if (tok.startsWith('**')) {
      parts.push(<strong key={'b' + key++} style={{ fontWeight: 700 }}>{tok.slice(2, -2)}</strong>)
    } else if (tok.startsWith('`')) {
      parts.push(<code key={'c' + key++} style={{ background: 'rgba(0,0,0,0.06)', padding: '1px 6px', borderRadius: 4, fontSize: '0.92em', fontFamily: 'monospace' }}>{tok.slice(1, -1)}</code>)
    }
    lastIdx = m.index + tok.length
  }
  if (lastIdx < text.length) parts.push(text.slice(lastIdx))
  return parts
}


const btnBase = { background: '#fff', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)', padding: '8px 16px', fontSize: 14, fontWeight: 600, cursor: 'pointer', color: 'var(--text-secondary, #555)', transition: 'all 0.15s', display: 'inline-flex', alignItems: 'center', gap: 6 }
const btnPrimary = { ...btnBase, background: 'var(--reyna-accent)', color: '#fff', border: '1.5px solid var(--reyna-accent)', fontWeight: 700 }
const btnDanger = { ...btnBase, background: '#fef2f2', color: 'var(--error-color)', border: '1.5px solid rgba(220,38,38,0.15)' }
const cardStyle = { background: 'var(--card-bg)', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)', boxShadow: 'var(--card-shadow)' }

export default function Search() {
  // mode — 'search' is filename keyword search; 'recall' is the unified
  // Recall mode that merges NLP retrieval + Notes Q&A into one thread
  // (semantic-backed when Qdrant is configured).
  const [mode, setMode] = useState('recall')
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
  // Recall conversation — merged NLP retrieval + Q&A. Each turn =
  //   { question, answer, sources, files, drive_sources, ts, pending? }
  // Newest turn is at the END so the thread reads top-to-bottom.
  const [recallConversation, setRecallConversation] = useState([])
  const [llmStatus, setLlmStatus] = useState(null)
  const qaEndRef = useRef(null)
  useEffect(() => {
    if (qaEndRef.current) qaEndRef.current.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [recallConversation])

  const inputRef = useRef(null)
  const debounceRef = useRef(null)

  useEffect(() => { api.llmStatus().then(s => setLlmStatus(s)) }, [])

  const doLiveSearch = useCallback(async (q) => {
    if (!q.trim() || q.trim().length < 2) { setResults(null); return }
    setLoading(true); notify.showLoader()
    const files = await api.search(q.trim()); setResults(files || []); setLoading(false); notify.hideLoader()
  }, [])

  // Legacy NLP-only handler. Kept as dead code for reference — the unified
  // Recall mode now covers everything it did plus Q&A and semantic search.
  // eslint-disable-next-line no-unused-vars
  const doNLPSearch = async (_q) => {
    // Intentionally disabled. See doRecall below.
  }

  // doRecall appends a new turn to the recall conversation. If there's a
  // recent prior turn (within 30 min) AND the new question looks like a
  // follow-up — short, pronoun-led, refinement cues, multi-language — we
  // pass the previous turn through so Gemini threads the answer.
  // Otherwise it's a fresh question.
  const isQAFollowup = (text) => {
    const t = text.toLowerCase().trim()
    if (t.length > 80) return false
    const cues = [
      'tell me more', 'more', 'elaborate', 'expand', 'continue', 'go on',
      'what about', 'and ', 'also ', 'how about', 'what else', 'anything else',
      'simpler', 'simply', 'in simple', 'asaan', 'easy', 'shorter', 'shorten', 'short me', 'briefly',
      'longer', 'in detail', 'detailed', 'vistaar', 'aur', 'aur kya', 'kuch aur',
      'example', 'examples', 'for example', 'udaharan',
      'why', 'kyu', 'kyun', 'how', 'kaise',
      'translate', 'in english', 'in hindi',
      'that ', 'this ', 'it ',
    ]
    for (const c of cues) {
      if (t === c.trim() || t.startsWith(c) || t.includes(' ' + c)) return true
    }
    return false
  }

  const doRecall = async (q) => {
    if (!q.trim()) return
    const question = q.trim()
    setLoading(true); notify.showLoader()

    // Follow-up detection — same 30-min window used for Q&A threading.
    let prevTurn = null
    if (recallConversation.length > 0) {
      const last = recallConversation[recallConversation.length - 1]
      const fresh = (Date.now() - (last.ts || 0)) < 30 * 60 * 1000
      if (fresh && isQAFollowup(question)) {
        prevTurn = { question: last.question, answer: last.answer, sources: last.sources }
      }
    }
    const placeholder = { question, answer: '', sources: [], files: [], drive_sources: [], ts: Date.now(), pending: true }
    setRecallConversation(prev => [...prev, placeholder])
    setQuery('')

    // Recall endpoint merges retrieval + Q&A, returning answer + files +
    // sources + drive_sources in one shot. Backend does semantic search via
    // Qdrant when configured and injects Reyna's Memory context.
    const resp = await api.recallAsk(question, '', prevTurn)
    const entry = {
      question,
      answer: resp?.answer || 'No answer',
      sources: resp?.sources || [],
      files: resp?.files || [],
      drive_sources: resp?.drive_sources || [],
      ts: Date.now(),
    }
    setRecallConversation(prev => {
      const copy = [...prev]
      copy[copy.length - 1] = entry
      return copy
    })
    setLoading(false); notify.hideLoader()
  }

  // Alias kept so existing callers (like the old mode === 'qa' check below)
  // continue to work while the UI transitions to 'recall'.
  const doQA = doRecall

  const clearQA = () => { setRecallConversation([]); setQuery('') }

  const fetchSuggestions = useCallback(async (q) => {
    if (!q.trim() || q.startsWith('/content:')) { setSuggestions([]); return }
    const names = await api.suggest(q.trim()); setSuggestions(names || [])
  }, [])

  const handleChange = (e) => {
    const val = e.target.value; setQuery(val); setSelectedSuggestion(-1); setIsContentSearch(val.startsWith('/content:'))
    if (mode === 'search') {
      if (debounceRef.current) clearTimeout(debounceRef.current)
      debounceRef.current = setTimeout(() => {
        if (val.trim().length >= 2) { fetchSuggestions(val.trim()); setShowSuggestions(true); doLiveSearch(val) }
        else { setSuggestions([]); setShowSuggestions(false); if (val.trim().length === 0) setResults(null) }
      }, 300)
    }
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Tab' && suggestions.length > 0 && mode === 'search') { e.preventDefault(); const idx = selectedSuggestion >= 0 ? selectedSuggestion : 0; if (suggestions[idx]) { setQuery(suggestions[idx]); setShowSuggestions(false); doLiveSearch(suggestions[idx]) } return }
    if (e.key === 'ArrowDown') { e.preventDefault(); setSelectedSuggestion(prev => Math.min(prev + 1, suggestions.length - 1)); return }
    if (e.key === 'ArrowUp') { e.preventDefault(); setSelectedSuggestion(prev => Math.max(prev - 1, -1)); return }
    if (e.key === 'Enter') {
      setShowSuggestions(false)
      if (mode === 'search') { if (selectedSuggestion >= 0 && suggestions[selectedSuggestion]) { setQuery(suggestions[selectedSuggestion]); doLiveSearch(suggestions[selectedSuggestion]) } else { doLiveSearch(query) } }
      else if (mode === 'recall') { doRecall(query) }
      return
    }
    if (e.key === 'Escape') { setShowSuggestions(false) }
  }

  const selectSuggestion = (name) => { setQuery(name); setShowSuggestions(false); doLiveSearch(name) }

  const openPreview = async (f) => {
    setPreviewFile(f); setPreviewLoading(true); setPreviewData(null)
    if (f.drive_file_id && !f.drive_file_id.startsWith('local_') && !f.drive_file_id.startsWith('meta_')) {
      setPreviewData({ type: 'drive', url: `https://drive.google.com/file/d/${f.drive_file_id}/preview`, driveUrl: `https://drive.google.com/file/d/${f.drive_file_id}/view` })
      setPreviewLoading(false); return
    }
    try {
      const resp = await fetch(api.downloadUrl(f.id), { headers: { 'Authorization': `Bearer ${getToken()}` } })
      const ct = resp.headers.get('content-type') || ''
      if (ct.includes('json')) { const info = await resp.json(); setPreviewData(info.preview_url ? { type: 'drive', url: info.preview_url, driveUrl: info.drive_url } : { type: 'not_available' }) }
      else if (ct.includes('image')) { setPreviewData({ type: 'image', url: URL.createObjectURL(await resp.blob()) }) }
      else if (ct.includes('text')) { setPreviewData({ type: 'text', content: await resp.text() }) }
      else { setPreviewData({ type: 'not_available' }) }
    } catch { setPreviewData({ type: 'not_available' }) }
    setPreviewLoading(false)
  }
  const downloadFile = (f) => {
    if (f.drive_file_id && !f.drive_file_id.startsWith('local_') && !f.drive_file_id.startsWith('meta_')) { window.open(`https://drive.google.com/file/d/${f.drive_file_id}/view`, '_blank'); return }
    fetch(api.downloadUrl(f.id), { headers: { 'Authorization': `Bearer ${getToken()}` } }).then(r => r.blob()).then(blob => { const u = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = u; a.download = f.file_name; a.click(); URL.revokeObjectURL(u) })
  }
  const confirmDelete = (f) => setDeleteTarget(f)
  const doDelete = async () => { if (!deleteTarget) return; setDeleting(true); await api.deleteFile(deleteTarget.id); notify.success(`deleted ${deleteTarget.file_name}`); setDeleteTarget(null); setDeleting(false); doLiveSearch(query) }

  const switchMode = (m) => { setMode(m); setResults(null); setRecallConversation([]); setQuery(''); setSuggestions([]); setShowSuggestions(false) }

  const placeholders = {
    search: 'search files by name...',
    recall: 'ask by meaning, e.g. "summarize the dbms notes mohit sent last week"',
  }

  const tagColors = { WHO: '#D85A30', WHAT: '#7F77DD', WHEN: '#1D9E75', WHY: '#BA7517' }

  return (
    <div style={{ padding: '32px 40px', maxWidth: 900, position: 'relative' }} className="fade-in">
      <div style={{ position: 'relative', zIndex: 1 }}>
      {/* Recall hero. Big hourglass icon anchors the heading, title-case
          brand name. Subtext stays lowercase to match the rest of the UI. */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 22 }}>
        <Fa
          icon={mode === 'recall' ? 'fa-clock-rotate-left' : icons.search}
          style={{ fontSize: 42, color: mode === 'recall' ? 'var(--reyna-accent)' : 'var(--text-color)' }}
        />
        <div>
          <h1 style={{ fontSize: 40, fontWeight: 800, letterSpacing: -0.5, margin: 0, color: 'var(--text-color)', lineHeight: 1.05 }}>
            {mode === 'recall' ? "Reyna's Recall" : 'Search'}
          </h1>
          <p style={{ fontSize: 14, color: '#888', margin: '4px 0 0 0' }}>
            {mode === 'recall'
              ? 'ask by meaning, and reyna finds, summarises, and answers from your notes'
              : 'keyword lookup by filename, fast and literal'}
          </p>
        </div>
      </div>

      {/* mode tabs. merged nlp retrieval + notes q&a into a single recall;
          old 3-tab ui preserved as a commented block at the end of the file. */}
      <div style={{ display: 'flex', gap: 4, margin: '0 0 20px', background: '#f5f5f5', borderRadius: 'var(--roundness)', padding: 3 }}>
        {[
          { id: 'recall', icon: 'fa-clock-rotate-left', label: 'recall' },
          { id: 'search', icon: icons.search, label: 'search by name' },
        ].map(tab => (
          <button key={tab.id} onClick={() => switchMode(tab.id)} style={{
            flex: 1, padding: '8px 12px', border: 'none', borderRadius: 'var(--roundness)', fontSize: 12, fontWeight: 600,
            cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            background: mode === tab.id ? '#fff' : 'transparent',
            color: mode === tab.id ? 'var(--text-color)' : 'var(--sub-color)',
            boxShadow: mode === tab.id ? '0 1px 3px rgba(0,0,0,0.08)' : 'none',
          }}>
            <Fa icon={tab.icon} style={{ fontSize: 10 }} /> {tab.label}
          </button>
        ))}
      </div>

      {/* llm status badge. only relevant for recall. */}
      {mode === 'recall' && llmStatus && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14, fontSize: 11, color: llmStatus.enabled ? '#0F6E56' : '#BA7517' }}>
          <span style={{ width: 6, height: 6, borderRadius: '50%', background: llmStatus.enabled ? '#25D366' : '#f59e0b' }}/>
          {llmStatus.enabled ? `ai powered by ${llmStatus.provider}` : 'ai not configured, keyword fallback only'}
        </div>
      )}

      {/* Search bar */}
      <div style={{ position: 'relative', marginBottom: 28 }}>
        <div style={{ position: 'relative' }}>
          {mode === 'search' ? (
            <>
              <Fa icon={icons.search} style={{ position: 'absolute', left: 18, top: 18, fontSize: 15, color: '#666', pointerEvents: 'none', zIndex: 1 }} />
              <input ref={inputRef} value={query} onChange={handleChange} onKeyDown={handleKeyDown}
                onFocus={() => suggestions.length > 0 && mode === 'search' && setShowSuggestions(true)}
                onBlur={() => setTimeout(() => setShowSuggestions(false), 200)}
                placeholder={placeholders[mode]}
                style={{
                  width: '100%', padding: '16px 18px 16px 46px', fontSize: 15,
                  background: '#fff', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)',
                  color: '#1a1a1a', outline: 'none', transition: 'border-color 0.15s',
                  fontFamily: 'inherit',
                }}
                onFocusCapture={e => e.target.style.borderColor = '#2563eb'}
                onBlurCapture={e => e.target.style.borderColor = '#ccc'}
              />
            </>
          ) : (
            <div style={{
              position: 'relative',
              display: 'flex', alignItems: 'flex-end', gap: 10,
              background: '#fff',
              border: '1.5px solid #ccc', borderRadius: 14,
              padding: '12px 12px 12px 18px',
              transition: 'border-color 0.15s, box-shadow 0.25s',
              boxShadow: loading ? '0 0 0 3px rgba(37,211,102,0.15)' : 'none',
            }}
              onFocusCapture={e => e.currentTarget.style.borderColor = 'var(--reyna-accent)'}
              onBlurCapture={e => e.currentTarget.style.borderColor = '#ccc'}
            >
              <Fa icon="fa-clock-rotate-left" style={{ fontSize: 16, color: 'var(--reyna-accent)', marginBottom: 8, flexShrink: 0 }} />
              <textarea ref={inputRef} value={query} onChange={handleChange}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    doRecall(query)
                    return
                  }
                  handleKeyDown(e)
                }}
                onInput={(e) => { e.target.style.height = 'auto'; e.target.style.height = Math.min(e.target.scrollHeight, 280) + 'px' }}
                rows={1}
                placeholder={placeholders[mode] || placeholders.recall}
                style={{
                  flex: 1, minWidth: 0,
                  padding: '6px 4px', fontSize: 15,
                  background: 'transparent', border: 'none',
                  color: '#1a1a1a', outline: 'none',
                  resize: 'none',
                  overflowY: 'hidden',
                  minHeight: 24, maxHeight: 280,
                  fontFamily: 'inherit', lineHeight: 1.55,
                }}
              />
              <button onClick={() => doRecall(query)}
                disabled={!query.trim() || loading}
                style={{
                  ...btnPrimary,
                  background: 'var(--reyna-accent)', border: '1.5px solid var(--reyna-accent)',
                  padding: '8px 16px', fontSize: 13,
                  opacity: (!query.trim() || loading) ? 0.5 : 1,
                  cursor: (!query.trim() || loading) ? 'not-allowed' : 'pointer',
                  flexShrink: 0, alignSelf: 'flex-end',
                }}>
                <Fa icon="fa-clock-rotate-left" style={{ fontSize: 11 }} /> use recall
              </button>
            </div>
          )}
          {/* Autocomplete */}
          {showSuggestions && suggestions.length > 0 && mode === 'search' && (
            <div style={{ position: 'absolute', top: '100%', left: 0, right: 0, ...cardStyle, marginTop: 4, zIndex: 50, overflow: 'hidden' }}>
              {suggestions.slice(0, 6).map((name, i) => (
                <div key={i} onClick={() => selectSuggestion(name)} style={{
                  padding: '8px 14px', cursor: 'pointer', fontSize: 12,
                  background: i === selectedSuggestion ? 'var(--card-hover)' : 'var(--card-bg)',
                  borderBottom: i < suggestions.length - 1 ? '1px solid var(--card-border)' : 'none',
                  display: 'flex', alignItems: 'center', gap: 8, color: 'var(--text-color)',
                }}
                  onMouseEnter={e => e.currentTarget.style.background = 'var(--card-hover)'}
                  onMouseLeave={e => e.currentTarget.style.background = i === selectedSuggestion ? 'var(--card-hover)' : 'var(--card-bg)'}
                >
                  <Fa icon={icons.file} style={{ fontSize: 13, color: 'var(--sub-color)' }} />
                  <span style={{ fontWeight: 500 }}>{name}</span>
                  {i === 0 && <span style={{ marginLeft: 'auto', fontSize: 9, color: 'var(--sub-color)', background: 'var(--bg-color)', padding: '2px 6px', borderRadius: 'var(--roundness)' }}>tab</span>}
                </div>
              ))}
            </div>
          )}
        </div>
        {loading && mode === 'search' && (
          <Fa icon={icons.loading} spin style={{ position: 'absolute', right: 14, top: 18, fontSize: 13, color: 'var(--sub-color)' }} />
        )}
      </div>

      {/* ── Unified Recall thread — merged NLP retrieval + Q&A ──
          Every turn renders:
            (1) the user's question bubble
            (2) a row of file cards (who/when/subject) retrieved for the turn
            (3) Reyna's answer, with source citations
          Follow-up questions thread automatically (see isQAFollowup). */}
      {mode === 'recall' && recallConversation.length > 0 && (
        <div style={{ marginBottom: 20 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
            <div style={{ fontSize: 11, fontWeight: 700, color: 'var(--sub-color)', letterSpacing: 1, textTransform: 'uppercase', display: 'flex', alignItems: 'center', gap: 8 }}>
              <Fa icon="fa-clock-rotate-left" style={{ fontSize: 11, color: 'var(--reyna-accent)' }} />
              recall thread
            </div>
            <button onClick={clearQA} style={{
              ...btnBase, fontSize: 11, padding: '4px 12px', color: 'var(--sub-color)',
            }}>
              <Fa icon="fa-arrow-rotate-left" style={{ fontSize: 9 }} /> new conversation
            </button>
          </div>
          {recallConversation.map((turn, i) => (
            <div key={i} style={{ marginBottom: 22 }}>
              {/* user question */}
              <div style={{
                background: 'rgba(37,211,102,0.08)',
                border: '1px solid rgba(37,211,102,0.25)',
                borderRadius: 14, padding: '10px 16px', marginBottom: 10,
                maxWidth: '85%', marginLeft: 'auto',
                fontSize: 14, color: '#1a1a1a', lineHeight: 1.55,
                whiteSpace: 'pre-wrap', wordBreak: 'break-word',
              }}>
                {turn.question}
              </div>

              {/* Files retrieved — unified list of DB files (turn.files, have
                  full metadata + can preview/download/delete) AND Drive matches
                  (turn.drive_sources, files sitting in the user's Drive that
                  the bot never captured — still useful, open directly in Drive).
                  Earlier bug: drive_sources were returned by the backend but
                  never rendered, so searches that only matched Drive looked
                  like "no files found" in the UI. */}
              {!turn.pending && (() => {
                const dbFiles = turn.files || []
                const dbNames = new Set(dbFiles.map(f => f.file_name))
                const driveOnly = (turn.drive_sources || []).filter(m => !dbNames.has(m.file_name))
                const total = dbFiles.length + driveOnly.length
                return total > 0
              })() && (
                <div style={{ marginBottom: 10 }}>
                  <div style={{ fontSize: 10, fontWeight: 700, color: 'var(--sub-color)', letterSpacing: 0.8, textTransform: 'uppercase', marginBottom: 6 }}>
                    {(() => {
                      const dbFiles = turn.files || []
                      const dbNames = new Set(dbFiles.map(f => f.file_name))
                      const driveOnly = (turn.drive_sources || []).filter(m => !dbNames.has(m.file_name))
                      const total = dbFiles.length + driveOnly.length
                      return `found ${total} file${total !== 1 ? 's' : ''}`
                    })()}
                  </div>
                  <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))' }}>
                    {/* DB files with full metadata + action buttons */}
                    {(turn.files || []).slice(0, 6).map((f, j) => (
                      <div key={'db' + j} style={{
                        ...cardStyle, padding: '10px 12px',
                        display: 'flex', gap: 10, alignItems: 'flex-start',
                        transition: 'transform 0.12s, border-color 0.12s',
                      }}
                        onMouseEnter={e => { e.currentTarget.style.transform = 'translateY(-1px)'; e.currentTarget.style.borderColor = 'var(--reyna-accent)' }}
                        onMouseLeave={e => { e.currentTarget.style.transform = 'translateY(0)'; e.currentTarget.style.borderColor = '#ccc' }}
                      >
                        <IconBox icon={fileIconClass(f.mime_type, f.file_name)} color="var(--reyna-accent)" bg="rgba(37,211,102,0.08)" size={32} iconSize={12} />
                        <div style={{ flex: 1, minWidth: 0, cursor: 'pointer' }} onClick={() => openPreview(f)}>
                          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-color)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{f.file_name}</div>
                          <div style={{ fontSize: 11, color: 'var(--sub-color)', marginTop: 2 }}>
                            {f.shared_by_name || 'unknown'} · {timeAgo(f.created_at)}
                          </div>
                          {f.subject && <div style={{ fontSize: 10, color: 'var(--reyna-accent)', marginTop: 2, fontWeight: 600 }}>{f.subject}</div>}
                        </div>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 4, flexShrink: 0 }}>
                          <button onClick={(e) => { e.stopPropagation(); openPreview(f) }} title="preview" style={{ ...btnBase, padding: '4px 7px', fontSize: 10 }}><Fa icon={icons.preview} style={{ fontSize: 10 }} /></button>
                          <button onClick={(e) => { e.stopPropagation(); downloadFile(f) }} title="download" style={{ ...btnBase, padding: '4px 7px', fontSize: 10 }}><Fa icon={icons.download} style={{ fontSize: 10 }} /></button>
                          <button onClick={(e) => { e.stopPropagation(); confirmDelete(f) }} title="delete" style={{ ...btnDanger, padding: '4px 7px', fontSize: 10 }}><Fa icon={icons.delete} style={{ fontSize: 10 }} /></button>
                        </div>
                      </div>
                    ))}
                    {/* Drive-only matches — only render the ones we don't
                        already have as DB files (preview/download/delete
                        handles those better). Drive cards are useful only
                        when a file exists in Drive but never made it into
                        Reyna's DB. */}
                    {(turn.drive_sources || [])
                      .filter(m => !(turn.files || []).some(f => f.file_name === m.file_name))
                      .slice(0, 6)
                      .map((m, j) => (
                      <div key={'dr' + j} style={{
                        ...cardStyle, padding: '10px 12px',
                        display: 'flex', gap: 10, alignItems: 'flex-start',
                        transition: 'transform 0.12s, border-color 0.12s',
                      }}
                        onMouseEnter={e => { e.currentTarget.style.transform = 'translateY(-1px)'; e.currentTarget.style.borderColor = 'var(--reyna-accent)' }}
                        onMouseLeave={e => { e.currentTarget.style.transform = 'translateY(0)'; e.currentTarget.style.borderColor = '#ccc' }}
                      >
                        <IconBox icon={fileIconClass(m.mime_type, m.file_name)} color="#888" bg="rgba(0,0,0,0.04)" size={32} iconSize={12} />
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-color)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m.file_name}</div>
                          <div style={{ fontSize: 11, color: 'var(--sub-color)', marginTop: 2 }}>
                            in drive · {m.folder_name || 'root'}
                          </div>
                          {m.sender_name && <div style={{ fontSize: 10, color: 'var(--reyna-accent)', marginTop: 2, fontWeight: 600 }}>by {m.sender_name}</div>}
                        </div>
                        <a href={`https://drive.google.com/file/d/${m.file_id}/view`} target="_blank" rel="noreferrer"
                          title="open in drive"
                          style={{ ...btnBase, padding: '4px 7px', fontSize: 10, textDecoration: 'none', alignSelf: 'flex-start' }}>
                          <Fa icon={icons.link} style={{ fontSize: 10 }} />
                        </a>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* reyna answer */}
              <div style={{ ...cardStyle, padding: '14px 18px', maxWidth: '96%' }}>
                <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--reyna-accent)', marginBottom: 6, display: 'flex', alignItems: 'center', gap: 6 }}>
                  <Fa icon="fa-clock-rotate-left" style={{ fontSize: 11 }} /> reyna's recall
                </div>
                {turn.pending ? (
                  <div style={{ fontSize: 13, color: 'var(--sub-color)', display: 'flex', alignItems: 'center', gap: 8 }}>
                    <Fa icon={icons.loading} spin style={{ fontSize: 12 }} />
                    recalling…
                  </div>
                ) : (
                  <>
                    <div style={{ fontSize: 14, color: 'var(--text-color)' }}>
                      {renderMarkdown(cleanReply(turn.answer))}
                    </div>
                    {turn.sources && turn.sources.length > 0 && (
                      <div style={{ marginTop: 10, paddingTop: 8, borderTop: '1px solid var(--card-border)', fontSize: 11, color: 'var(--sub-color)' }}>
                        <Fa icon={icons.file} style={{ fontSize: 9, marginRight: 4 }} />
                        sources: {turn.sources.join(', ')}
                      </div>
                    )}
                  </>
                )}
              </div>
            </div>
          ))}
          {/* Follow-up call-to-action — unchanged shortcut chips + "ask another" button */}
          {recallConversation.length > 0 && !recallConversation[recallConversation.length - 1].pending && (
            <div style={{
              display: 'flex', gap: 8, marginTop: 4, marginBottom: 8, flexWrap: 'wrap',
              alignItems: 'center',
            }}>
              <span style={{ fontSize: 11, color: 'var(--sub-color)', marginRight: 4 }}>
                ask a follow-up:
              </span>
              {['tell me more', 'in simpler words', 'give an example', 'why?'].map(s => (
                <button key={s} onClick={() => { setQuery(s); inputRef.current?.focus(); window.scrollTo({ top: 0, behavior: 'smooth' }) }}
                  style={{
                    padding: '4px 10px', background: '#fff', border: '1px solid #ddd',
                    borderRadius: 14, fontSize: 11, cursor: 'pointer',
                    color: '#555', fontWeight: 500,
                  }}>{s}</button>
              ))}
              <button onClick={() => { inputRef.current?.focus(); window.scrollTo({ top: 0, behavior: 'smooth' }) }}
                style={{
                  padding: '4px 10px', background: 'var(--reyna-accent)', border: '1px solid var(--reyna-accent)',
                  borderRadius: 14, fontSize: 11, cursor: 'pointer',
                  color: '#fff', fontWeight: 600, marginLeft: 'auto',
                }}>
                <Fa icon="fa-arrow-up" style={{ fontSize: 9 }} /> ask another question
              </button>
            </div>
          )}
          <div ref={qaEndRef} />
        </div>
      )}

      {/* ── OLD UI — kept commented for reference (NLP parsed tags + separate Q&A thread).
          The render above replaces both. Uncomment only if you need to A/B compare. */}
      {false && mode === 'nlp' && (
        <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
          {[{ k: 'WHO' }, { k: 'WHAT' }, { k: 'WHEN' }, { k: 'WHY' }].map(t => (
            <span key={t.k} style={{ padding: '4px 12px', borderRadius: 20, fontSize: 11, background: tagColors[t.k] + '15', color: tagColors[t.k] }}>{t.k}</span>
          ))}
        </div>
      )}

      {/* Empty state */}
      {results === null && mode === 'search' ? (
        <div style={{ textAlign: 'center', padding: 60, color: 'var(--sub-color)' }}>
          <Fa icon={icons.search} style={{ fontSize: 48, marginBottom: 14, color: 'var(--sub-color)', opacity: 0.3 }} />
          <p style={{ fontSize: 14 }}>search for files across all your groups</p>
          <p style={{ fontSize: 11, marginTop: 6, color: 'var(--sub-color)' }}>start typing to search by filename. results update live.</p>
          <div style={{ marginTop: 16, display: 'flex', gap: 6, justifyContent: 'center', flexWrap: 'wrap' }}>
            {['DSA', 'notes', 'PYQ', 'assignment', 'lab'].map(q => (
              <button key={q} onClick={() => { setQuery(q); doLiveSearch(q); fetchSuggestions(q) }} style={{
                padding: '5px 12px', background: 'var(--reyna-accent-dim)', border: '1px solid rgba(37,211,102,0.2)', borderRadius: 'var(--roundness)',
                fontSize: 11, cursor: 'pointer', color: 'var(--reyna-accent)', fontWeight: 400,
              }}>{q}</button>
            ))}
          </div>
        </div>
      ) : results === null && mode === 'recall' && recallConversation.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '40px 20px 60px', color: 'var(--sub-color)' }}>
          <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 18 }}>
            <Fa icon="fa-clock-rotate-left" style={{ fontSize: 64, color: 'var(--reyna-accent)', opacity: 0.35 }} />
          </div>
          <p style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-color)', marginBottom: 4 }}>
            one recall, everything your notes know
          </p>
          <p style={{ fontSize: 13, maxWidth: 460, margin: '0 auto' }}>
            ask by meaning and reyna finds the files, summarises them properly, and answers in one reply. memories you've taught her shape every answer.
          </p>
          <div style={{ marginTop: 20, display: 'flex', flexDirection: 'column', gap: 6, maxWidth: 460, margin: '20px auto 0' }}>
            {[
              'summarize the dbms notes mohit sent last week',
              'what does the os syllabus say about scheduling?',
              'find anything about compiler design',
              'explain transactions from our notes',
            ].map(q => (
              <button key={q} onClick={() => { setQuery(q); doRecall(q) }} style={{
                padding: '9px 14px', background: '#fff',
                border: '1px solid #eee', borderLeft: '3px solid var(--reyna-accent)',
                borderRadius: 'var(--roundness)',
                fontSize: 12, cursor: 'pointer', color: '#444', textAlign: 'left',
                transition: 'background 0.12s, border-color 0.12s',
              }}
                onMouseEnter={e => { e.currentTarget.style.background = '#f5fdf8' }}
                onMouseLeave={e => { e.currentTarget.style.background = '#fff' }}
              >{q}</button>
            ))}
          </div>
        </div>
      ) : results !== null && results.length === 0 ? (
        <div style={{ textAlign: 'center', padding: 60, color: 'var(--sub-color)' }}>
          <Fa icon="fa-inbox" style={{ fontSize: 40, marginBottom: 12, opacity: 0.3 }} />
          <p style={{ fontSize: 14 }}>nothing found for "{query}"</p>
        </div>
      ) : results && results.length > 0 && (
        <div>
          <p style={{ fontSize: 13, color: 'var(--sub-color)', marginBottom: 14 }}>
            {results.length} result{results.length !== 1 ? 's' : ''}
            {mode === 'search' && ` for "${query}"`}
            {isContentSearch && <span style={{ color: '#8b5cf6', fontWeight: 500 }}> (content search)</span>}
          </p>
          <div style={{ ...cardStyle, overflow: 'hidden' }}>
            {results.map((f, i) => (
              <div key={i} style={{
                display: 'flex', alignItems: 'center', gap: 14, padding: '12px 16px',
                borderBottom: i < results.length - 1 ? '1px solid var(--card-border)' : 'none',
                transition: 'background 0.15s',
              }}
                onMouseEnter={e => e.currentTarget.style.background = 'var(--card-hover)'}
                onMouseLeave={e => e.currentTarget.style.background = 'var(--card-bg)'}
              >
                <IconBox icon={fileIconClass(f.mime_type, f.file_name)} color="var(--main-color)" bg="rgba(37,211,102,0.08)" size={36} iconSize={14} />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)' }}>{f.file_name}</div>
                  <div style={{ fontSize: 13, color: 'var(--sub-color)', display: 'flex', gap: 10, marginTop: 2, flexWrap: 'wrap' }}>
                    <span>{f.subject || 'General'}</span><span>v{f.version}</span><span>{formatBytes(f.file_size)}</span>
                    <span>by {f.shared_by_name || 'unknown'}</span><span>{timeAgo(f.created_at)}</span>
                  </div>
                  {f.content_summary && <div style={{ fontSize: 11, color: '#8b5cf6', marginTop: 2 }}><Fa icon="fa-brain" style={{ fontSize: 9, marginRight: 4 }} />{f.content_summary}</div>}
                </div>
                <div style={{ display: 'flex', gap: 3, flexShrink: 0 }}>
                  <button onClick={() => openPreview(f)} title="preview" style={{ ...btnBase, padding: '5px 8px' }}><Fa icon={icons.preview} style={{ fontSize: 11 }} /></button>
                  <button onClick={() => downloadFile(f)} title="download" style={{ ...btnBase, padding: '5px 8px' }}><Fa icon={icons.download} style={{ fontSize: 11 }} /></button>
                  <button onClick={() => confirmDelete(f)} title="delete" style={{ ...btnDanger, padding: '5px 8px' }}><Fa icon={icons.delete} style={{ fontSize: 11 }} /></button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Preview */}
      {previewFile && createPortal(
        <div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.8)', display: 'flex', flexDirection: 'column', zIndex: 99999 }}>
          <div style={{ padding: '10px 20px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: '#fff', borderBottom: '1px solid var(--card-border)', flexShrink: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <Fa icon={fileIconClass(previewFile.mime_type, previewFile.file_name)} style={{ fontSize: 18, color: 'var(--main-color)' }} />
              <div><div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-color)' }}>{previewFile.file_name}</div><div style={{ fontSize: 13, color: 'var(--sub-color)' }}>{formatBytes(previewFile.file_size)} · by {previewFile.shared_by_name || 'unknown'}</div></div>
            </div>
            <div style={{ display: 'flex', gap: 6 }}>
              {previewData?.driveUrl && <a href={previewData.driveUrl} target="_blank" rel="noreferrer" style={{ ...btnBase, textDecoration: 'none' }}><Fa icon={icons.link} style={{ fontSize: 9 }} /> open in drive</a>}
              <button onClick={() => downloadFile(previewFile)} style={btnBase}><Fa icon={icons.download} style={{ fontSize: 10 }} /> download</button>
              <button onClick={() => { setPreviewFile(null); setPreviewData(null) }} style={{ ...btnBase, borderRadius: '50%', width: 32, height: 32, padding: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Fa icon={icons.close} style={{ fontSize: 12 }} /></button>
            </div>
          </div>
          <div style={{ flex: 1, overflow: 'auto', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            {previewLoading ? <div style={{ color: 'var(--sub-color)' }}><Fa icon={icons.loading} spin /> loading...</div>
            : previewData?.type === 'drive' ? <iframe src={previewData.url} style={{ width: '100%', height: '100%', border: 'none' }} title="preview" />
            : previewData?.type === 'image' ? <img src={previewData.url} alt="" style={{ maxWidth: '95%', maxHeight: '95%', borderRadius: 'var(--roundness)', objectFit: 'contain' }} />
            : previewData?.type === 'text' ? <div style={{ width: '100%', height: '100%', overflow: 'auto', padding: '20px 28px', background: '#fff' }}><pre style={{ fontFamily: 'var(--font-mono)', fontSize: 12, lineHeight: 1.7, whiteSpace: 'pre-wrap', color: 'var(--text-color)', margin: 0 }}>{previewData.content}</pre></div>
            : <div style={{ color: 'var(--sub-color)', textAlign: 'center' }}><Fa icon={icons.file} style={{ fontSize: 48, color: '#1a1a1a', marginBottom: 12 }} /><p>no preview available</p><button onClick={() => downloadFile(previewFile)} style={{ ...btnPrimary, marginTop: 12 }}><Fa icon={icons.download} /> download</button></div>}
          </div>
        </div>, document.body
      )}

      {/* Delete */}
      {deleteTarget && createPortal(
        <div onClick={() => setDeleteTarget(null)} style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 99999 }}>
          <div onClick={e => e.stopPropagation()} style={{ background: '#fff', borderRadius: 'var(--roundness)', padding: 28, maxWidth: 420, width: '90vw', border: '1.5px solid #ccc', boxShadow: '0 25px 60px rgba(0,0,0,0.15), 0 8px 20px rgba(0,0,0,0.1)' }}>
            <h3 style={{ fontSize: 20, fontWeight: 700, letterSpacing: -0.3, color: 'var(--error-color)', marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}><Fa icon={icons.warning} style={{ fontSize: 14 }} /> delete file?</h3>
            <p style={{ fontSize: 14, color: 'var(--text-secondary, #555)', lineHeight: 1.6, marginBottom: 24 }}>delete <strong style={{ color: 'var(--text-color)' }}>{deleteTarget.file_name}</strong>? moves to drive trash (30 day recovery).</p>
            <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
              <button onClick={() => setDeleteTarget(null)} style={btnBase}>cancel</button>
              <button onClick={doDelete} disabled={deleting} style={{ ...btnDanger, opacity: deleting ? 0.6 : 1 }}>{deleting ? 'deleting...' : 'delete'}</button>
            </div>
          </div>
        </div>, document.body
      )}
      </div>{/* ← closes the zIndex:1 content wrapper */}
    </div>
  )
}
