import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa } from '../components/icons'

// Reyna's Memory — persistent user context. Think "CLAUDE.md, but smarter":
// add a memory, give it a name, choose whether it's always on or only
// recalled when relevant, toggle it off without deleting, or remove it.

export default function Memory() {
  const [memories, setMemories] = useState([])
  const [loading, setLoading] = useState(true)
  const [showAdd, setShowAdd] = useState(false)
  const [editing, setEditing] = useState(null) // memory being edited

  const load = async () => {
    setLoading(true)
    const res = await api.listMemories()
    setMemories(res?.memories || [])
    setLoading(false)
  }
  useEffect(() => { load() }, [])

  const handleToggle = async (m) => {
    const newVal = !m.is_active
    setMemories(prev => prev.map(x => x.id === m.id ? { ...x, is_active: newVal } : x))
    await api.toggleMemory(m.id, newVal)
  }

  const handleDelete = async (m) => {
    if (!confirm(`delete memory "${m.title}"? this cannot be undone.`)) return
    await api.deleteMemory(m.id)
    notify.success(`forgotten: ${m.title}`)
    load()
  }

  return (
    <div style={{ padding: '32px 40px', maxWidth: 900 }} className="fade-in">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20, gap: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          <Fa icon="fa-brain" style={{ fontSize: 42, color: 'var(--reyna-accent)' }} />
          <div>
            <h1 style={{ fontSize: 40, fontWeight: 800, letterSpacing: -0.5, margin: 0, color: 'var(--text-color)', lineHeight: 1.05 }}>
              Reyna's Memory
            </h1>
            <p style={{ fontSize: 14, color: '#888', margin: '4px 0 0 0' }}>
              teach reyna once and she remembers your syllabus, exam schedule, study style, anything
            </p>
          </div>
        </div>
        <button onClick={() => { setEditing(null); setShowAdd(true) }} style={primaryBtn}>
          <Fa icon="fa-plus" style={{ fontSize: 12 }} /> add a memory
        </button>
      </div>

      {loading ? (
        <div style={{ color: '#888', padding: '40px 0', textAlign: 'center' }}>loading memories...</div>
      ) : memories.length === 0 ? (
        <div style={emptyBox}>
          <Fa icon="fa-brain" style={{ fontSize: 32, color: '#ccc', marginBottom: 12 }} />
          <div style={{ fontSize: 16, fontWeight: 600, marginBottom: 6 }}>no memories yet</div>
          <div style={{ fontSize: 13, color: '#888', marginBottom: 16 }}>
            add one and reyna will use it on every recall answer.
          </div>
          <button onClick={() => setShowAdd(true)} style={primaryBtn}>
            <Fa icon="fa-plus" style={{ fontSize: 12 }} /> add your first memory
          </button>
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 12 }}>
          {memories.map(m => (
            <MemoryCard key={m.id}
              memory={m}
              onToggle={() => handleToggle(m)}
              onEdit={() => { setEditing(m); setShowAdd(true) }}
              onDelete={() => handleDelete(m)}
            />
          ))}
        </div>
      )}

      {showAdd && (
        <MemoryModal
          initial={editing}
          onClose={() => { setShowAdd(false); setEditing(null) }}
          onSaved={() => { setShowAdd(false); setEditing(null); load() }}
        />
      )}
    </div>
  )
}

