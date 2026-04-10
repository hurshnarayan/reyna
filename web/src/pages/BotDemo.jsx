import { useState, useRef, useEffect } from 'react'
import { api, getUser } from '../lib/api'
import { Fa, icons } from '../components/icons'

function timeNow() {
  const d = new Date()
  let h = d.getHours(), m = d.getMinutes().toString().padStart(2, '0'), a = h >= 12 ? 'PM' : 'AM'
  return `${h % 12 || 12}:${m} ${a}`
}

const cardStyle = { background: 'var(--card-bg)', border: '1px solid var(--card-border)', borderRadius: 'var(--roundness)', boxShadow: 'var(--card-shadow)' }

export default function BotDemo() {
  const user = getUser()
  const [messages, setMessages] = useState([
    { from: 'Rahul', type: 'other', text: 'CN_Unit4_Notes.pdf (2.3 MB)', time: '10:42 AM' },
    { from: 'You', type: 'sent', text: 'reyna save', time: '10:42 AM' },
    { from: 'Reyna', type: 'reyna', text: 'Staged `CN_Unit4_Notes.pdf`. 12 files in your repository.\n\nUse `push` to commit to Drive, or `remove [filename]` to unstage.', time: '10:42 AM' },
  ])
  const [input, setInput] = useState('')
  const [typing, setTyping] = useState(false)
  const chatRef = useRef(null)

  useEffect(() => { if (chatRef.current) chatRef.current.scrollTop = chatRef.current.scrollHeight }, [messages, typing])

  const send = async () => {
    if (!input.trim()) return
    const text = input.trim(); setInput(''); const t = timeNow()
    setMessages(p => [...p, { from: 'You', type: 'sent', text, time: t }]); setTyping(true)
    try {
      const resp = await api.botCommand('120363xxxxx@g.us', text, user?.phone || '+919876543210')
      setTimeout(() => {
        setTyping(false)
        setMessages(p => [...p, { from: 'Reyna', type: 'reyna', text: resp?.reply || 'Could not reach server.', time: timeNow() }])
      }, 600 + Math.random() * 800)
    } catch {
      setTyping(false)
      setMessages(p => [...p, { from: 'Reyna', type: 'reyna', text: 'Server is not responding.', time: timeNow() }])
    }
  }

  const quickCmds = ['reyna help', 'reyna find DSA notes', 'reyna save', 'reyna push', 'reyna status', '/reyna log']

  return (
    <div style={{ padding: '32px 40px', maxWidth: 900 }} className="fade-in">
      <h1 style={{ fontSize: 44, fontWeight: 900, letterSpacing: -0.5, marginBottom: 6, color: 'var(--text-color)', display: 'flex', alignItems: 'center', gap: 10 }}>
        <Fa icon={icons.bot} style={{ fontSize: 32 }} /> bot demo
      </h1>
      <p style={{ fontSize: 15, color: '#888', marginBottom: 28 }}>
        this hits the <strong style={{ color: 'var(--text-color)' }}>real Go backend</strong>. try natural language or slash commands.
      </p>

      <div style={{ display: 'flex', gap: 20, alignItems: 'flex-start' }}>
        {/* Chat */}
        <div style={{ background: '#0b141a', borderRadius: 'var(--roundness)', overflow: 'hidden', width: 420, flexShrink: 0, boxShadow: '0 20px 60px rgba(0,0,0,0.3)', border: '1px solid #1a2730' }}>
          <div style={{ background: '#075E54', padding: '12px 16px', display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{ width: 38, height: 38, borderRadius: '50%', background: 'linear-gradient(135deg, #25D366, #128C7E)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 16, fontWeight: 800, color: '#fff' }}>R</div>
            <div><div style={{ fontSize: 14, fontWeight: 600, color: '#e9edef' }}>CSE 2026 — Section B</div><div style={{ fontSize: 11, color: '#8696a0' }}>Reyna, Rahul, Priya, +47</div></div>
          </div>
          <div ref={chatRef} style={{ padding: '12px 14px', minHeight: 400, maxHeight: 500, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 5 }}>
            {messages.map((m, i) => (
              <div key={i} style={{
                alignSelf: m.type === 'sent' ? 'flex-end' : 'flex-start',
                background: m.type === 'sent' ? '#005c4b' : '#1f2c34',
                borderLeft: m.type === 'reyna' ? '2px solid #25D366' : 'none',
                maxWidth: '85%', padding: '7px 11px', borderRadius: 8,
                borderBottomRightRadius: m.type === 'sent' ? 2 : 8,
                borderBottomLeftRadius: m.type !== 'sent' ? 2 : 8,
                animation: 'fadeIn 0.3s ease',
              }}>
                <div style={{ fontSize: 11, fontWeight: 600, marginBottom: 2, color: m.type === 'sent' ? '#a8d8a8' : m.type === 'reyna' ? '#25D366' : '#53bdeb' }}>{m.from}</div>
                <div style={{ fontSize: 13, color: '#e9edef', whiteSpace: 'pre-wrap', lineHeight: 1.5 }}>{m.text}</div>
                <div style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', textAlign: 'right', marginTop: 3 }}>{m.time}</div>
              </div>
            ))}
            {typing && (
              <div style={{ alignSelf: 'flex-start', background: '#1f2c34', borderLeft: '2px solid #25D366', padding: '7px 11px', borderRadius: 8, borderBottomLeftRadius: 2 }}>
                <div style={{ fontSize: 11, fontWeight: 600, color: '#25D366' }}>Reyna</div>
                <div style={{ fontSize: 12, color: '#8696a0', fontStyle: 'italic' }}>typing...</div>
              </div>
            )}
          </div>
          <div style={{ padding: '6px 12px', display: 'flex', gap: 6, overflowX: 'auto', borderTop: '1px solid #1a2730' }}>
            {quickCmds.map(c => (
              <button key={c} onClick={() => setInput(c)} style={{ background: '#1a2730', border: '1px solid #2a3942', borderRadius: 14, padding: '4px 10px', color: '#25D366', fontSize: 11, cursor: 'pointer', whiteSpace: 'nowrap', fontFamily: 'var(--font-mono)' }}>{c}</button>
            ))}
          </div>
          <div style={{ padding: '8px 12px', background: '#075E54', display: 'flex', gap: 8, alignItems: 'center' }}>
            <input value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => e.key === 'Enter' && send()} placeholder="type a command..."
              style={{ flex: 1, background: '#2a3942', border: 'none', borderRadius: 20, padding: '9px 16px', color: '#e9edef', fontSize: 13, fontFamily: 'var(--font-mono)', outline: 'none' }} />
            <button onClick={send} style={{ width: 36, height: 36, borderRadius: '50%', background: '#25D366', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Fa icon={icons.send} style={{ color: '#fff', fontSize: 14 }} />
            </button>
          </div>
        </div>

        {/* Command Reference */}
        <div style={{ flex: 1 }}>
          <div style={{ ...cardStyle, padding: 20 }}>
            <h3 style={{ fontSize: 14, fontWeight: 400, marginBottom: 14, color: 'var(--main-color)' }}>command reference</h3>
            <div style={{ fontSize: 9, color: 'var(--reyna-accent)', fontWeight: 400, marginBottom: 10, textTransform: 'lowercase', letterSpacing: 1.5 }}>natural language (recommended)</div>
            {[
              { cmd: 'reyna save', desc: 'stage recent untracked files' },
              { cmd: 'reyna find DSA notes', desc: 'search files by topic' },
              { cmd: 'reyna push', desc: 'commit staged files to drive' },
              { cmd: 'reyna status', desc: "see what's new" },
              { cmd: 'reyna history', desc: 'show recent files' },
              { cmd: 'reyna help', desc: 'show available commands' },
            ].map((c, i) => (
              <div key={i} onClick={() => setInput(c.cmd)} style={{ padding: '6px 10px', borderBottom: '1px solid var(--card-border)', cursor: 'pointer', transition: 'background 0.15s', borderRadius: 'var(--roundness)' }}
                onMouseEnter={e => e.currentTarget.style.background = 'var(--card-hover)'}
                onMouseLeave={e => e.currentTarget.style.background = 'transparent'}>
                <code style={{ fontSize: 12, fontWeight: 500, color: 'var(--reyna-accent)', background: 'var(--reyna-accent-dim)', padding: '2px 8px', borderRadius: 'var(--roundness)' }}>{c.cmd}</code>
                <div style={{ fontSize: 11, color: 'var(--sub-color)', marginTop: 3 }}>{c.desc}</div>
              </div>
            ))}
            <div style={{ fontSize: 9, color: 'var(--sub-color)', fontWeight: 400, marginTop: 14, marginBottom: 8, textTransform: 'lowercase', letterSpacing: 1.5 }}>slash commands (legacy)</div>
            {[
              { cmd: '/reyna add .', desc: 'stage the last shared file' },
              { cmd: '/reyna commit', desc: 'commit all staged to drive' },
              { cmd: '/reyna rm File', desc: 'remove a staged file' },
              { cmd: '/reyna find "query"', desc: 'search stored files' },
              { cmd: '/reyna log', desc: 'show file history' },
              { cmd: '/reyna staged', desc: 'view staged files' },
            ].map((c, i) => (
              <div key={i} onClick={() => setInput(c.cmd)} style={{ padding: '6px 10px', borderBottom: '1px solid var(--card-border)', cursor: 'pointer', transition: 'background 0.15s', borderRadius: 'var(--roundness)' }}
                onMouseEnter={e => e.currentTarget.style.background = 'var(--card-hover)'}
                onMouseLeave={e => e.currentTarget.style.background = 'transparent'}>
                <code style={{ fontSize: 12, fontWeight: 500, color: 'var(--reyna-accent)', background: 'var(--reyna-accent-dim)', padding: '2px 8px', borderRadius: 'var(--roundness)' }}>{c.cmd}</code>
                <div style={{ fontSize: 11, color: 'var(--sub-color)', marginTop: 3 }}>{c.desc}</div>
              </div>
            ))}
          </div>

          <div style={{ ...cardStyle, padding: 14, marginTop: 12, borderTop: '2px solid var(--main-color)' }}>
            <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--text-color)', marginBottom: 4, display: 'flex', alignItems: 'center', gap: 6 }}>
              <Fa icon={icons.tip} style={{ fontSize: 11 }} /> how it connects
            </div>
            <p style={{ fontSize: 11, color: 'var(--sub-color)', lineHeight: 1.7 }}>
              this demo hits <code style={{ fontSize: 10 }}>POST /api/bot/command</code> — the same endpoint the WhatsApp bot (Baileys) will call.
              every command here creates real database entries.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
