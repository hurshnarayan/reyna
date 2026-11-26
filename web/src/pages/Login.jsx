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
      setError('Please enter your username or WhatsApp number.')
      return
    }
    // Smoothly slide in Step 2 (Password / Details)
    setStep(2)
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const data = isRegister
        ? await api.register(phone.trim(), name.trim() || 'Reyna User')
        : await api.login(phone.trim())

      if (data?.error) {
        if (data.error.includes('not found')) {
          setIsRegister(true)
          setStep(2)
          setError('No account found with this number. Enter your details to create one.')
        } else {
          setError(data.error)
        }
        setLoading(false)
      } else {
        saveAuth(data)
        // Hold the playful XOXO revolving state briefly for fluid feel
        setTimeout(() => {
          navigate('/dashboard')
        }, 700)
      }
    } catch {
      setError('Connection failed. Is the backend server running?')
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
      color: '#111827',
    }}>
      {/* Brand Header */}
      <div style={{ textAlign: 'center', marginBottom: 28 }}>
        <h1
          onClick={() => navigate('/')}
          style={{
            fontSize: 24,
            fontWeight: 800,
            letterSpacing: -0.5,
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
          <span style={{ fontSize: 11, color: '#6b7280', fontWeight: 500, marginLeft: 4 }}>v2</span>
        </h1>
        <p style={{ fontSize: 13, color: '#4b5563', margin: 0 }}>your group chat's knowledge base</p>
      </div>

      <div style={{ width: '100%', maxWidth: 520 }}>
        {/* Top Toggle Button (Porkbun style) */}
        <div style={{ marginBottom: 16 }}>
          <button
            type="button"
            onClick={() => toggleMode(!isRegister)}
            style={{
              background: '#ffffff',
              border: '1px solid #10b981',
              borderRadius: 6,
              padding: '8px 18px',
              fontSize: 14,
              fontWeight: 600,
              color: '#064e3b',
              cursor: 'pointer',
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              transition: 'all 0.15s ease',
              fontFamily: 'inherit',
              boxShadow: '0 1px 2px rgba(0,0,0,0.04)',
            }}
            onMouseEnter={e => {
              e.currentTarget.style.background = '#f0fdf4'
              e.currentTarget.style.borderColor = '#059669'
            }}
            onMouseLeave={e => {
              e.currentTarget.style.background = '#ffffff'
              e.currentTarget.style.borderColor = '#10b981'
            }}
          >
            {isRegister ? 'Log in to an Existing Account' : 'Create a New Account'}
          </button>
        </div>

        {/* Main Porkbun-style Card */}
        <div style={{
          background: '#ffffff',
          border: '1px solid #d1d5db',
          borderRadius: 8,
          padding: '36px 40px',
          boxShadow: '0 4px 20px rgba(0, 0, 0, 0.05)',
        }}>
          <h2 style={{
            fontSize: 26,
            fontWeight: 700,
            color: '#111827',
            margin: '0 0 24px 0',
            letterSpacing: '-0.02em',
            lineHeight: 1.2,
          }}>
            {isRegister ? 'Create a New Account' : 'Log in to an Existing Account'}
          </h2>

          <form onSubmit={step === 1 ? handleContinue : handleSubmit}>
            {/* Field 1: Username / WhatsApp Number */}
            <div style={{ marginBottom: 16 }}>
              <label style={{
                fontSize: 11,
                fontWeight: 700,
                color: '#111827',
                display: 'block',
                textTransform: 'uppercase',
                letterSpacing: '0.04em',
                marginBottom: 6,
              }}>
                USERNAME OR WHATSAPP NUMBER
              </label>

              <input
                type="text"
                value={phone}
                readOnly={step === 2}
                onChange={e => setPhone(e.target.value)}
                placeholder="+91 9876543210"
                style={{
                  width: '100%',
                  padding: '10px 14px',
                  border: '1px solid #d1d5db',
                  borderRadius: 6,
                  fontSize: 15,
                  outline: 'none',
                  boxSizing: 'border-box',
                  fontFamily: 'var(--font-mono, monospace)',
                  background: step === 2 ? '#f3f4f6' : '#ffffff',
                  color: step === 2 ? '#4b5563' : '#111827',
                  cursor: step === 2 ? 'default' : 'text',
                  transition: 'border-color 0.15s, background 0.15s',
                }}
                onFocus={e => {
                  if (step === 1) e.currentTarget.style.borderColor = '#0d3824'
                }}
                onBlur={e => {
                  if (step === 1) e.currentTarget.style.borderColor = '#d1d5db'
                }}
              />

              {step === 2 && (
                <div style={{ marginTop: 6 }}>
                  <button
                    type="button"
                    onClick={() => { setStep(1); setError(''); }}
                    style={{
                      background: 'none',
                      border: 'none',
                      color: '#064e3b',
                      fontSize: 13,
                      cursor: 'pointer',
                      padding: 0,
                      fontWeight: 500,
                      textDecoration: 'underline',
                      fontFamily: 'inherit',
                    }}
                  >
                    Use a different account
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
                  marginBottom: 4,
                  marginTop: 18,
                }}>
                  <Fa icon={icons.lock} style={{ fontSize: 13, color: '#111827' }} />
                  {isRegister ? 'Next, enter your details.' : 'Next, enter your password.'}
                </div>
                <p style={{ fontSize: 12, color: '#6b7280', margin: '0 0 16px 0', lineHeight: 1.45 }}>
                  {isRegister
                    ? 'Choose your display name and credentials to finish setting up your workspace.'
                    : 'If this account also uses two-factor or a security key, we\'ll ask for that after.'}
                </p>

                {isRegister && (
                  <div style={{ marginBottom: 14 }}>
                    <label style={{
                      fontSize: 11,
                      fontWeight: 700,
                      color: '#111827',
                      display: 'block',
                      textTransform: 'uppercase',
                      letterSpacing: '0.04em',
                      marginBottom: 6,
                    }}>
                      FULL NAME
                    </label>
                    <input
                      type="text"
                      value={name}
                      onChange={e => setName(e.target.value)}
                      placeholder="e.g. Harsh Narayan"
                      autoFocus
                      style={{
                        width: '100%',
                        padding: '10px 14px',
                        border: '1px solid #d1d5db',
                        borderRadius: 6,
                        fontSize: 14,
                        outline: 'none',
                        boxSizing: 'border-box',
                        background: '#ffffff',
                        color: '#111827',
                      }}
                      onFocus={e => e.currentTarget.style.borderColor = '#0d3824'}
                      onBlur={e => e.currentTarget.style.borderColor = '#d1d5db'}
                    />
                  </div>
                )}

                <div style={{ marginBottom: 14 }}>
                  <label style={{
                    fontSize: 11,
                    fontWeight: 700,
                    color: '#111827',
                    display: 'block',
                    textTransform: 'uppercase',
                    letterSpacing: '0.04em',
                    marginBottom: 6,
                  }}>
                    PASSWORD
                  </label>
                  <div style={{ display: 'flex', position: 'relative' }}>
                    <input
                      type={showPassword ? 'text' : 'password'}
                      value={password}
                      onChange={e => setPassword(e.target.value)}
                      placeholder="••••••••••••"
                      autoFocus={!isRegister}
                      style={{
                        flex: 1,
                        padding: '10px 14px',
                        border: '1px solid #d1d5db',
                        borderRight: 'none',
                        borderRadius: '6px 0 0 6px',
                        fontSize: 14,
                        outline: 'none',
                        boxSizing: 'border-box',
                        background: '#ffffff',
                        color: '#111827',
                      }}
                      onFocus={e => {
                        e.currentTarget.style.borderColor = '#0d3824'
                        if (e.currentTarget.nextElementSibling) {
                          e.currentTarget.nextElementSibling.style.borderColor = '#0d3824'
                        }
                      }}
                      onBlur={e => {
                        e.currentTarget.style.borderColor = '#d1d5db'
                        if (e.currentTarget.nextElementSibling) {
                          e.currentTarget.nextElementSibling.style.borderColor = '#d1d5db'
                        }
                      }}
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      style={{
                        padding: '0 14px',
                        background: '#f9fafb',
                        border: '1px solid #d1d5db',
                        borderRadius: '0 6px 6px 0',
                        cursor: 'pointer',
                        color: '#4b5563',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        transition: 'border-color 0.15s, background 0.15s',
                      }}
                      title={showPassword ? 'Hide password' : 'Show password'}
                    >
                      <Fa icon={showPassword ? 'fa-eye-slash' : 'fa-eye'} style={{ fontSize: 13 }} />
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Checkbox: Remember me */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, margin: '14px 0 20px 0' }}>
              <input
                type="checkbox"
                id="rememberMe"
                checked={rememberMe}
                onChange={e => setRememberMe(e.target.checked)}
                style={{
                  width: 15,
                  height: 15,
                  accentColor: '#0d3824',
                  cursor: 'pointer',
                  borderRadius: 3,
                }}
              />
              <label htmlFor="rememberMe" style={{ fontSize: 13, color: '#374151', cursor: 'pointer', userSelect: 'none' }}>
                Remember me for 30 days <span style={{ color: '#6b7280' }}>(not on a shared or public device)</span>
              </label>
            </div>

            {/* Error Message */}
            {error && (
              <div style={{
                fontSize: 12,
                color: '#dc2626',
                background: '#fef2f2',
                border: '1px solid #fecaca',
                borderRadius: 6,
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

            {/* Buttons Row (Porkbun style) */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
              {step === 1 ? (
                <>
                  <button
                    type="submit"
                    style={{
                      background: '#0d3824',
                      border: 'none',
                      borderRadius: 6,
                      color: '#ffffff',
                      padding: '10px 24px',
                      fontSize: 14,
                      fontWeight: 600,
                      cursor: 'pointer',
                      transition: 'all 0.15s',
                      fontFamily: 'inherit',
                      boxShadow: '0 1px 2px rgba(0,0,0,0.06)',
                    }}
                    onMouseEnter={e => e.currentTarget.style.background = '#15803d'}
                    onMouseLeave={e => e.currentTarget.style.background = '#0d3824'}
                  >
                    Continue
                  </button>
                  <span style={{ fontSize: 13, color: '#6b7280' }}>or</span>
                  <button
                    type="button"
                    onClick={() => toggleMode(!isRegister)}
                    style={{
                      background: '#ffffff',
                      border: '1px solid #d1d5db',
                      borderRadius: 6,
                      color: '#111827',
                      padding: '10px 18px',
                      fontSize: 14,
                      fontWeight: 500,
                      cursor: 'pointer',
                      transition: 'all 0.15s',
                      fontFamily: 'inherit',
                    }}
                    onMouseEnter={e => e.currentTarget.style.borderColor = '#9ca3af'}
                    onMouseLeave={e => e.currentTarget.style.borderColor = '#d1d5db'}
                  >
                    {isRegister ? 'Log in' : 'Create a New Account'}
                  </button>
                </>
              ) : (
                <>
                  {/* Step 2 button with XOXO & circular revolving animation! */}
                  <button
                    type="submit"
                    disabled={loading}
                    style={{
                      background: loading ? '#e0f2fe' : '#0d3824',
                      border: loading ? '1px solid #7dd3fc' : 'none',
                      borderRadius: 6,
                      color: loading ? '#0369a1' : '#ffffff',
                      padding: '10px 26px',
                      fontSize: 14,
                      fontWeight: 700,
                      cursor: loading ? 'not-allowed' : 'pointer',
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 8,
                      transition: 'all 0.18s ease',
                      fontFamily: 'inherit',
                      boxShadow: '0 1px 2px rgba(0,0,0,0.06)',
                    }}
                    onMouseEnter={e => {
                      if (!loading) e.currentTarget.style.background = '#15803d'
                    }}
                    onMouseLeave={e => {
                      if (!loading) e.currentTarget.style.background = '#0d3824'
                    }}
                  >
                    {loading ? (
                      <>
                        <span className="porkbun-spinner" style={{ borderTopColor: '#0369a1' }} />
                        <span>XOXO</span>
                      </>
                    ) : (
                      isRegister ? 'Create Account' : 'Log in'
                    )}
                  </button>
                  <span style={{ fontSize: 13, color: '#6b7280' }}>or</span>
                  <button
                    type="button"
                    onClick={() => toggleMode(!isRegister)}
                    style={{
                      background: '#ffffff',
                      border: '1px solid #d1d5db',
                      borderRadius: 6,
                      color: '#111827',
                      padding: '10px 18px',
                      fontSize: 14,
                      fontWeight: 500,
                      cursor: 'pointer',
                      transition: 'all 0.15s',
                      fontFamily: 'inherit',
                    }}
                    onMouseEnter={e => e.currentTarget.style.borderColor = '#9ca3af'}
                    onMouseLeave={e => e.currentTarget.style.borderColor = '#d1d5db'}
                  >
                    {isRegister ? 'Log in to an Existing Account' : 'Create a New Account'}
                  </button>
                </>
              )}
            </div>

            {/* Sub-links */}
            <div style={{ marginTop: 18 }}>
              <span
                onClick={() => alert('To reset or recover credentials, authenticate with your linked WhatsApp number.')}
                style={{
                  fontSize: 12,
                  color: '#4b5563',
                  textDecoration: 'underline',
                  cursor: 'pointer',
                }}
              >
                Forgotten password, 2FA, or security key.
              </span>
            </div>
          </form>

          {/* Legal / Policy Box (Porkbun style) */}
          <div style={{
            background: '#fafaf9',
            border: '1px solid #e5e7eb',
            borderRadius: 6,
            padding: '16px 18px',
            marginTop: 26,
            fontSize: 12,
            color: '#4b5563',
            lineHeight: 1.5,
          }}>
            <div style={{ fontWeight: 700, color: '#111827', marginBottom: 8 }}>
              By continuing you agree to the following:
            </div>
            <p style={{ margin: '0 0 8px 0' }}>
              I acknowledge that Reyna only captures, stages, and indexes files shared in authorized WhatsApp groups, in accordance with the Product Terms of Service and Privacy Policy.
            </p>
            <p style={{ margin: 0, color: '#6b7280' }}>
              The lawyers made us say all of that ☝️
            </p>
          </div>
        </div>

        {/* Anti-phishing advisory banner with left accent bar (Porkbun style) */}
        <div style={{
          borderLeft: '3px solid #10b981',
          paddingLeft: 14,
          marginTop: 24,
          fontSize: 12,
          color: '#374151',
          lineHeight: 1.5,
        }}>
          Make sure you're signing in on <strong>reyna.local</strong> or <strong>localhost</strong>. We only authenticate through your registered WhatsApp session and will never ask for third-party passwords.
        </div>
      </div>
    </div>
  )
}
