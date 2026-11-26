import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, saveAuth, isLoggedIn } from '../lib/api'
import { Fa, icons } from '../components/icons'

/* ── Cat claw scratch marks (playful Reyna branding) ── */
function ClawMark({ size = 52, color = 'rgba(16, 185, 129, 0.14)', style = {} }) {
  return (
    <svg width={size} height={size * 0.85} viewBox="0 0 60 50" fill="none" style={{ pointerEvents: 'none', ...style }}>
      <path d="M 10 4 C 13 14, 12 26, 6 42 C 10 28, 15 15, 13 4 Z" fill={color} />
      <path d="M 28 2 C 31 16, 29 32, 21 48 C 26 33, 33 18, 31 2 Z" fill={color} />
      <path d="M 46 8 C 48 20, 45 32, 38 44 C 43 31, 49 19, 48 8 Z" fill={color} />
    </svg>
  )
}

/* ── Interactive Corner Cat (like Porkbun's spiderweb) ── */
function CornerCat() {
  const [hovered, setHovered] = useState(false)
  return (
    <div
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        position: 'fixed',
        bottom: 0,
        left: 0,
        zIndex: 20,
        cursor: 'pointer',
        padding: '16px 20px',
        display: 'flex',
        alignItems: 'flex-end',
        gap: 10,
        userSelect: 'none',
      }}
    >
      <div style={{ position: 'relative', width: 92, height: 78 }}>
        {/* Cat claw scratch marks raking outward */}
        <svg
          width="88"
          height="74"
          viewBox="0 0 88 74"
          fill="none"
          style={{
            position: 'absolute',
            bottom: 4,
            left: 2,
            opacity: hovered ? 0.42 : 0.2,
            transition: 'opacity 0.25s ease, transform 0.25s ease',
            transform: hovered ? 'scale(1.08) rotate(-4deg)' : 'scale(1)',
          }}
        >
          <path d="M 8 64 C 16 48, 28 32, 44 14 C 36 28, 24 44, 16 64 Z" fill="#0d3824" />
          <path d="M 20 68 C 30 50, 46 32, 66 8 C 54 26, 38 48, 28 68 Z" fill="#0d3824" />
          <path d="M 32 70 C 42 54, 60 38, 82 18 C 68 34, 52 54, 40 70 Z" fill="#0d3824" />
        </svg>

        {/* Playful Cat pill badge */}
        <div style={{
          position: 'absolute',
          bottom: 0,
          left: 0,
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          background: 'rgba(255, 255, 255, 0.85)',
          backdropFilter: 'blur(16px)',
          WebkitBackdropFilter: 'blur(16px)',
          border: '1px solid rgba(255, 255, 255, 0.9)',
          borderRadius: 9999,
          padding: '6px 14px',
          boxShadow: '0 4px 18px rgba(0,0,0,0.06)',
          transition: 'all 0.2s cubic-bezier(0.16, 1, 0.3, 1)',
          transform: hovered ? 'translateY(-3px)' : 'translateY(0)',
        }}>
          <Fa
            icon="fa-cat"
            style={{
              fontSize: 14,
              color: '#0d3824',
              filter: hovered ? 'drop-shadow(0 0 6px #25D366)' : 'none',
              transition: 'filter 0.2s ease',
            }}
          />
          <span style={{ fontSize: 11, fontWeight: 700, color: '#064e3b', letterSpacing: '-0.01em' }}>
            {hovered ? 'meow! 🐾' : 'reyna 🐾'}
          </span>
        </div>
      </div>

      {hovered && (
        <div style={{
          background: '#111827',
          color: '#ffffff',
          fontSize: 11,
          fontWeight: 500,
          padding: '6px 12px',
          borderRadius: 8,
          marginBottom: 6,
          boxShadow: '0 4px 14px rgba(0,0,0,0.18)',
          whiteSpace: 'nowrap',
          animation: 'porkbunSlideIn 0.2s ease forwards',
        }}>
          reyna’s keeping an eye on your chats 🐱
        </div>
      )}
    </div>
  )
}