function MemoryCard({ memory, onToggle, onEdit, onDelete }) {
  return (
    <div style={{
      background: '#fff',
      border: '1px solid #e0e0e0',
      borderRadius: 'var(--roundness)',
      padding: 20,
      opacity: memory.is_active ? 1 : 0.55,
      transition: 'opacity 0.2s',
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <h3 style={{ fontSize: 17, fontWeight: 700, color: 'var(--text-color)', margin: 0 }}>{memory.title}</h3>
            {memory.always_include && (
              <span style={pinBadge}><Fa icon="fa-thumbtack" style={{ fontSize: 9 }} /> always on</span>
            )}
            <span style={{ fontSize: 11, color: '#999' }}>
              {memory.source === 'voice' ? 'added by voice' : 'added by paste'}
            </span>
          </div>
          <p style={{ fontSize: 13, color: '#666', lineHeight: 1.6, margin: 0, whiteSpace: 'pre-wrap' }}>
            {memory.content.length > 260 ? memory.content.slice(0, 260) + '…' : memory.content}
          </p>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, alignItems: 'flex-end' }}>
          <ToggleSwitch active={memory.is_active} onClick={onToggle} />
          <div style={{ display: 'flex', gap: 6 }}>
            <button onClick={onEdit} style={iconBtn} title="edit">
              <Fa icon="fa-pen" style={{ fontSize: 11 }} />
            </button>
            <button onClick={onDelete} style={{ ...iconBtn, color: '#b44' }} title="delete">
              <Fa icon="fa-trash" style={{ fontSize: 11 }} />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function ToggleSwitch({ active, onClick }) {
  return (
    <button onClick={onClick} style={{
      width: 44, height: 24, borderRadius: 12, border: 'none', cursor: 'pointer',
      background: active ? 'var(--reyna-accent)' : '#ccc', padding: 0, position: 'relative',
      transition: 'background 0.2s',
    }} aria-label={active ? 'disable memory' : 'enable memory'}>
      <div style={{
        width: 18, height: 18, borderRadius: '50%', background: '#fff',
        position: 'absolute', top: 3, left: active ? 23 : 3,
        transition: 'left 0.2s', boxShadow: '0 1px 3px rgba(0,0,0,0.2)',
      }} />
    </button>
  )
}

function MemoryModal({ initial, onClose, onSaved }) {
  const [title, setTitle] = useState(initial?.title || '')
  const [content, setContent] = useState(initial?.content || '')
  const [alwaysInclude, setAlwaysInclude] = useState(initial?.always_include ?? false)
  const [isActive, setIsActive] = useState(initial?.is_active ?? true)
  const [saving, setSaving] = useState(false)
  const isEdit = !!initial

  const save = async () => {
    if (!title.trim() || !content.trim()) {
      notify.error('title and content are both required')
      return
    }
    setSaving(true)
    if (isEdit) {
      await api.updateMemory({
        id: initial.id,
        title: title.trim(),
        content: content.trim(),
        is_active: isActive,
        always_include: alwaysInclude,
      })
      notify.success('updated memory')
    } else {
      await api.createMemory({
        title: title.trim(),
        content: content.trim(),
        source: 'paste',
        always_include: alwaysInclude,
      })
      notify.success('reyna will remember this.')
    }
    setSaving(false)
    onSaved()
  }

  return (
    <div onClick={onClose} style={modalBackdrop}>
      <div onClick={(e) => e.stopPropagation()} style={modalBody}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
          <h2 style={{ fontSize: 22, fontWeight: 800, margin: 0 }}>
            {isEdit ? 'edit memory' : 'add a memory'}
          </h2>
          <button onClick={onClose} style={iconBtn}>
            <Fa icon="fa-xmark" style={{ fontSize: 14 }} />
          </button>
        </div>

        <label style={label}>title</label>
        <input
          type="text"
          value={title}
          onChange={e => setTitle(e.target.value)}
          placeholder="e.g. my sem 5 syllabus"
          style={input}
          autoFocus
        />

        <label style={label}>content</label>
        <textarea
          value={content}
          onChange={e => setContent(e.target.value)}
          placeholder="paste your syllabus, exam schedule, study goals, or any context you want reyna to remember..."
          rows={10}
          style={{ ...input, fontFamily: 'inherit', resize: 'vertical', minHeight: 180 }}
        />

        <div style={{ display: 'flex', gap: 16, alignItems: 'center', marginBottom: 16, marginTop: 8 }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#444', cursor: 'pointer' }}>
            <input type="checkbox" checked={alwaysInclude} onChange={e => setAlwaysInclude(e.target.checked)} />
            <span>
              <strong>always include</strong> so reyna uses this on every answer.
              best for small pinned facts like exam dates or study style. larger
              memories get smart-recalled only when relevant.
            </span>
          </label>
        </div>

        {isEdit && (
          <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 16, fontSize: 13, color: '#444' }}>
            <ToggleSwitch active={isActive} onClick={() => setIsActive(v => !v)} />
            <span>{isActive ? 'active, in use' : 'off, reyna will ignore this'}</span>
          </div>
        )}

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 12 }}>
          <button onClick={onClose} style={secondaryBtn} disabled={saving}>cancel</button>
          <button onClick={save} style={primaryBtn} disabled={saving}>
            {saving ? 'saving…' : (isEdit ? 'save changes' : 'save memory')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── inline styles ──
const primaryBtn = {
  background: 'var(--reyna-accent)', color: '#fff', border: 'none',
  padding: '10px 20px', borderRadius: 'var(--roundness)',
  fontSize: 13, fontWeight: 600, cursor: 'pointer', fontFamily: 'inherit',
  display: 'flex', alignItems: 'center', gap: 8,
}
const secondaryBtn = {
  background: 'transparent', color: '#666', border: '1px solid #ccc',
  padding: '10px 20px', borderRadius: 'var(--roundness)',
  fontSize: 13, fontWeight: 500, cursor: 'pointer', fontFamily: 'inherit',
}
const iconBtn = {
  background: 'transparent', border: '1px solid #e0e0e0', borderRadius: 6,
  padding: '6px 10px', color: '#666', cursor: 'pointer',
}
const pinBadge = {
  display: 'inline-flex', alignItems: 'center', gap: 4,
  fontSize: 10, fontWeight: 700, background: 'rgba(37,211,102,0.12)',
  color: '#0f6e56', padding: '3px 8px', borderRadius: 10,
  textTransform: 'uppercase', letterSpacing: 0.4,
}
const emptyBox = {
  border: '1px dashed #ccc', borderRadius: 'var(--roundness)',
  padding: '48px 24px', textAlign: 'center', background: '#fafafa',
}
const modalBackdrop = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)',
  display: 'flex', alignItems: 'center', justifyContent: 'center',
  zIndex: 10000, padding: 20,
}
const modalBody = {
  background: '#fff', borderRadius: 'var(--roundness)',
  padding: 28, width: '100%', maxWidth: 560, maxHeight: '90vh', overflowY: 'auto',
  boxShadow: '0 24px 64px rgba(0,0,0,0.25)',
}
const label = {
  display: 'block', fontSize: 12, fontWeight: 600, color: '#444',
  textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: 6, marginTop: 10,
}
const input = {
  width: '100%', padding: '10px 12px', fontSize: 14,
  border: '1.5px solid #ccc', borderRadius: 'var(--roundness)',
  marginBottom: 8, fontFamily: 'inherit', background: '#fff', color: '#222',
}
