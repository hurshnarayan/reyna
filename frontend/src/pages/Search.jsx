import { useState, useEffect, useRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { api, getToken } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa, icons, fileIconClass, IconBox } from '../components/icons'

function formatBytes(b) { if (b < 1024) return b + ' B'; if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'; return (b / 1048576).toFixed(1) + ' MB' }
function timeAgo(d) { const s = Math.floor((Date.now() - new Date(d).getTime()) / 1000); if (s < 60) return 'just now'; if (s < 3600) return Math.floor(s / 60) + 'm ago'; if (s < 86400) return Math.floor(s / 3600) + 'h ago'; return Math.floor(s / 86400) + 'd ago' }

const btnBase = { background: '#fff', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)', padding: '8px 16px', fontSize: 14, fontWeight: 600, cursor: 'pointer', color: 'var(--text-secondary, #555)', transition: 'all 0.15s', display: 'inline-flex', alignItems: 'center', gap: 6 }
const btnPrimary = { ...btnBase, background: 'var(--reyna-accent)', color: '#fff', border: '1.5px solid var(--reyna-accent)', fontWeight: 700 }
const btnDanger = { ...btnBase, background: '#fef2f2', color: 'var(--error-color)', border: '1.5px solid rgba(220,38,38,0.15)' }
const cardStyle = { background: 'var(--card-bg)', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)', boxShadow: 'var(--card-shadow)' }

export default function Search() {
  const [mode, setMode] = useState('search') // 'search' | 'nlp' | 'qa'
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
  // NLP state
  const [nlpParsed, setNlpParsed] = useState(null)
  const [nlpReply, setNlpReply] = useState('')
  // Q&A state
  const [qaAnswer, setQaAnswer] = useState(null)
  const [qaHistory, setQaHistory] = useState([])
  const [llmStatus, setLlmStatus] = useState(null)

  const inputRef = useRef(null)
  const debounceRef = useRef(null)

  useEffect(() => { api.llmStatus().then(s => setLlmStatus(s)) }, [])

  const doLiveSearch = useCallback(async (q) => {
    if (!q.trim() || q.trim().length < 2) { setResults(null); return }
    setLoading(true); notify.showLoader()
    const files = await api.search(q.trim()); setResults(files || []); setLoading(false); notify.hideLoader()
  }, [])

  const doNLPSearch = async (q) => {
    if (!q.trim()) return
    setLoading(true); notify.showLoader(); setNlpParsed(null); setNlpReply('')
    const resp = await api.nlpRetrieve(q.trim())
    setResults(resp?.files || [])
    setNlpParsed(resp?.parsed_query || null)
    setNlpReply(resp?.reply || '')
    setLoading(false); notify.hideLoader()
  }

  const doQA = async (q) => {
    if (!q.trim()) return
    setLoading(true); notify.showLoader(); setQaAnswer(null)
    const resp = await api.notesQA(q.trim())
    const entry = { question: q, answer: resp?.answer || 'No answer', sources: resp?.sources || [] }
    setQaAnswer(entry)
    setQaHistory(prev => [entry, ...prev])
    setLoading(false); notify.hideLoader()
    setQuery('')
  }

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
      else if (mode === 'nlp') { doNLPSearch(query) }
      else if (mode === 'qa') { doQA(query) }
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
  const doDelete = async () => { if (!deleteTarget) return; setDeleting(true); await api.deleteFile(deleteTarget.id); notify.success(`Deleted ${deleteTarget.file_name}`); setDeleteTarget(null); setDeleting(false); doLiveSearch(query) }

  const switchMode = (m) => { setMode(m); setResults(null); setNlpParsed(null); setNlpReply(''); setQaAnswer(null); setQuery(''); setSuggestions([]); setShowSuggestions(false) }

  const placeholders = {
    search: 'search files by name...',
    nlp: '"What did Rahul share about drones last week?"',
    qa: '"Summarize Chapter 5" or "Explain photosynthesis from our notes"',
  }

  const tagColors = { WHO: '#D85A30', WHAT: '#7F77DD', WHEN: '#1D9E75', WHY: '#BA7517' }

  return (
    <div style={{ padding: '32px 40px', maxWidth: 800 }} className="fade-in">
      <h1 style={{ fontSize: 44, fontWeight: 900, letterSpacing: -0.5, marginBottom: 6, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 10 }}>
        <Fa icon={icons.search} style={{ fontSize: 32 }} /> search
      </h1>
      <p style={{ fontSize: 15, color: '#888', marginBottom: 20 }}>find files, ask questions, or retrieve with natural language.</p>

      {/* Mode tabs */}
      <div style={{ display: 'flex', gap: 4, marginBottom: 20, background: '#f5f5f5', borderRadius: 'var(--roundness)', padding: 3 }}>
        {[
          { id: 'search', icon: icons.search, label: 'Search' },
          { id: 'nlp', icon: 'fa-comments', label: 'NLP Retrieval' },
          { id: 'qa', icon: 'fa-graduation-cap', label: 'Notes Q&A' },
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

      {/* LLM status badge */}
      {(mode === 'nlp' || mode === 'qa') && llmStatus && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14, fontSize: 11, color: llmStatus.enabled ? '#0F6E56' : '#BA7517' }}>
          <span style={{ width: 6, height: 6, borderRadius: '50%', background: llmStatus.enabled ? '#25D366' : '#f59e0b' }}/>
          {llmStatus.enabled ? `AI powered by ${llmStatus.provider}` : 'AI not configured — keyword fallback only'}
        </div>
      )}

      {/* Search bar */}
      <div style={{ position: 'relative', marginBottom: 28 }}>
        <div style={{ position: 'relative' }}>
          <Fa icon={mode === 'qa' ? 'fa-graduation-cap' : mode === 'nlp' ? 'fa-comments' : icons.search} style={{ position: 'absolute', left: 14, top: '50%', transform: 'translateY(-50%)', fontSize: 14, color: 'var(--sub-color)', pointerEvents: 'none' }} />
          <input ref={inputRef} value={query} onChange={handleChange} onKeyDown={handleKeyDown}
            onFocus={() => suggestions.length > 0 && mode === 'search' && setShowSuggestions(true)}
            onBlur={() => setTimeout(() => setShowSuggestions(false), 200)}
            placeholder={placeholders[mode]}
            style={{
              width: '100%', padding: '14px 14px 14px 40px', fontSize: 14,
              background: '#fff', border: '1.5px solid #ccc', borderRadius: 'var(--roundness)',
              color: 'var(--text-color)', outline: 'none', transition: 'border-color 0.15s',
            }}
            onFocusCapture={e => e.target.style.borderColor = '#2563eb'}
            onBlurCapture={e => e.target.style.borderColor = '#ccc'}
          />
          {/* Submit button for NLP/QA modes */}
          {(mode === 'nlp' || mode === 'qa') && (
            <button onClick={() => mode === 'nlp' ? doNLPSearch(query) : doQA(query)}
              style={{ position: 'absolute', right: 6, top: '50%', transform: 'translateY(-50%)', ...btnPrimary, padding: '6px 14px', fontSize: 12 }}>
              <Fa icon={mode === 'qa' ? 'fa-paper-plane' : icons.search} style={{ fontSize: 10 }} /> {mode === 'qa' ? 'Ask' : 'Search'}
            </button>
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
        {loading && <Fa icon={icons.loading} spin style={{ position: 'absolute', right: mode === 'search' ? 14 : 100, top: '50%', transform: 'translateY(-50%)', fontSize: 13, color: 'var(--sub-color)' }} />}
      </div>

      {/* NLP parsed query display */}
      {mode === 'nlp' && nlpParsed && (
        <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
          {[
            { k: 'WHO', v: nlpParsed.who },
            { k: 'WHAT', v: nlpParsed.what },
            { k: 'WHEN', v: nlpParsed.when },
            { k: 'WHY', v: nlpParsed.why },
          ].filter(t => t.v).map(t => (
            <span key={t.k} style={{
              padding: '4px 12px', borderRadius: 20, fontSize: 11, fontWeight: 600,
              background: tagColors[t.k] + '15', color: tagColors[t.k],
            }}>{t.k}: {t.v}</span>
          ))}
        </div>
      )}
      {mode === 'nlp' && nlpReply && (
        <div style={{ ...cardStyle, padding: '12px 16px', marginBottom: 16, fontSize: 13, lineHeight: 1.7, color: 'var(--text-color)', whiteSpace: 'pre-line' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
            <Fa icon="fa-crown" style={{ fontSize: 11, color: 'var(--reyna-accent)' }} />
            <strong style={{ fontSize: 12 }}>Reyna</strong>
          </div>
          {nlpReply}
        </div>
      )}

      {/* Q&A answer display */}
      {mode === 'qa' && qaAnswer && (
        <div style={{ ...cardStyle, padding: '16px 20px', marginBottom: 20 }}>
          <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--reyna-accent)', marginBottom: 4, display: 'flex', alignItems: 'center', gap: 6 }}>
            <Fa icon="fa-crown" style={{ fontSize: 11 }} /> Reyna
          </div>
          <div style={{ fontSize: 13, lineHeight: 1.8, color: 'var(--text-color)', whiteSpace: 'pre-line' }}>
            {qaAnswer.answer}
          </div>
          {qaAnswer.sources.length > 0 && (
            <div style={{ marginTop: 10, paddingTop: 10, borderTop: '1px solid var(--card-border)', fontSize: 11, color: 'var(--sub-color)' }}>
              <Fa icon={icons.file} style={{ fontSize: 9, marginRight: 4 }} />
              Sources: {qaAnswer.sources.join(', ')}
            </div>
          )}
        </div>
      )}

      {/* Q&A history */}
      {mode === 'qa' && qaHistory.length > 1 && (
        <div style={{ marginBottom: 20 }}>
          <div style={{ fontSize: 11, fontWeight: 700, color: 'var(--sub-color)', letterSpacing: 1, textTransform: 'uppercase', marginBottom: 8 }}>Previous questions</div>
          {qaHistory.slice(1, 4).map((h, i) => (
            <div key={i} style={{ ...cardStyle, padding: '10px 14px', marginBottom: 6, fontSize: 12, cursor: 'pointer' }}
              onClick={() => { setQuery(h.question); setQaAnswer(h) }}>
              <strong>{h.question}</strong>
              <div style={{ color: 'var(--sub-color)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{h.answer.slice(0, 100)}...</div>
            </div>
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
      ) : results === null && mode === 'nlp' ? (
        <div style={{ textAlign: 'center', padding: 60, color: 'var(--sub-color)' }}>
          <Fa icon="fa-comments" style={{ fontSize: 48, marginBottom: 14, opacity: 0.3 }} />
          <p style={{ fontSize: 14 }}>ask naturally, like you would ask a friend</p>
          <div style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 6, maxWidth: 400, margin: '16px auto 0' }}>
            {[
              'What did Priya upload yesterday?',
              'Do we have any OS notes?',
              'Find the compiler lab manual',
              "What's new since Monday?",
            ].map(q => (
              <button key={q} onClick={() => { setQuery(q); doNLPSearch(q) }} style={{
                padding: '8px 14px', background: '#fff', border: '1px solid #eee', borderRadius: 'var(--roundness)',
                fontSize: 12, cursor: 'pointer', color: 'var(--text-secondary)', textAlign: 'left',
              }}>{q}</button>
            ))}
          </div>
        </div>
      ) : results === null && mode === 'qa' ? (
        <div style={{ textAlign: 'center', padding: 60, color: 'var(--sub-color)' }}>
          <Fa icon="fa-graduation-cap" style={{ fontSize: 48, marginBottom: 14, opacity: 0.3 }} />
          <p style={{ fontSize: 14 }}>ask anything from your shared notes</p>
          <div style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 6, maxWidth: 400, margin: '16px auto 0' }}>
            {[
              'Summarize Chapter 5',
              'What are the types of scheduling algorithms?',
              'Explain photosynthesis from our bio notes',
              'What did the teacher say about integrals?',
            ].map(q => (
              <button key={q} onClick={() => { setQuery(q); doQA(q) }} style={{
                padding: '8px 14px', background: '#fff', border: '1px solid #eee', borderRadius: 'var(--roundness)',
                fontSize: 12, cursor: 'pointer', color: 'var(--text-secondary)', textAlign: 'left',
              }}>{q}</button>
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
                  <button onClick={() => openPreview(f)} title="Preview" style={{ ...btnBase, padding: '5px 8px' }}><Fa icon={icons.preview} style={{ fontSize: 11 }} /></button>
                  <button onClick={() => downloadFile(f)} title="Download" style={{ ...btnBase, padding: '5px 8px' }}><Fa icon={icons.download} style={{ fontSize: 11 }} /></button>
                  <button onClick={() => confirmDelete(f)} title="Delete" style={{ ...btnDanger, padding: '5px 8px' }}><Fa icon={icons.delete} style={{ fontSize: 11 }} /></button>
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
            : previewData?.type === 'drive' ? <iframe src={previewData.url} style={{ width: '100%', height: '100%', border: 'none' }} title="Preview" />
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
            <h3 style={{ fontSize: 20, fontWeight: 700, letterSpacing: -0.3, color: 'var(--error-color)', marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}><Fa icon={icons.warning} style={{ fontSize: 14 }} /> Delete File?</h3>
            <p style={{ fontSize: 14, color: 'var(--text-secondary, #555)', lineHeight: 1.6, marginBottom: 24 }}>delete <strong style={{ color: 'var(--text-color)' }}>{deleteTarget.file_name}</strong>? moves to drive trash (30 day recovery).</p>
            <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
              <button onClick={() => setDeleteTarget(null)} style={btnBase}>cancel</button>
              <button onClick={doDelete} disabled={deleting} style={{ ...btnDanger, opacity: deleting ? 0.6 : 1 }}>{deleting ? 'deleting...' : 'delete'}</button>
            </div>
          </div>
        </div>, document.body
      )}
    </div>
  )
}