export default function Login() {
  const [phone, setPhone] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [step, setStep] = useState(1) // 1: username/phone, 2: password slide-in
  const [rememberMe, setRememberMe] = useState(true)
  const [isRegister, setIsRegister] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    if (isLoggedIn()) navigate('/dashboard')
  }, [])

  const handleContinue = (e) => {
    e.preventDefault()
    setError('')
    const trimmed = phone.trim()
    if (!trimmed) {
      setError('please enter your username or whatsapp number.')
      return
    }
    setStep(2)
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const data = isRegister
        ? await api.register(phone.trim(), name.trim() || 'reyna user')
        : await api.login(phone.trim())

      if (data?.error) {
        if (data.error.includes('not found')) {
          setIsRegister(true)
          setStep(2)
          setError('no account found with this identifier. enter your details to create one.')
        } else {
          setError(data.error.toLowerCase())
        }
        setLoading(false)
      } else {
        saveAuth(data)
        setTimeout(() => {
          navigate('/dashboard')
        }, 700)
      }
    } catch {
      setError('connection failed. is the backend server running?')
      setLoading(false)
    }
  }

  const toggleMode = (registerMode) => {
    setIsRegister(registerMode)
    setStep(1)
    setError('')
    setPassword('')
  }

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'var(--bg-color, #e8e6e1)',
      padding: '40px 16px',
      fontFamily: 'var(--font, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif)',
      color: '#1a1a1a',
      position: 'relative',
      overflow: 'hidden',
    }}>
      {/* Living Ambient Background Orbs (eliminates flat boxy feel) */}
      <div style={{
        position: 'fixed',
        top: '-80px',
        right: '12%',
        width: 440,
        height: 440,
        background: '#25D366',
        borderRadius: '50%',
        filter: 'blur(130px)',
        opacity: 0.16,
        pointerEvents: 'none',
      }} />
      <div style={{
        position: 'fixed',
        bottom: '-100px',
        left: '10%',
        width: 400,
        height: 400,
        background: '#2563eb',
        borderRadius: '50%',
        filter: 'blur(130px)',
        opacity: 0.12,
        pointerEvents: 'none',
      }} />
      <div style={{
        position: 'fixed',
        top: '40%',
        left: '52%',
        transform: 'translate(-50%, -50%)',
        width: 320,
        height: 320,
        background: '#C9A961',
        borderRadius: '50%',
        filter: 'blur(150px)',
        opacity: 0.1,
        pointerEvents: 'none',
      }} />

      {/* Brand Header with Glowing Crown Pill */}
      <div style={{ textAlign: 'center', marginBottom: 24, zIndex: 1 }}>
        <div
          onClick={() => navigate('/')}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 8,
            padding: '7px 18px',
            background: 'rgba(255, 255, 255, 0.82)',
            backdropFilter: 'blur(20px)',
            WebkitBackdropFilter: 'blur(20px)',
            border: '1px solid rgba(255, 255, 255, 0.85)',
            borderRadius: 9999,
            boxShadow: '0 4px 18px rgba(0, 0, 0, 0.04)',
            cursor: 'pointer',
            transition: 'all 0.15s ease',
            marginBottom: 8,
          }}
          onMouseEnter={e => {
            e.currentTarget.style.transform = 'translateY(-1px)'
            e.currentTarget.style.boxShadow = '0 6px 22px rgba(0, 0, 0, 0.07)'
          }}
          onMouseLeave={e => {
            e.currentTarget.style.transform = 'translateY(0)'
            e.currentTarget.style.boxShadow = '0 4px 18px rgba(0, 0, 0, 0.04)'
          }}
        >
          <Fa
            icon="fa-crown"
            style={{
              fontSize: 18,
              color: 'var(--reyna-accent, #25D366)',
              filter: 'drop-shadow(0 0 8px rgba(37,211,102,0.55))',
            }}
          />
          <span style={{ fontSize: 19, fontWeight: 800, letterSpacing: -0.5, color: '#111827' }}>
            reyna
          </span>
          <span style={{
            fontSize: 10,
            fontWeight: 700,
            color: '#065f46',
            background: 'rgba(16, 185, 129, 0.12)',
            padding: '2px 7px',
            borderRadius: 9999,
          }}>
            v2
          </span>
        </div>
        <p style={{ fontSize: 13, color: '#6b7280', margin: 0, fontWeight: 500, letterSpacing: '-0.01em' }}>
          your group chat's knowledge base
        </p>
      </div>

      <div style={{ width: '100%', maxWidth: 510, position: 'relative', zIndex: 1 }}>
        {/* Main Frosted Glass Card */}
        <div style={{
          background: 'rgba(255, 255, 255, 0.82)',
          backdropFilter: 'blur(28px) saturate(180%)',
          WebkitBackdropFilter: 'blur(28px) saturate(180%)',
          border: '1px solid rgba(255, 255, 255, 0.8)',
          borderRadius: 24,
          padding: '38px 38px',
          boxShadow: '0 24px 60px rgba(0, 0, 0, 0.05), 0 2px 8px rgba(0, 0, 0, 0.02)',
          position: 'relative',
          overflow: 'hidden',
        }}>
          {/* Subtle Cat Scratch Mark in Card Corner */}
          <ClawMark
            size={56}
            color="rgba(16, 185, 129, 0.14)"
            style={{
              position: 'absolute',
              top: 14,
              right: 18,
              transform: 'rotate(24deg)',
            }}
          />

          {/* Card Header */}
          <h2 style={{
            fontSize: 26,
            fontWeight: 800,
            color: '#111827',
            margin: '0 0 24px 0',
            letterSpacing: '-0.025em',
            lineHeight: 1.25,
          }}>
            {isRegister ? 'create a new account' : 'log in to an existing account'}
          </h2>

          <form onSubmit={step === 1 ? handleContinue : handleSubmit}>
            {/* Field 1: username or whatsapp number */}
            <div style={{ marginBottom: 18 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
                <label style={{
                  fontSize: 13,
                  fontWeight: 600,
                  color: '#374151',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  letterSpacing: '-0.01em',
                }}>
                  <Fa icon="fa-user" style={{ fontSize: 11, color: '#10b981' }} />
                  username or whatsapp number
                </label>
                <span style={{ fontSize: 11, color: '#9ca3af', fontWeight: 500 }}>
                  phone or handle
                </span>
              </div>

              <input
                type="text"
                value={phone}
                readOnly={step === 2}
                onChange={e => setPhone(e.target.value)}
                placeholder="harsh or +91 9876543210"
                style={{
                  width: '100%',
                  padding: '12px 16px',
                  border: '1.5px solid #e5e7eb',
                  borderRadius: 14,
                  fontSize: 14,
                  outline: 'none',
                  boxSizing: 'border-box',
                  fontFamily: 'inherit',
                  background: step === 2 ? 'rgba(243, 244, 246, 0.8)' : '#ffffff',
                  color: step === 2 ? '#6b7280' : '#111827',
                  cursor: step === 2 ? 'default' : 'text',
                  transition: 'all 0.15s ease',
                  boxShadow: '0 1px 2px rgba(0,0,0,0.02)',
                }}
                onFocus={e => {
                  if (step === 1) {
                    e.currentTarget.style.borderColor = '#0d3824'
                    e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.08)'
                  }
                }}
                onBlur={e => {
                  if (step === 1) {
                    e.currentTarget.style.borderColor = '#e5e7eb'
                    e.currentTarget.style.boxShadow = '0 1px 2px rgba(0,0,0,0.02)'
                  }
                }}
              />

              {step === 2 && (
                <div style={{ marginTop: 8 }}>
                  <button
                    type="button"
                    onClick={() => { setStep(1); setError(''); }}
                    style={{
                      background: 'none',
                      border: 'none',
                      color: '#059669',
                      fontSize: 13,
                      cursor: 'pointer',
                      padding: 0,
                      fontWeight: 600,
                      textDecoration: 'underline',
                      fontFamily: 'inherit',
                    }}
                  >
                    use a different account
                  </button>
                </div>
              )}
            </div>

            {/* Step 2: Smooth Slide-In Password / Details (Porkbun animation) */}
            {step === 2 && (
              <div className="porkbun-slide-in" style={{ marginBottom: 20 }}>
                <div style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  fontSize: 14,
                  fontWeight: 700,
                  color: '#111827',
                  marginBottom: isRegister ? 4 : 16,
                  marginTop: 18,
                }}>
                  <Fa icon={icons.lock} style={{ fontSize: 12, color: '#059669' }} />
                  {isRegister ? 'next, enter your details.' : 'next, confirm your sign in.'}
                </div>
                {isRegister && (
                  <p style={{ fontSize: 13, color: '#6b7280', margin: '0 0 16px 0', lineHeight: 1.45 }}>
                    choose your display name to finish setting up your workspace.
                  </p>
                )}

                {isRegister && (
                  <div style={{ marginBottom: 16 }}>
                    <label style={{
                      fontSize: 13,
                      fontWeight: 600,
                      color: '#374151',
                      display: 'block',
                      marginBottom: 8,
                      letterSpacing: '-0.01em',
                    }}>
                      full name
                    </label>
                    <input
                      type="text"
                      value={name}
                      onChange={e => setName(e.target.value)}
                      placeholder="e.g. harsh narayan"
                      autoFocus
                      style={{
                        width: '100%',
                        padding: '12px 16px',
                        border: '1.5px solid #e5e7eb',
                        borderRadius: 14,
                        fontSize: 14,
                        outline: 'none',
                        boxSizing: 'border-box',
                        background: '#ffffff',
                        color: '#111827',
                        fontFamily: 'inherit',
                        transition: 'all 0.15s ease',
                      }}
                      onFocus={e => {
                        e.currentTarget.style.borderColor = '#0d3824'
                        e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.08)'
                      }}
                      onBlur={e => {
                        e.currentTarget.style.borderColor = '#e5e7eb'
                        e.currentTarget.style.boxShadow = 'none'
                      }}
                    />
                  </div>
                )}

                <div style={{ marginBottom: 16 }}>
                  <label style={{
                    fontSize: 13,
                    fontWeight: 600,
                    color: '#374151',
                    display: 'block',
                    marginBottom: 8,
                    letterSpacing: '-0.01em',
                  }}>
                    password or pin (optional)
                  </label>
                  <div style={{ display: 'flex', position: 'relative' }}>
                    <input
                      type={showPassword ? 'text' : 'password'}
                      value={password}
                      onChange={e => setPassword(e.target.value)}
                      placeholder="leave blank or enter password"
                      autoFocus={!isRegister}
                      style={{
                        flex: 1,
                        padding: '12px 16px',
                        border: '1.5px solid #e5e7eb',
                        borderRight: 'none',
                        borderRadius: '14px 0 0 14px',
                        fontSize: 14,
                        outline: 'none',
                        boxSizing: 'border-box',
                        background: '#ffffff',
                        color: '#111827',
                        fontFamily: 'inherit',
                        transition: 'all 0.15s ease',
                      }}
                      onFocus={e => {
                        e.currentTarget.style.borderColor = '#0d3824'
                        e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.08)'
                        if (e.currentTarget.nextElementSibling) {
                          e.currentTarget.nextElementSibling.style.borderColor = '#0d3824'
                        }
                      }}
                      onBlur={e => {
                        e.currentTarget.style.borderColor = '#e5e7eb'
                        e.currentTarget.style.boxShadow = 'none'
                        if (e.currentTarget.nextElementSibling) {
                          e.currentTarget.nextElementSibling.style.borderColor = '#e5e7eb'
                        }
                      }}
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      style={{
                        padding: '0 16px',
                        background: '#f9fafb',
                        border: '1.5px solid #e5e7eb',
                        borderRadius: '0 14px 14px 0',
                        cursor: 'pointer',
                        color: '#6b7280',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        transition: 'all 0.15s ease',
                      }}
                      title={showPassword ? 'hide password' : 'show password'}
                    >
                      <Fa icon={showPassword ? 'fa-eye-slash' : 'fa-eye'} style={{ fontSize: 13 }} />
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Checkbox: remember me */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, margin: '14px 0 22px 0' }}>
              <input
                type="checkbox"
                id="rememberMe"
                checked={rememberMe}
                onChange={e => setRememberMe(e.target.checked)}
                style={{
                  width: 16,
                  height: 16,
                  accentColor: '#0d3824',
                  cursor: 'pointer',
                  borderRadius: 4,
                }}
              />
              <label htmlFor="rememberMe" style={{ fontSize: 13, color: '#4b5563', cursor: 'pointer', userSelect: 'none' }}>
                remember me for 30 days <span style={{ color: '#9ca3af' }}>(not on a shared or public device)</span>
              </label>
            </div>

            {/* Error Message */}
            {error && (
              <div style={{
                fontSize: 13,
                color: '#dc2626',
                background: '#fef2f2',
                border: '1px solid #fecaca',
                borderRadius: 12,
                padding: '10px 14px',
                marginBottom: 18,
                display: 'flex',
                alignItems: 'center',
                gap: 8,
              }}>
                <Fa icon={icons.warning} style={{ fontSize: 12 }} />
                <span>{error}</span>
              </div>
            )}

            {/* Action Buttons Row */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
              {step === 1 ? (
                <>
                  <button
                    type="submit"
                    style={{
                      background: '#0d3824',
                      border: '1.5px solid #0d3824',
                      borderRadius: 9999,
                      color: '#ffffff',
                      padding: '11px 28px',
                      fontSize: 14,
                      fontWeight: 600,
                      cursor: 'pointer',
                      transition: 'all 0.15s ease',
                      fontFamily: 'inherit',
                      boxShadow: '0 2px 8px rgba(13, 56, 36, 0.2)',
                    }}
                    onMouseEnter={e => {
                      e.currentTarget.style.background = '#14532d'
                      e.currentTarget.style.borderColor = '#14532d'
                    }}
                    onMouseLeave={e => {
                      e.currentTarget.style.background = '#0d3824'
                      e.currentTarget.style.borderColor = '#0d3824'
                    }}
                  >
                    continue
                  </button>
                  <span style={{ fontSize: 13, color: '#9ca3af' }}>or</span>
                  <button
                    type="button"
                    onClick={() => toggleMode(!isRegister)}
                    style={{
                      background: '#ffffff',
                      border: '1.5px solid #e5e7eb',
                      borderRadius: 9999,
                      color: '#374151',
                      padding: '11px 22px',
                      fontSize: 14,
                      fontWeight: 600,
                      cursor: 'pointer',
                      transition: 'all 0.15s ease',
                      fontFamily: 'inherit',
                    }}
                    onMouseEnter={e => {
                      e.currentTarget.style.borderColor = '#9ca3af'
                      e.currentTarget.style.background = '#f9fafb'
                    }}
                    onMouseLeave={e => {
                      e.currentTarget.style.borderColor = '#e5e7eb'
                      e.currentTarget.style.background = '#ffffff'
                    }}
                  >
                    {isRegister ? 'log in' : 'create a new account'}
                  </button>
                </>
              ) : (
                <>
                  {/* Step 2 button with revolving XOXO micro-interaction */}
                  <button
                    type="submit"
                    disabled={loading}
                    style={{
                      background: loading ? '#f0fdf4' : '#0d3824',
                      border: loading ? '1.5px solid #86efac' : '1.5px solid #0d3824',
                      borderRadius: 9999,
                      color: loading ? '#15803d' : '#ffffff',
                      padding: '11px 30px',
                      fontSize: 14,
                      fontWeight: 700,
                      cursor: loading ? 'not-allowed' : 'pointer',
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 8,
                      transition: 'all 0.18s ease',
                      fontFamily: 'inherit',
                      boxShadow: '0 2px 8px rgba(13, 56, 36, 0.2)',
                    }}
                    onMouseEnter={e => {
                      if (!loading) {
                        e.currentTarget.style.background = '#14532d'
                        e.currentTarget.style.borderColor = '#14532d'
                      }
                    }}
                    onMouseLeave={e => {
                      if (!loading) {
                        e.currentTarget.style.background = '#0d3824'
                        e.currentTarget.style.borderColor = '#0d3824'
                      }
                    }}
                  >
                    {loading ? (
                      <>
                        <span className="porkbun-spinner" style={{ borderTopColor: '#15803d' }} />
                        <span>xoxo</span>
                      </>
                    ) : (
                      isRegister ? 'create account' : 'log in'
                    )}
                  </button>
                  <span style={{ fontSize: 13, color: '#9ca3af' }}>or</span>
                  <button
                    type="button"
                    onClick={() => toggleMode(!isRegister)}
                    style={{
                      background: '#ffffff',
                      border: '1.5px solid #e5e7eb',
                      borderRadius: 9999,
                      color: '#374151',
                      padding: '11px 22px',
                      fontSize: 14,
                      fontWeight: 600,
                      cursor: 'pointer',
                      transition: 'all 0.15s ease',
                      fontFamily: 'inherit',
                    }}
                    onMouseEnter={e => {
                      e.currentTarget.style.borderColor = '#9ca3af'
                      e.currentTarget.style.background = '#f9fafb'
                    }}
                    onMouseLeave={e => {
                      e.currentTarget.style.borderColor = '#e5e7eb'
                      e.currentTarget.style.background = '#ffffff'
                    }}
                  >
                    {isRegister ? 'log in to an existing account' : 'create a new account'}
                  </button>
                </>
              )}
            </div>

            {/* Sub-links */}
            <div style={{ marginTop: 20 }}>
              <span
                onClick={() => alert('to reset or recover credentials, authenticate with your linked whatsapp number.')}
                style={{
                  fontSize: 12,
                  color: '#6b7280',
                  textDecoration: 'underline',
                  cursor: 'pointer',
                }}
              >
                forgotten password, 2fa, or security key.
              </span>
            </div>
          </form>

          {/* Legal / Policy Box */}
          <div style={{
            background: 'rgba(249, 250, 251, 0.65)',
            border: '1px solid rgba(229, 231, 235, 0.6)',
            borderRadius: 16,
            padding: '16px 20px',
            marginTop: 26,
            fontSize: 12,
            color: '#6b7280',
            lineHeight: 1.5,
          }}>
            <div style={{ fontWeight: 600, color: '#374151', marginBottom: 6 }}>
              by continuing you agree to the following:
            </div>
            <p style={{ margin: '0 0 8px 0', lineHeight: 1.5 }}>
              i acknowledge that reyna only captures, stages, and indexes files shared in authorized whatsapp groups, in accordance with the product terms of service and privacy policy.
            </p>
            <p style={{ margin: 0, color: '#9ca3af' }}>
              the lawyers made us say all of that ☝️
            </p>
          </div>
        </div>
      </div>

      {/* Interactive Bottom-Left Cat Corner Vignette (like Porkbun's spiderweb) */}
      <CornerCat />
    </div>
  )
}
