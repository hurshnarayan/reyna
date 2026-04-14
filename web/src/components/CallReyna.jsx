import { useEffect, useRef, useState } from 'react'
import { api, getUser } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa } from '../components/icons'

// Reyna Live — "Call Reyna" floating button using the Vapi web SDK.
// Loads the SDK lazily on first click so the page bundle stays lean.
// Falls back to a helpful message if VAPI_PUBLIC_KEY / VAPI_ASSISTANT_ID
// aren't configured on the server.

export default function CallReyna() {
  const [config, setConfig] = useState(null) // { public_key, assistant_id, enabled }
  const [status, setStatus] = useState('idle') // idle | connecting | live | error
  const [expanded, setExpanded] = useState(false)
  const [transcript, setTranscript] = useState([]) // [{ role, text }]
  const vapiRef = useRef(null)

  useEffect(() => { api.voiceConfig().then(setConfig) }, [])

  const ensureVapi = async () => {
    if (vapiRef.current) return vapiRef.current
    // @vapi-ai/web ships as CommonJS with `exports.default = Vapi`. Under
    // Vite's ESM interop the namespace object sometimes wraps this a level
    // deeper (mod.default.default), so unwrap defensively.
    const mod = await import('@vapi-ai/web')
    const Vapi =
      (typeof mod === 'function' && mod) ||
      (typeof mod.default === 'function' && mod.default) ||
      (mod.default && typeof mod.default.default === 'function' && mod.default.default) ||
      mod.Vapi
    if (typeof Vapi !== 'function') {
      throw new Error('vapi constructor not found. module keys: ' + Object.keys(mod).join(','))
    }
    const v = new Vapi(config.public_key)
    v.on('call-start', () => { setStatus('live'); setTranscript([]) })
    v.on('call-end', () => { setStatus('idle') })
    v.on('error', (e) => {
      console.error('[Vapi] error:', e)
      setStatus('error')
      notify.error('Call failed — ' + (e?.errorMsg || e?.error?.msg || 'try again'))
    })
    v.on('message', (msg) => {
      // Stream transcripts so the user can read along while speaking.
      if (msg.type === 'transcript' && msg.transcriptType === 'final') {
        setTranscript(prev => [...prev, { role: msg.role, text: msg.transcript }])
      }
    })
    vapiRef.current = v
    return v
  }

  const startCall = async () => {
    if (!config?.enabled) {
      notify.error('Reyna Live isn\'t set up yet — ask the admin to configure VAPI_PUBLIC_KEY and VAPI_ASSISTANT_ID.')
      return
    }
    setStatus('connecting')
    setExpanded(true)
    try {
      const v = await ensureVapi()
      // Stash the user's phone in assistant metadata so our backend tools
      // can identify them. The assistant system prompt references this.
      const user = getUser()
      const vars = {
        user_phone: user?.phone || '',
        user_name: user?.name || 'the user',
      }
      // Pass identity via overrides. The backend's voice handler auto-fills
      // user_phone from call.assistantOverrides.metadata if the LLM forgets
      // to pass it, so tool calls work regardless. firstMessage overrides
      // the stored greeting so the session starts by name.
      await v.start(config.assistant_id, {
        variableValues: vars,
        metadata: vars,
        firstMessage: `Hey ${vars.user_name}, Reyna here. What do you need?`,
      })
    } catch (err) {
      console.error('[Vapi] start error:', err)
      setStatus('error')
      notify.error('Could not start call: ' + (err?.message || 'unknown error'))
    }
  }

  const endCall = () => {
    try { vapiRef.current?.stop() } catch {}
    setStatus('idle')
  }

  const closePanel = () => {
    if (status === 'live' || status === 'connecting') endCall()
    setExpanded(false)
  }

  // Compact mic button when collapsed; panel with transcript when expanded.
  if (!expanded) {
    return (
      <button onClick={() => config?.enabled ? startCall() : notify.error('Reyna Live isn\'t set up yet.')}
        title="Call Reyna" style={floatingBtn}>
        <Fa icon="fa-phone" style={{ fontSize: 18 }} />
        <span style={{ fontSize: 13, fontWeight: 700 }}>Call Reyna</span>
      </button>
    )
  }

  return (
    <div style={panel}>
      <div style={panelHeader}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{
            width: 10, height: 10, borderRadius: '50%',
            background: status === 'live' ? '#25D366' : status === 'connecting' ? '#f59e0b' : '#999',
            boxShadow: status === 'live' ? '0 0 8px #25D366' : 'none',
            animation: status === 'live' ? 'reynaPulse 1.4s infinite' : 'none',
          }} />
          <span style={{ fontSize: 14, fontWeight: 700 }}>
            {status === 'live' ? 'Reyna is listening' :
             status === 'connecting' ? 'Connecting…' :
             status === 'error' ? 'Call failed' : 'Reyna Live'}
          </span>
        </div>
        <button onClick={closePanel} style={iconBtn}>
          <Fa icon="fa-xmark" style={{ fontSize: 14 }} />
        </button>
      </div>

      <div style={transcriptBox}>
        {transcript.length === 0 ? (
          <div style={{ color: '#888', fontSize: 13, textAlign: 'center', padding: 20 }}>
            {status === 'live' ? 'Speak now — ask anything about your notes.' :
             status === 'connecting' ? 'Opening mic…' : 'Press the call button below to start.'}
          </div>
        ) : (
          transcript.map((t, i) => (
            <div key={i} style={{
              marginBottom: 10, fontSize: 13,
              color: t.role === 'user' ? '#222' : 'var(--reyna-accent)',
            }}>
              <strong style={{ textTransform: 'capitalize' }}>{t.role === 'user' ? 'you' : 'reyna'}:</strong>{' '}
              {t.text}
            </div>
          ))
        )}
      </div>

      <div style={{ display: 'flex', gap: 8, padding: '12px 16px', borderTop: '1px solid #eee' }}>
        {status === 'idle' || status === 'error' ? (
          <button onClick={startCall} style={{ ...primaryBtn, flex: 1 }}>
            <Fa icon="fa-phone" style={{ fontSize: 12 }} /> start call
          </button>
        ) : (
          <button onClick={endCall} style={{ ...primaryBtn, flex: 1, background: '#dc2626' }}>
            <Fa icon="fa-phone-slash" style={{ fontSize: 12 }} /> end call
          </button>
        )}
      </div>

      <style>{`@keyframes reynaPulse { 0%,100% { transform: scale(1); } 50% { transform: scale(1.3); } }`}</style>
    </div>
  )
}

