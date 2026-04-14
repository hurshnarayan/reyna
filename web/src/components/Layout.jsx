import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom'
import { isLoggedIn, logout, getUser, api } from '../lib/api'
import { useEffect, useState, useRef } from 'react'
import { NotificationBell } from './Notifications'
import { Fa, icons } from './icons'
import CallReyna from './CallReyna'

const navItems = [
  { to: '/dashboard', label: 'dashboard', icon: icons.dashboard },
  { to: '/files', label: 'files', icon: icons.files },
  { to: '/search', label: 'recall', icon: icons.search },
  { to: '/memory', label: 'memory', icon: 'fa-brain' },
]
const navAfterAlerts = [
  // { to: '/bot', label: 'bot demo', icon: icons.bot },  // Removed — bot demo disabled
]

function load(k, fb) { try { const v = localStorage.getItem('reyna_' + k); return v !== null ? JSON.parse(v) : fb } catch { return fb } }
function save(k, v) { try { localStorage.setItem('reyna_' + k, JSON.stringify(v)) } catch {} }

export default function Layout() {
  const navigate = useNavigate()
  const location = useLocation()
  const user = getUser()

  const [showSettings, setShowSettings] = useState(false)
  const [musicOn, setMusicOn] = useState(false)
  const [musicVol, setMusicVol] = useState(() => load('musicVol', 0.3))
  const [blobsOn, setBlobsOn] = useState(() => load('blobsOn', true))
  const [btnAnimOn, setBtnAnimOn] = useState(() => load('btnAnimOn', true))
  const [darkMode, setDarkMode] = useState(() => load('darkMode', false))
  const [autoCommit, setAutoCommit] = useState(() => load('autoCommit', '24 hours'))
  const [defaultMode, setDefaultMode] = useState(() => load('defaultMode', 'auto'))
  const [openSections, setOpenSections] = useState({ sound: true, appearance: true, tracking: true })

  const audioRef = useRef(null)

  // network status bar
  const [offline, setOffline] = useState(!navigator.onLine)
  const [showOfflineBar, setShowOfflineBar] = useState(!navigator.onLine)
  const [justReconnected, setJustReconnected] = useState(false)
  useEffect(() => {
    const goOffline = () => { setOffline(true); setShowOfflineBar(true); setJustReconnected(false) }
    const goOnline = () => {
      setOffline(false); setShowOfflineBar(true); setJustReconnected(true)
      setTimeout(() => { setShowOfflineBar(false); setJustReconnected(false) }, 3000)
    }
    window.addEventListener('offline', goOffline)
    window.addEventListener('online', goOnline)
    return () => { window.removeEventListener('offline', goOffline); window.removeEventListener('online', goOnline) }
  }, [])

  useEffect(() => { if (!isLoggedIn()) navigate('/login') }, [])
  useEffect(() => { document.body.classList.toggle('reyna-dark', darkMode); save('darkMode', darkMode) }, [darkMode])
  useEffect(() => { document.querySelectorAll('.reyna-blob').forEach(el => { el.style.opacity = blobsOn ? '0.1' : '0' }); save('blobsOn', blobsOn) }, [blobsOn])
  useEffect(() => { document.body.classList.toggle('reyna-no-btn-anim', !btnAnimOn); save('btnAnimOn', btnAnimOn) }, [btnAnimOn])

  const toggleMusic = () => {
    if (!audioRef.current) { audioRef.current = new Audio('/lusion_music.mp3'); audioRef.current.loop = true; audioRef.current.volume = musicVol }
    if (musicOn) audioRef.current.pause(); else audioRef.current.play().catch(() => {})
    setMusicOn(!musicOn)
  }
  const changeVol = (v) => { setMusicVol(v); save('musicVol', v); if (audioRef.current) audioRef.current.volume = v }
  const toggleSection = (s) => setOpenSections(p => ({ ...p, [s]: !p[s] }))

  // when settings open, nav links should look inactive
  const navLinkStyle = (isActive) => {
    const active = isActive && !showSettings
    return {
      display: 'flex', alignItems: 'center', gap: 12, padding: '10px 20px',
      fontSize: 15, fontWeight: active ? 600 : 400,
      color: active ? 'var(--reyna-accent)' : '#888',
      background: active ? 'rgba(37,211,102,0.08)' : 'transparent',
      borderLeft: active ? '2px solid var(--reyna-accent)' : '2px solid transparent',
      transition: 'all 0.15s', letterSpacing: 0.3,
    }
  }

  const SideNavLink = ({ to, icon, label }) => {
    const isActive = location.pathname === to || location.pathname.startsWith(to + '/')
    const active = isActive && !showSettings
    return (
      <NavLink to={to} onClick={() => setShowSettings(false)} style={() => navLinkStyle(isActive)}>
        <Fa icon={icon} fw style={{ fontSize: 18, width: 22, textAlign: 'center' }} />
        {label}
        <i className={`fas fa-chevron-right reyna-nav-arrow ${active ? 'reyna-arrow-active' : ''}`} />
      </NavLink>
    )
  }

  const sectionHeader = (id, title) => (
    <div onClick={() => toggleSection(id)} style={{
      display: 'flex', alignItems: 'center', gap: 10, marginBottom: openSections[id] ? 24 : 16,
      cursor: 'pointer', userSelect: 'none',
    }}>
      <i className={`fas fa-chevron-right reyna-nav-arrow ${openSections[id] ? 'reyna-arrow-open' : ''}`}
        style={{ fontSize: 12, color: 'var(--reyna-accent)', marginLeft: 0 }} />
      <h2 style={{ fontSize: 20, fontWeight: 700, color: 'var(--reyna-accent)', margin: 0 }}>{title}</h2>
    </div>
  )

  const SettingRow = ({ icon, title, desc, options, active, onSelect, extra }) => (
    <div style={{ marginBottom: 28 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <Fa icon={icon} style={{ fontSize: 11, color: 'var(--reyna-accent)' }} />
        <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--reyna-accent)' }}>{title}</span>
      </div>
      <p style={{ fontSize: 13, color: '#666', marginBottom: 12, lineHeight: 1.6 }}>{desc}</p>
      <div style={{ display: 'flex', gap: 0, border: '1px solid #333', borderRadius: 'var(--roundness)', overflow: 'hidden', width: 'fit-content' }}>
        {options.map((opt, idx) => (
          <button key={opt} onClick={() => onSelect(opt)} style={{
            padding: '8px 24px', fontSize: 13, fontWeight: opt === active ? 600 : 400,
            background: opt === active ? 'var(--reyna-accent)' : 'transparent',
            color: opt === active ? '#fff' : '#666',
            border: 'none', cursor: 'pointer', fontFamily: 'inherit',
            borderLeft: idx > 0 ? '1px solid #333' : 'none', transition: 'all 0.15s',
          }}>{opt}</button>
        ))}
      </div>
      {extra}
    </div>
  )

  return (
    <div style={{ display: 'flex', minHeight: '100vh', flexDirection: 'column' }}>
      {/* network status bar */}
      {showOfflineBar && (
        <div onClick={() => setShowOfflineBar(false)} style={{
          position: 'fixed', top: 0, left: 0, right: 0, zIndex: 9999,
          background: offline ? '#dc2626' : '#25D366',
          color: '#fff', fontSize: 13, fontWeight: 600,
          padding: '8px 20px',
          display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8,
          animation: 'slideDownBar 0.3s ease',
          boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
          cursor: 'pointer',
        }}>
          <Fa icon={offline ? 'fa-wifi' : 'fa-check-circle'} style={{ fontSize: 12 }} />
          {offline ? "you're offline. check your internet connection." : 'back online.'}
          <Fa icon="fa-xmark" style={{ position: 'absolute', right: 16, fontSize: 14, opacity: 0.8 }} />
        </div>
      )}
      <style>{`@keyframes slideDownBar{from{transform:translateY(-100%)}to{transform:translateY(0)}}`}</style>
      <div style={{ display: 'flex', flex: 1 }}>
      <aside style={{
        width: 220, background: 'var(--sidebar-bg)', padding: '24px 0',
        display: 'flex', flexDirection: 'column', position: 'fixed', top: 0, bottom: 0, left: 0,
        zIndex: 100, borderRight: '1px solid var(--sidebar-border)',
      }}>
        <div style={{ padding: '0 20px 24px', borderBottom: '1px solid var(--sidebar-border)' }}>
          <a href="/" style={{ fontWeight: 800, fontSize: 20, color: '#fff', letterSpacing: -0.5, display: 'flex', alignItems: 'center', gap: 0, textDecoration: 'none' }}>
            <Fa icon="fa-crown" style={{ fontSize: 20, color: 'var(--reyna-accent)', marginRight: 6, filter: 'drop-shadow(0 0 6px rgba(37,211,102,0.4))' }} />
            reyna
            <span style={{ fontSize: 10, color: '#555', fontWeight: 400, marginLeft: 3 }}>v2</span>
          </a>
        </div>

        <nav style={{ flex: 1, padding: '16px 0' }}>
          {navItems.map(n => <SideNavLink key={n.to} {...n} />)}
          <NotificationBell />
          {navAfterAlerts.map(n => <SideNavLink key={n.to} {...n} />)}

          <div style={{ height: 1, background: '#222', margin: '8px 16px' }} />

          <div onClick={() => setShowSettings(p => !p)} style={{
            display: 'flex', alignItems: 'center', gap: 12, padding: '10px 20px',
            fontSize: 15, fontWeight: showSettings ? 600 : 400,
            color: showSettings ? 'var(--reyna-accent)' : '#888', cursor: 'pointer',
            borderLeft: showSettings ? '2px solid var(--reyna-accent)' : '2px solid transparent',
            background: showSettings ? 'rgba(37,211,102,0.08)' : 'transparent', transition: 'all 0.15s',
          }}>
            <Fa icon={icons.settings} fw style={{ fontSize: 18, width: 22, textAlign: 'center' }} />
            settings
            <i className={`fas fa-chevron-right reyna-nav-arrow ${showSettings ? 'reyna-arrow-active' : ''}`} />
          </div>
        </nav>

        <div style={{ padding: '16px 20px', borderTop: '1px solid var(--sidebar-border)' }}>
          {user && <div style={{ fontSize: 12, color: '#666', marginBottom: 8, display: 'flex', alignItems: 'center', gap: 8 }}>
            <Fa icon={icons.user} style={{ fontSize: 11 }} /> {user.name || user.phone}
          </div>}
          <button onClick={() => { logout(); navigate('/login') }} style={{
            background: 'none', border: '1px solid #333', color: '#888',
            padding: '6px 14px', borderRadius: 'var(--roundness)', fontSize: 11, cursor: 'pointer',
            width: '100%', transition: 'all 0.15s', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, fontFamily: 'inherit',
          }}>
            <Fa icon={icons.logout} style={{ fontSize: 10 }} /> logout
          </button>
        </div>
      </aside>

      {/* ── Settings Panel (always dark bg) ── */}
      {showSettings && (
        <div className="reyna-settings-panel" style={{
          position: 'fixed', top: 0, left: 220, bottom: 0, right: 0,
          background: '#111', color: '#ccc', zIndex: 99, overflowY: 'auto', padding: '48px 60px',
        }}>
          <div style={{ maxWidth: 700 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 40 }}>
              <h1 style={{ fontSize: 32, fontWeight: 800, color: 'var(--reyna-accent)', letterSpacing: -1 }}>settings</h1>
              <button onClick={() => setShowSettings(false)} style={{
                background: 'none', border: '1px solid #333', color: '#888', padding: '8px 20px',
                borderRadius: 'var(--roundness)', fontSize: 13, cursor: 'pointer', fontFamily: 'inherit',
                display: 'flex', alignItems: 'center', gap: 6,
              }}><Fa icon={icons.close} style={{ fontSize: 10 }} /> close</button>
            </div>

            {sectionHeader('sound', 'sound')}
            {openSections.sound && <>
              <SettingRow icon={icons.fileAudio} title="ambient music"
                desc="play a looping ambient track in the background while you use reyna."
                options={['off', 'on']} active={musicOn ? 'on' : 'off'}
                onSelect={(v) => { if ((v === 'on') !== musicOn) toggleMusic() }}
                extra={musicOn ? <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginTop: 8 }}>
                  <span style={{ width: 5, height: 5, borderRadius: '50%', background: '#25D366' }} className="reyna-music-dot" />
                  <span style={{ fontSize: 10, color: '#555' }}>now playing</span>
                </div> : null}
              />
              <div style={{ marginBottom: 28 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
                  <Fa icon={icons.fileAudio} style={{ fontSize: 11, color: 'var(--reyna-accent)' }} />
                  <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--reyna-accent)' }}>music volume</span>
                </div>
                <p style={{ fontSize: 13, color: '#666', marginBottom: 12 }}>change the volume of the ambient music.</p>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <span style={{ fontSize: 12, color: '#888', minWidth: 28 }}>{Math.round(musicVol * 100)}%</span>
                  <input type="range" min="0" max="1" step="0.05" value={musicVol} onChange={e => changeVol(parseFloat(e.target.value))}
                    style={{ flex: 1, maxWidth: 300, accentColor: 'var(--reyna-accent)' }} />
                </div>
              </div>
            </>}

            {sectionHeader('appearance', 'appearance')}
            {openSections.appearance && <>
              <SettingRow icon={icons.tip} title="theme"
                desc="switch between light and dark mode. the settings panel always stays dark."
                options={['light', 'dark']} active={darkMode ? 'dark' : 'light'}
                onSelect={(v) => setDarkMode(v === 'dark')}
              />
            </>}

            {sectionHeader('tracking', 'tracking')}
            {openSections.tracking && <>
              <SettingRow icon={icons.staging} title="auto-commit timer"
                desc="staged files are automatically committed after this period. synced with the dashboard."
                options={['off', '6 hours', '12 hours', '24 hours']} active={autoCommit}
                onSelect={async (v) => {
                  setAutoCommit(v); save('autoCommit', v)
                  const hours = v === 'off' ? 0 : parseInt(v) || 24
                  try {
                    const groups = await api.groupSettings()
                    if (groups && Array.isArray(groups)) {
                      for (const gs of groups) {
                        if (gs.group?.id) await api.updateGroupSettings(gs.group.id, { auto_commit_hours: hours })
                      }
                    }
                  } catch {}
                }}
              />
              <SettingRow icon={icons.pin} title="default tracking mode"
                desc="changes tracking mode for all your groups. synced with the dashboard."
                options={['auto', 'reactions only']} active={defaultMode}
                onSelect={async (v) => {
                  setDefaultMode(v); save('defaultMode', v)
                  // sync with backend for all groups
                  try {
                    const mode = v === 'auto' ? 'auto' : 'reaction'
                    const groups = await api.groupSettings()
                    if (groups && Array.isArray(groups)) {
                      for (const gs of groups) {
                        if (gs.group?.id) await api.updateGroupSettings(gs.group.id, { tracking_mode: mode })
                      }
                    }
                  } catch {}
                }}
              />
            </>}
          </div>
        </div>
      )}

      <main style={{ flex: 1, marginLeft: 220, background: 'var(--bg-color)', minHeight: '100vh', position: 'relative', overflow: 'hidden', transition: 'background 0.3s' }}>
        <div className="reyna-bg-blobs">
          <div className="reyna-blob reyna-blob-1" />
          <div className="reyna-blob reyna-blob-2" />
          <div className="reyna-blob reyna-blob-3" />
        </div>
        <div style={{ position: 'relative', zIndex: 1 }}>
          <Outlet />
        </div>
        <CallReyna />
      </main>
    </div>
    </div>
  )
}
