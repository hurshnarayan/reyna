import { useNavigate } from 'react-router-dom'

export default function NotFound() {
  const navigate = useNavigate()

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg-color)', textAlign: 'center', padding: 32,
    }}>
      <div style={{ maxWidth: 480 }}>
        <div style={{ fontSize: 120, lineHeight: 1, marginBottom: 16 }}>
          <span style={{ fontWeight: 900, color: '#1a1a1a', letterSpacing: -6 }}>4</span>
          <span style={{ fontWeight: 900, color: 'var(--reyna-accent)', letterSpacing: -6 }}>0</span>
          <span style={{ fontWeight: 900, color: '#1a1a1a', letterSpacing: -6 }}>4</span>
        </div>

        <h2 style={{ fontSize: 24, fontWeight: 800, color: '#1a1a1a', marginBottom: 12 }}>
          this page got lost in the group chat
        </h2>

        <p style={{ fontSize: 15, color: '#888', lineHeight: 1.7, marginBottom: 8 }}>
          just like that PDF from last tuesday that nobody can find anymore.
        </p>
        <p style={{ fontSize: 14, color: '#aaa', marginBottom: 32 }}>
          maybe try asking reyna? she remembers everything.
        </p>

        <div style={{ display: 'flex', gap: 12, justifyContent: 'center', flexWrap: 'wrap' }}>
          <button onClick={() => navigate('/')} style={{
            padding: '12px 28px', fontSize: 14, fontWeight: 700,
            background: 'var(--reyna-accent)', color: '#fff', border: 'none',
            borderRadius: 8, cursor: 'pointer',
          }}>
            go home
          </button>
          <button onClick={() => navigate('/dashboard')} style={{
            padding: '12px 28px', fontSize: 14, fontWeight: 600,
            background: '#fff', color: '#1a1a1a', border: '1px solid #ddd',
            borderRadius: 8, cursor: 'pointer',
          }}>
            dashboard
          </button>
          <button onClick={() => navigate('/search')} style={{
            padding: '12px 28px', fontSize: 14, fontWeight: 600,
            background: '#fff', color: '#1a1a1a', border: '1px solid #ddd',
            borderRadius: 8, cursor: 'pointer',
          }}>
            search
          </button>
        </div>

        <p style={{ fontSize: 11, color: '#ccc', marginTop: 40 }}>
          error 404. the file was never staged.
        </p>
      </div>
    </div>
  )
}