const floatingBtn = {
  position: 'fixed', right: 24, bottom: 24, zIndex: 500,
  background: 'var(--reyna-accent)', color: '#fff', border: 'none',
  padding: '14px 20px', borderRadius: 28,
  display: 'flex', alignItems: 'center', gap: 10,
  fontFamily: 'inherit', fontSize: 14, fontWeight: 700,
  cursor: 'pointer', boxShadow: '0 6px 20px rgba(37,211,102,0.35)',
  transition: 'transform 0.15s, box-shadow 0.15s',
}
const panel = {
  position: 'fixed', right: 24, bottom: 24, zIndex: 500,
  width: 360, maxHeight: '60vh',
  background: '#fff', borderRadius: 14, border: '1px solid #e0e0e0',
  boxShadow: '0 12px 40px rgba(0,0,0,0.18)',
  display: 'flex', flexDirection: 'column', overflow: 'hidden',
}
const panelHeader = {
  padding: '14px 16px', borderBottom: '1px solid #eee',
  display: 'flex', justifyContent: 'space-between', alignItems: 'center',
}
const transcriptBox = {
  flex: 1, padding: '16px', overflowY: 'auto', minHeight: 180,
  background: '#fafafa',
}
const primaryBtn = {
  background: 'var(--reyna-accent)', color: '#fff', border: 'none',
  padding: '10px 16px', borderRadius: 'var(--roundness)',
  fontSize: 13, fontWeight: 700, cursor: 'pointer', fontFamily: 'inherit',
  display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8,
}
const iconBtn = {
  background: 'transparent', border: 'none', color: '#666',
  padding: 6, cursor: 'pointer',
}
