import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, saveAuth, isLoggedIn } from '../lib/api'
import { Fa, icons } from '../components/icons'

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
      padding: '48px 20px',
      fontFamily: 'var(--font, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif)',
      color: '#1a1a1a',
      position: 'relative',
      overflow: 'hidden',
    }}>
      {/* Subtle Aging Paper Grain Filter Overlay */}
      <svg
        style={{
          position: 'fixed',
          top: 0,
          left: 0,
          width: '100%',
          height: '100%',
          pointerEvents: 'none',
          opacity: 0.04,
          zIndex: 1,
        }}
      >
        <filter id="reyna-grain">
          <feTurbulence type="fractalNoise" baseFrequency="0.75" numOctaves="3" stitchTiles="stitch" />
          <feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 1 0" />
        </filter>
        <rect width="100%" height="100%" filter="url(#reyna-grain)" />
      </svg>

      {/* Stripe-style Faint Dotted/Dashed Vertical Guidelines */}
      <div
        style={{
          position: 'fixed',
          top: 0,
          bottom: 0,
          left: 0,
          right: 0,
          pointerEvents: 'none',
          zIndex: 0,
          userSelect: 'none',
        }}
      >
        {/* Form boundary guidelines (framing the login column path) */}
        <div style={{ position: 'absolute', top: 0, bottom: 0, left: 'calc(50% - 205px)', borderLeft: '1px dashed rgba(0, 0, 0, 0.06)' }} />
        <div style={{ position: 'absolute', top: 0, bottom: 0, left: 'calc(50% + 205px)', borderLeft: '1px dashed rgba(0, 0, 0, 0.06)' }} />
        {/* Flanking proportional column guidelines */}
        <div style={{ position: 'absolute', top: 0, bottom: 0, left: 'calc(50% - 365px)', borderLeft: '1px dashed rgba(0, 0, 0, 0.04)' }} />
        <div style={{ position: 'absolute', top: 0, bottom: 0, left: 'calc(50% + 365px)', borderLeft: '1px dashed rgba(0, 0, 0, 0.04)' }} />
        <div style={{ position: 'absolute', top: 0, bottom: 0, left: 'calc(50% - 525px)', borderLeft: '1px dashed rgba(0, 0, 0, 0.025)' }} />
        <div style={{ position: 'absolute', top: 0, bottom: 0, left: 'calc(50% + 525px)', borderLeft: '1px dashed rgba(0, 0, 0, 0.025)' }} />
      </div>

      {/* Ambient Green Glowing Orbs */}
      <div style={{
        position: 'fixed',
        top: '6%',
        right: '12%',
        width: 520,
        height: 520,
        background: 'radial-gradient(circle, rgba(37, 211, 102, 0.24) 0%, rgba(37, 211, 102, 0) 68%)',
        filter: 'blur(35px)',
        pointerEvents: 'none',
        zIndex: 0,
      }} />
      <div style={{
        position: 'fixed',
        bottom: '8%',
        left: '6%',
        width: 440,
        height: 440,
        background: 'radial-gradient(circle, rgba(37, 211, 102, 0.12) 0%, rgba(37, 211, 102, 0) 68%)',
        filter: 'blur(45px)',
        pointerEvents: 'none',
        zIndex: 0,
      }} />

      {/* Hand-drawn Cat Paws (Top-Left upside-down & Bottom-Right) */}
      {/* 1. Top-Left Upside-Down Paw */}
      <img
        src="/cat-paw.png"
        alt=""
        style={{
          position: 'fixed',
          top: -20,
          left: -20,
          width: 380,
          opacity: 0.16,
          transform: 'rotate(180deg)',
          mixBlendMode: 'multiply',
          pointerEvents: 'none',
          zIndex: 0,
          userSelect: 'none',
        }}
      />

      {/* 2. Bottom-Right Paw */}
      <img
        src="/cat-paw.png"
        alt=""
        style={{
          position: 'fixed',
          bottom: -15,
          right: -15,
          width: 390,
          opacity: 0.18,
          mixBlendMode: 'multiply',
          pointerEvents: 'none',
          zIndex: 0,
          userSelect: 'none',
        }}
      />

      {/* Main Integrated Form Area (aligned and visually balanced) */}
      <div style={{ width: '100%', maxWidth: 410, position: 'relative', zIndex: 2 }}>
        {/* Brand Header */}
        <div style={{ marginBottom: 24 }}>
          <div
            onClick={() => navigate('/')}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              cursor: 'pointer',
              marginBottom: 4,
            }}
          >
            <Fa
              icon="fa-crown"
              style={{
                fontSize: 17,
                color: 'var(--reyna-accent, #25D366)',
                marginRight: 2,
              }}
            />
            <span style={{ fontSize: 20, fontWeight: 800, letterSpacing: -0.4, color: '#111827' }}>
              reyna
            </span>
            <span style={{
              fontSize: 10,
              fontWeight: 700,
              color: '#059669',
              background: '#dcfce7',
              padding: '2px 7px',
              borderRadius: 9999,
              marginLeft: 2,
            }}>
              v2
            </span>
          </div>
          <p style={{ fontSize: 13, color: '#6b7280', margin: 0, fontWeight: 400 }}>
            your group chat's knowledge base
          </p>
        </div>

        {/* Heading (wrapped across 2 lines with balanced size and leading) */}
        <h2 style={{
          fontSize: 32,
          fontWeight: 800,
          color: '#111827',
          margin: '0 0 20px 0',
          letterSpacing: '-0.03em',
          lineHeight: 1.15,
        }}>
          {isRegister ? (
            <>
              create a new<br />account
            </>
          ) : (
            <>
              log in to an existing<br />account
            </>
          )}
        </h2>

        {/* Form Fields */}
        <form onSubmit={step === 1 ? handleContinue : handleSubmit}>
          {/* Field 1: username or whatsapp number */}
          <div style={{ marginBottom: 16 }}>
            <label style={{
              fontSize: 13,
              fontWeight: 600,
              color: '#374151',
              display: 'block',
              marginBottom: 8,
              letterSpacing: '-0.01em',
              paddingLeft: 2,
            }}>
              username or whatsapp number
            </label>

            <input
              type="text"
              value={phone}
              readOnly={step === 2}
              onChange={e => setPhone(e.target.value)}
              placeholder="harsh or +91 9876543210"
              style={{
                width: '100%',
                padding: '13px 22px',
                border: '1.5px solid #d1d5db',
                borderRadius: 9999,
                fontSize: 14,
                outline: 'none',
                boxSizing: 'border-box',
                fontFamily: 'inherit',
                background: step === 2 ? '#f3f4f6' : 'rgba(255, 255, 255, 0.94)',
                color: step === 2 ? '#6b7280' : '#111827',
                cursor: step === 2 ? 'default' : 'text',
                transition: 'all 0.15s ease',
                boxShadow: 'inset 0 1px 2px rgba(0,0,0,0.03), 0 1px 2px rgba(0,0,0,0.02)',
              }}
              onFocus={e => {
                if (step === 1) {
                  e.currentTarget.style.borderColor = '#0d3824'
                  e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.08)'
                }
              }}
              onBlur={e => {
                if (step === 1) {
                  e.currentTarget.style.borderColor = '#d1d5db'
                  e.currentTarget.style.boxShadow = 'inset 0 1px 2px rgba(0,0,0,0.03), 0 1px 2px rgba(0,0,0,0.02)'
                }
              }}
            />

            {step === 2 && (
              <div style={{ marginTop: 8, paddingLeft: 6 }}>
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
            <div className="porkbun-slide-in" style={{ marginBottom: 18 }}>
              <div style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                fontSize: 14,
                fontWeight: 700,
                color: '#111827',
                marginBottom: isRegister ? 4 : 14,
                marginTop: 16,
              }}>
                <Fa icon={icons.lock} style={{ fontSize: 12, color: '#059669' }} />
                {isRegister ? 'next, enter your details.' : 'next, confirm your sign in.'}
              </div>
              {isRegister && (
                <p style={{ fontSize: 13, color: '#6b7280', margin: '0 0 14px 0', lineHeight: 1.45 }}>
                  choose your display name to finish setting up your workspace.
                </p>
              )}

              {isRegister && (
                <div style={{ marginBottom: 14 }}>
                  <label style={{
                    fontSize: 13,
                    fontWeight: 600,
                    color: '#374151',
                    display: 'block',
                    marginBottom: 8,
                    letterSpacing: '-0.01em',
                    paddingLeft: 2,
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
                      padding: '13px 22px',
                      border: '1.5px solid #d1d5db',
                      borderRadius: 9999,
                      fontSize: 14,
                      outline: 'none',
                      boxSizing: 'border-box',
                      background: 'rgba(255, 255, 255, 0.94)',
                      color: '#111827',
                      fontFamily: 'inherit',
                      transition: 'all 0.15s ease',
                      boxShadow: 'inset 0 1px 2px rgba(0,0,0,0.03), 0 1px 2px rgba(0,0,0,0.02)',
                    }}
                    onFocus={e => {
                      e.currentTarget.style.borderColor = '#0d3824'
                      e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.08)'
                    }}
                    onBlur={e => {
                      e.currentTarget.style.borderColor = '#d1d5db'
                      e.currentTarget.style.boxShadow = 'inset 0 1px 2px rgba(0,0,0,0.03), 0 1px 2px rgba(0,0,0,0.02)'
                    }}
                  />
                </div>
              )}

              <div style={{ marginBottom: 14 }}>
                <label style={{
                  fontSize: 13,
                  fontWeight: 600,
                  color: '#374151',
                  display: 'block',
                  marginBottom: 8,
                  letterSpacing: '-0.01em',
                  paddingLeft: 2,
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
                      padding: '13px 22px',
                      border: '1.5px solid #d1d5db',
                      borderRight: 'none',
                      borderRadius: '9999px 0 0 9999px',
                      fontSize: 14,
                      outline: 'none',
                      boxSizing: 'border-box',
                      background: 'rgba(255, 255, 255, 0.94)',
                      color: '#111827',
                      fontFamily: 'inherit',
                      transition: 'all 0.15s ease',
                      boxShadow: 'inset 0 1px 2px rgba(0,0,0,0.03)',
                    }}
                    onFocus={e => {
                      e.currentTarget.style.borderColor = '#0d3824'
                      e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.08)'
                      if (e.currentTarget.nextElementSibling) {
                        e.currentTarget.nextElementSibling.style.borderColor = '#0d3824'
                      }
                    }}
                    onBlur={e => {
                      e.currentTarget.style.borderColor = '#d1d5db'
                      e.currentTarget.style.boxShadow = 'inset 0 1px 2px rgba(0,0,0,0.03)'
                      if (e.currentTarget.nextElementSibling) {
                        e.currentTarget.nextElementSibling.style.borderColor = '#d1d5db'
                      }
                    }}
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    style={{
                      padding: '0 20px',
                      background: '#f9fafb',
                      border: '1.5px solid #d1d5db',
                      borderRadius: '0 9999px 9999px 0',
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
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, margin: '14px 0 20px 0', paddingLeft: 2 }}>
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
            <label htmlFor="rememberMe" style={{ fontSize: 13, color: '#374151', cursor: 'pointer', userSelect: 'none' }}>
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
              marginBottom: 16,
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
                    padding: '10px 28px',
                    fontSize: 14,
                    fontWeight: 600,
                    cursor: 'pointer',
                    transition: 'all 0.15s ease',
                    fontFamily: 'inherit',
                    boxShadow: '0 2px 6px rgba(13, 56, 36, 0.2)',
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
                    border: '1.5px solid #d1d5db',
                    borderRadius: 9999,
                    color: '#111827',
                    padding: '10px 22px',
                    fontSize: 14,
                    fontWeight: 600,
                    cursor: 'pointer',
                    transition: 'all 0.15s ease',
                    fontFamily: 'inherit',
                    boxShadow: '0 1px 2px rgba(0,0,0,0.02)',
                  }}
                  onMouseEnter={e => {
                    e.currentTarget.style.borderColor = '#9ca3af'
                    e.currentTarget.style.background = '#f9fafb'
                  }}
                  onMouseLeave={e => {
                    e.currentTarget.style.borderColor = '#d1d5db'
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
                    padding: '10px 30px',
                    fontSize: 14,
                    fontWeight: 700,
                    cursor: loading ? 'not-allowed' : 'pointer',
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 8,
                    transition: 'all 0.18s ease',
                    fontFamily: 'inherit',
                    boxShadow: '0 2px 6px rgba(13, 56, 36, 0.2)',
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
                    color: '#111827',
                    padding: '10px 22px',
                    fontSize: 14,
                    fontWeight: 600,
                    cursor: 'pointer',
                    transition: 'all 0.15s ease',
                    fontFamily: 'inherit',
                    boxShadow: '0 1px 2px rgba(0,0,0,0.02)',
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
          <div style={{ marginTop: 18, paddingLeft: 2 }}>
            <span
              onClick={() => alert('to reset or recover credentials, authenticate with your linked whatsapp number.')}
              style={{
                fontSize: 12,
                color: '#6b7280',
                textDecoration: 'underline',
                cursor: 'pointer',
              }}
            >
              forgotten password, 2fa, or security key
            </span>
          </div>
        </form>

        {/* Legal / Policy Disclaimer (perfectly proportioned to reference mockup) */}
        <div style={{
          borderTop: '1px solid #e5e7eb',
          paddingTop: 20,
          marginTop: 24,
          fontSize: 11,
          color: '#6b7280',
          lineHeight: 1.55,
        }}>
          <div style={{
            fontSize: 11,
            fontWeight: 700,
            color: '#374151',
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
            marginBottom: 6,
          }}>
            by continuing you agree to the following:
          </div>
          <p style={{ margin: '0 0 6px 0', lineHeight: 1.55 }}>
            i acknowledge that reyna only captures, stages, and indexes files shared in authorized whatsapp groups, in accordance with the product terms of service and privacy policy.
          </p>
          <p style={{ margin: 0, color: '#6b7280' }}>
            the lawyers made us say all of that ✌️
          </p>
        </div>
      </div>
    </div>
  )
}
