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
          setError('no account found with this number. enter your details to create one.')
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
    }}>
      {/* Brand Header */}
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <h1
          onClick={() => navigate('/')}
          style={{
            fontSize: 25,
            fontWeight: 800,
            letterSpacing: -0.6,
            margin: '0 0 6px 0',
            color: '#111827',
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            cursor: 'pointer',
          }}
        >
          <Fa icon="fa-crown" style={{ fontSize: 22, color: 'var(--reyna-accent, #25D366)', marginRight: 8 }} />
          reyna
          <span style={{ fontSize: 11, color: '#6b7280', fontWeight: 500, marginLeft: 5 }}>v2</span>
        </h1>
        <p style={{ fontSize: 13, color: '#6b7280', margin: 0, fontWeight: 400 }}>your group chat's knowledge base</p>
      </div>

      <div style={{ width: '100%', maxWidth: 520 }}>
        {/* Main Card */}
        <div style={{
          background: 'var(--card-bg, #ffffff)',
          border: '1px solid #e5e7eb',
          borderRadius: 20,
          padding: '36px 36px',
          boxShadow: '0 4px 24px rgba(0, 0, 0, 0.04)',
        }}>
          {/* Header */}
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
            {/* Field 1: Username / WhatsApp Number */}
            <div style={{ marginBottom: 18 }}>
              <label style={{
                fontSize: 13,
                fontWeight: 600,
                color: '#374151',
                display: 'block',
                letterSpacing: '-0.01em',
                marginBottom: 8,
              }}>
                whatsapp number or username
              </label>

              <input
                type="text"
                value={phone}
                readOnly={step === 2}
                onChange={e => setPhone(e.target.value)}
                placeholder="+91 9876543210 or harsh"
                style={{
                  width: '100%',
                  padding: '11px 16px',
                  border: '1.5px solid #e5e7eb',
                  borderRadius: 12,
                  fontSize: 14,
                  outline: 'none',
                  boxSizing: 'border-box',
                  fontFamily: 'inherit',
                  background: step === 2 ? '#f9fafb' : '#ffffff',
                  color: step === 2 ? '#6b7280' : '#111827',
                  cursor: step === 2 ? 'default' : 'text',
                  transition: 'all 0.15s ease',
                }}
                onFocus={e => {
                  if (step === 1) {
                    e.currentTarget.style.borderColor = '#0d3824'
                    e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.06)'
                  }
                }}
                onBlur={e => {
                  if (step === 1) {
                    e.currentTarget.style.borderColor = '#e5e7eb'
                    e.currentTarget.style.boxShadow = 'none'
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
                      fontWeight: 500,
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
                        padding: '11px 16px',
                        border: '1.5px solid #e5e7eb',
                        borderRadius: 12,
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
                        e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.06)'
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
                        padding: '11px 16px',
                        border: '1.5px solid #e5e7eb',
                        borderRight: 'none',
                        borderRadius: '12px 0 0 12px',
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
                        e.currentTarget.style.boxShadow = '0 0 0 3px rgba(13, 56, 36, 0.06)'
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
                        borderRadius: '0 12px 12px 0',
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

            {/* Checkbox: Remember me */}
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
                borderRadius: 10,
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
                      padding: '10px 26px',
                      fontSize: 14,
                      fontWeight: 600,
                      cursor: 'pointer',
                      transition: 'all 0.15s ease',
                      fontFamily: 'inherit',
                      boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
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
                      padding: '10px 20px',
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
                      padding: '10px 28px',
                      fontSize: 14,
                      fontWeight: 700,
                      cursor: loading ? 'not-allowed' : 'pointer',
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 8,
                      transition: 'all 0.18s ease',
                      fontFamily: 'inherit',
                      boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
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
                      padding: '10px 20px',
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
            background: '#fafaf9',
            border: '1px solid #f0eee9',
            borderRadius: 14,
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
    </div>
  )
}
