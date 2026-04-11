import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, saveAuth, isLoggedIn } from '../lib/api'
import { Fa, icons } from '../components/icons'

export default function Login() {
  const [phone, setPhone] = useState('')
  const [name, setName] = useState('')
  const [isRegister, setIsRegister] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  useEffect(() => { if (isLoggedIn()) navigate('/dashboard') }, [])

  const handleSubmit = async (e) => {
    e.preventDefault(); setError(''); setLoading(true)
    try {
      const data = isRegister ? await api.register(phone, name) : await api.login(phone)
      if (data?.error) { setError(data.error); if (data.error.includes('not found')) setIsRegister(true) }
      else { saveAuth(data); navigate('/dashboard') }
    } catch { setError('Connection failed. Is the backend running?') }
    setLoading(false)
  }

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg-color)' }}>
      <div style={{ maxWidth: 380, width: '100%', padding: 32 }}>
        <div style={{ textAlign: 'center', marginBottom: 40 }}>
          <h1 style={{ fontSize: 20, fontWeight: 800, letterSpacing: -0.5, marginBottom: 8, color: 'var(--text-color)', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0 }}>
            <Fa icon="fa-crown" style={{ fontSize: 20, color: 'var(--reyna-accent)', marginRight: 6, filter: 'drop-shadow(0 0 6px rgba(37,211,102,0.4))' }} />
            reyna
            <span style={{ fontSize: 10, color: 'var(--sub-color)', fontWeight: 400, marginLeft: 3 }}>v2</span>
          </h1>
          <p style={{ fontSize: 14, color: 'var(--sub-color)' }}>your group chat's knowledge base</p>
        </div>

        <div style={{ background: '#fff', border: '1px solid #ddd', borderRadius: '12px', padding: 32, boxShadow: '0 8px 30px rgba(0,0,0,0.08), 0 2px 8px rgba(0,0,0,0.04)' }}>
          <h2 style={{ fontSize: 18, fontWeight: 700, marginBottom: 4, color: 'var(--text-color)' }}>
            {isRegister ? 'create account' : 'sign in'}
          </h2>
          <p style={{ fontSize: 11, color: 'var(--sub-color)', marginBottom: 20 }}>
            {isRegister ? 'register with your WhatsApp number' : 'use your WhatsApp number to log in'}
          </p>

          <form onSubmit={handleSubmit}>
            <label style={{ fontSize: 11, fontWeight: 400, color: 'var(--sub-color)', display: 'block', marginBottom: 6 }}>whatsapp number</label>
            <input type="text" value={phone} onChange={e => setPhone(e.target.value)} placeholder="+91 9876543210"
              style={{ width: '100%', padding: '10px 12px', border: '1px solid var(--input-border, #d9d4cc)', borderRadius: 'var(--roundness)', fontSize: 14, marginBottom: 14, outline: 'none', fontFamily: 'var(--font-mono)', background: '#fff', color: 'var(--text-color)' }} />

            {isRegister && (<>
              <label style={{ fontSize: 11, fontWeight: 400, color: 'var(--sub-color)', display: 'block', marginBottom: 6 }}>name</label>
              <input type="text" value={name} onChange={e => setName(e.target.value)} placeholder="your name"
                style={{ width: '100%', padding: '10px 12px', border: '1px solid var(--input-border, #d9d4cc)', borderRadius: 'var(--roundness)', fontSize: 14, marginBottom: 14, outline: 'none', background: '#fff', color: 'var(--text-color)' }} />
            </>)}

            {error && (
              <div style={{ fontSize: 11, color: 'var(--error-color)', marginBottom: 12, padding: '8px 10px', background: 'rgba(220,38,38,0.06)', borderRadius: 'var(--roundness)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <Fa icon={icons.warning} style={{ fontSize: 10 }} /> {error}
              </div>
            )}

            <button type="submit" disabled={loading || !phone} style={{
              width: '100%', padding: '10px', background: 'var(--reyna-accent)', color: '#fff', border: 'none',
              borderRadius: 'var(--roundness)', fontSize: 15, fontWeight: 700, cursor: 'pointer', opacity: loading ? 0.6 : 1,
              display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            }}>
              {loading ? <><Fa icon={icons.loading} spin style={{ fontSize: 11 }} /> loading...</> : isRegister ? 'register' : 'sign in'}
            </button>
          </form>

          <div style={{ textAlign: 'center', marginTop: 14 }}>
            <button onClick={() => { setIsRegister(!isRegister); setError('') }} style={{ background: 'none', border: 'none', color: 'var(--main-color)', fontSize: 11, cursor: 'pointer', fontWeight: 400 }}>
              {isRegister ? <><Fa icon="fa-arrow-left" style={{ fontSize: 9 }} /> back to login</> : "don't have an account? register"}
            </button>
          </div>
        </div>

      </div>
    </div>
  )
}
