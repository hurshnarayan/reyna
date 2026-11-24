import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { notify } from '../components/Notifications'
import { Fa, icons } from '../components/icons'
import Modal from '../components/Modal'

export default function Groups() {
  const navigate = useNavigate()
  const [groupSettings, setGroupSettings] = useState([])
  const [loading, setLoading] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [activeTab, setActiveTab] = useState('all') // 'all' | 'active' | 'paused' | 'auto' | 'reaction'
  const [selectedGroupIds, setSelectedGroupIds] = useState(new Set())

  // Configure Modal State
  const [configGroup, setConfigGroup] = useState(null)
  const [configEnabled, setConfigEnabled] = useState(true)
  const [configMode, setConfigMode] = useState('auto')
  const [configEmoji, setConfigEmoji] = useState('📌')
  const [configHours, setConfigHours] = useState(24)
  const [saving, setSaving] = useState(false)

  const fetchGroups = async () => {
    try {
      const data = await api.allGroupSettings()
      setGroupSettings(data || [])
    } catch (err) {
      notify.error('Failed to load groups: ' + (err.message || err))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchGroups()
    const interval = setInterval(fetchGroups, 6000)
    return () => clearInterval(interval)
  }, [])

  const toggleGroupEnabled = async (groupId, enabled) => {
    setGroupSettings(prev => prev.map(g => g.group?.id === groupId ? { ...g, settings: { ...g.settings, enabled } } : g))
    await api.updateGroupSettings(groupId, { enabled })
    notify.success(enabled ? 'Group tracking enabled' : 'Group tracking paused')
  }

  const setTrackingMode = async (groupId, mode) => {
    setGroupSettings(prev => prev.map(g => g.group?.id === groupId ? { ...g, settings: { ...g.settings, tracking_mode: mode } } : g))
    await api.updateGroupSettings(groupId, { tracking_mode: mode })
    notify.success(mode === 'auto' ? 'Mode: Track all files' : 'Mode: Reactions only')
  }

  const openConfig = (gs) => {
    setConfigGroup(gs)
    setConfigEnabled(gs.settings?.enabled ?? true)
    setConfigMode(gs.settings?.tracking_mode || 'auto')
    setConfigEmoji(gs.settings?.reaction_emoji || '📌')
    setConfigHours(gs.settings?.auto_commit_hours || 24)
  }

  const saveConfig = async () => {
    if (!configGroup?.group?.id) return
    setSaving(true)
    try {
      const gid = configGroup.group.id
      await api.updateGroupSettings(gid, {
        enabled: configEnabled,
        tracking_mode: configMode,
        reaction_emoji: configEmoji,
        auto_commit_hours: configHours,
      })
      setGroupSettings(prev => prev.map(g => g.group?.id === gid ? {
        ...g,
        settings: {
          ...g.settings,
          enabled: configEnabled,
          tracking_mode: configMode,
          reaction_emoji: configEmoji,
          auto_commit_hours: configHours,
        }
      } : g))
      notify.success(`Updated bot configuration for ${(configGroup.group?.name || 'group').toLowerCase()}`)
      setConfigGroup(null)
    } catch (err) {
      notify.error('Failed to save settings: ' + (err.message || err))
    } finally {
      setSaving(false)
    }
  }

  const removeGroup = async (groupId) => {
    if (!window.confirm('Remove this group from tracking?')) return
    try {
      await api.updateGroupSettings(groupId, { enabled: false, hidden: true })
      setGroupSettings(prev => prev.filter(g => g.group?.id !== groupId))
      notify.success('Group removed. Send /reyna init in WhatsApp to re-add.')
    } catch (err) {
      notify.error('Failed to remove group: ' + (err.message || err))
    }
  }

  const toggleSelectGroup = (groupId) => {
    setSelectedGroupIds(prev => {
      const next = new Set(prev)
      if (next.has(groupId)) next.delete(groupId)
      else next.add(groupId)
      return next
    })
  }

  const toggleSelectAll = () => {
    if (selectedGroupIds.size === filteredGroups.length) {
      setSelectedGroupIds(new Set())
    } else {
      setSelectedGroupIds(new Set(filteredGroups.map(g => g.group?.id).filter(Boolean)))
    }
  }

  // Filter & Search
  const filteredGroups = groupSettings.filter(gs => {
    const name = (gs.group?.name || '').toLowerCase()
    const waId = (gs.group?.wa_id || '').toLowerCase()
    const q = searchQuery.toLowerCase().trim()
    const matchesSearch = !q || name.includes(q) || waId.includes(q)

    const isEnabled = gs.settings?.enabled ?? false
    const mode = gs.settings?.tracking_mode || 'auto'

    if (!matchesSearch) return false
    if (activeTab === 'active') return isEnabled
    if (activeTab === 'paused') return !isEnabled
    if (activeTab === 'auto') return mode === 'auto'
    if (activeTab === 'reaction') return mode === 'reaction'
    return true
  })

  return (
    <div style={{ padding: '40px 48px 60px', maxWidth: 1100 }} className="fade-in">
      {/* ── Breadcrumb ── */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#6b7280', marginBottom: 20 }}>
        <span
          style={{ cursor: 'pointer', transition: 'color 0.15s' }}
          onClick={() => navigate('/dashboard')}
          onMouseEnter={e => e.currentTarget.style.color = '#111827'}
          onMouseLeave={e => e.currentTarget.style.color = '#6b7280'}
        >
          reyna
        </span>
        <span style={{ color: '#d1d5db' }}>/</span>
        <span style={{ color: '#111827', fontWeight: 600 }}>all groups</span>
      </div>

      {/* ── Main All Groups Section ── */}
      <div style={{
        background: '#ffffff',
        border: '1px solid #e5e7eb',
        borderRadius: 18,
        padding: '22px 26px',
        boxShadow: '0 4px 20px -2px rgba(0, 0, 0, 0.04), 0 1px 3px rgba(0, 0, 0, 0.02)',
      }}>
        {/* Controls Bar */}
        <div style={{
          display: 'flex',
          flexWrap: 'wrap',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 14,
          marginBottom: 18,
          paddingBottom: 16,
          borderBottom: '1px solid #f3f4f6',
        }}>
          {/* Left Title & Filter Tabs */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <h2 style={{ fontSize: 18, fontWeight: 700, color: '#111827', margin: 0, textTransform: 'lowercase', letterSpacing: '-0.01em' }}>
                all groups
              </h2>
              <span style={{
                fontSize: 11,
                fontWeight: 600,
                color: '#15803d',
                background: '#dcfce7',
                padding: '2px 8px',
                borderRadius: 9999,
              }}>
                {filteredGroups.length}
              </span>
            </div>

            <div style={{
              display: 'flex',
              gap: 4,
              background: '#f9fafb',
              padding: 3,
              borderRadius: 10,
              border: '1px solid #f3f4f6',
            }}>
              {[
                { key: 'all', label: 'view all' },
                { key: 'active', label: 'active' },
                { key: 'paused', label: 'paused' },
                { key: 'auto', label: 'track all' },
                { key: 'reaction', label: 'reactions only' },
              ].map(tab => {
                const isSelected = activeTab === tab.key
                return (
                  <button
                    key={tab.key}
                    onClick={() => setActiveTab(tab.key)}
                    style={{
                      background: isSelected ? '#ffffff' : 'transparent',
                      color: isSelected ? '#111827' : '#6b7280',
                      border: isSelected ? '1px solid rgba(0,0,0,0.06)' : '1px solid transparent',
                      borderRadius: 7,
                      padding: '5px 12px',
                      fontSize: 12,
                      fontWeight: isSelected ? 600 : 500,
                      cursor: 'pointer',
                      fontFamily: 'inherit',
                      textTransform: 'lowercase',
                      boxShadow: isSelected ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
                      transition: 'all 0.15s ease',
                    }}
                  >
                    {tab.label}
                  </button>
                )
              })}
            </div>
          </div>

          {/* Right Search Input & Filters */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{
              position: 'relative',
              display: 'flex',
              alignItems: 'center',
            }}>
              <Fa icon="fa-search" style={{ position: 'absolute', left: 12, fontSize: 11, color: '#9ca3af' }} />
              <input
                type="text"
                placeholder="search groups..."
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                style={{
                  padding: '7px 12px 7px 32px',
                  fontSize: 12,
                  borderRadius: 9999,
                  border: '1px solid #e5e7eb',
                  background: '#f9fafb',
                  color: '#111827',
                  outline: 'none',
                  width: 190,
                  fontFamily: 'inherit',
                  textTransform: 'lowercase',
                  transition: 'border-color 0.15s, box-shadow 0.15s',
                }}
                onFocus={e => {
                  e.currentTarget.style.borderColor = '#111827'
                  e.currentTarget.style.background = '#ffffff'
                }}
                onBlur={e => {
                  e.currentTarget.style.borderColor = '#e5e7eb'
                  e.currentTarget.style.background = '#f9fafb'
                }}
              />
            </div>
            <button
              onClick={() => fetchGroups()}
              style={{
                background: '#ffffff',
                border: '1px solid #e5e7eb',
                borderRadius: 9999,
                padding: '7px 14px',
                fontSize: 12,
                fontWeight: 500,
                color: '#374151',
                cursor: 'pointer',
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                textTransform: 'lowercase',
                transition: 'all 0.15s',
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
              <Fa icon={icons.refresh} style={{ fontSize: 10 }} />
              refresh
            </button>
          </div>
        </div>

        {/* Groups Table */}
        {loading ? (
          <div style={{ padding: '48px 0', textAlign: 'center', color: '#9ca3af', fontSize: 14 }}>
            <Fa icon={icons.loading} spin style={{ fontSize: 16, marginRight: 8 }} />
            loading groups...
          </div>
        ) : filteredGroups.length === 0 ? (
          <div style={{ padding: '56px 16px', textAlign: 'center' }}>
            <div style={{
              width: 44,
              height: 44,
              borderRadius: '50%',
              background: '#f3f4f6',
              color: '#9ca3af',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 18,
              marginBottom: 12,
            }}>
              <Fa icon={icons.groups} />
            </div>
            <div style={{ fontSize: 15, fontWeight: 600, color: '#111827', marginBottom: 4, textTransform: 'lowercase' }}>
              no groups found
            </div>
            <p style={{ fontSize: 13, color: '#6b7280', margin: 0, textTransform: 'lowercase' }}>
              {searchQuery ? 'try adjusting your search or tab filters' : 'send /reyna init in a whatsapp group to start tracking'}
            </p>
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #f3f4f6' }}>
                  <th style={{ width: 40, padding: '10px 12px' }}>
                    <input
                      type="checkbox"
                      checked={selectedGroupIds.size > 0 && selectedGroupIds.size === filteredGroups.length}
                      onChange={toggleSelectAll}
                      style={{ cursor: 'pointer' }}
                    />
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    group name
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    capture mode
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    bot status
                  </th>
                  <th style={{ padding: '10px 14px', fontSize: 12, fontWeight: 600, color: '#6b7280', textTransform: 'lowercase' }}>
                    trigger emoji
                  </th>
                  <th style={{ width: 60, padding: '10px 12px', textAlign: 'right', fontSize: 12, fontWeight: 600, color: '#6b7280' }}>
                  </th>
                </tr>
              </thead>
              <tbody>
                {filteredGroups.map((gs) => {
                  const gid = gs.group?.id
                  const enabled = gs.settings?.enabled || false
                  const mode = gs.settings?.tracking_mode || 'auto'
                  const emoji = gs.settings?.reaction_emoji || '📌'
                  const isChecked = selectedGroupIds.has(gid)

                  return (
                    <tr
                      key={gid}
                      style={{
                        borderBottom: '1px solid #f9fafb',
                        background: isChecked ? '#f9fafb' : '#ffffff',
                        transition: 'background 0.1s',
                      }}
                      onMouseEnter={e => e.currentTarget.style.background = '#f9fafb'}
                      onMouseLeave={e => e.currentTarget.style.background = isChecked ? '#f9fafb' : '#ffffff'}
                    >
                      {/* Checkbox */}
                      <td style={{ padding: '14px 12px' }}>
                        <input
                          type="checkbox"
                          checked={isChecked}
                          onChange={() => toggleSelectGroup(gid)}
                          style={{ cursor: 'pointer' }}
                        />
                      </td>

                      {/* Group Name & WA ID */}
                      <td style={{ padding: '14px 14px' }}>
                        <div
                          onClick={() => openConfig(gs)}
                          style={{ display: 'flex', alignItems: 'center', gap: 12, cursor: 'pointer' }}
                        >
                          <div style={{
                            width: 36,
                            height: 36,
                            borderRadius: 10,
                            background: enabled ? '#f0fdf4' : '#f3f4f6',
                            border: `1px solid ${enabled ? '#dcfce7' : '#e5e7eb'}`,
                            color: enabled ? '#15803d' : '#6b7280',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            fontSize: 13,
                            flexShrink: 0,
                          }}>
                            <Fa icon={icons.groups} />
                          </div>
                          <div>
                            <div style={{
                              fontSize: 14,
                              fontWeight: 600,
                              color: '#111827',
                              textTransform: 'lowercase',
                              letterSpacing: '-0.01em',
                            }}>
                              {(gs.group?.name || 'whatsapp group').toLowerCase()}
                            </div>
                            <div style={{ fontSize: 12, color: '#9ca3af', fontFamily: 'monospace' }}>
                              {gs.group?.wa_id || 'ID unknown'}
                            </div>
                          </div>
                        </div>
                      </td>

                      {/* Mode Badge / Selector */}
                      <td style={{ padding: '14px 14px' }}>
                        <button
                          onClick={() => setTrackingMode(gid, mode === 'auto' ? 'reaction' : 'auto')}
                          title="Click to toggle mode"
                          style={{
                            padding: '5px 12px',
                            fontSize: 12,
                            fontWeight: 500,
                            cursor: 'pointer',
                            borderRadius: 9999,
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 6,
                            border: mode === 'reaction' ? '1px solid #0d3824' : '1px solid #bbf7d0',
                            background: mode === 'reaction' ? '#0d3824' : '#ffffff',
                            color: mode === 'reaction' ? '#ffffff' : '#064e3b',
                            textTransform: 'lowercase',
                            fontFamily: 'inherit',
                            transition: 'all 0.15s ease',
                          }}
                        >
                          <Fa
                            icon={mode === 'auto' ? icons.trackAll : icons.trackReact}
                            style={{ fontSize: 10, color: mode === 'reaction' ? '#ffffff' : '#064e3b' }}
                          />
                          {mode === 'auto' ? 'track all files' : 'reactions only'}
                        </button>
                      </td>

                      {/* Bot Status & Toggle */}
                      <td style={{ padding: '14px 14px' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                          <span style={{
                            fontSize: 11,
                            fontWeight: 600,
                            color: enabled ? '#15803d' : '#6b7280',
                            background: enabled ? '#dcfce7' : '#f3f4f6',
                            padding: '2px 8px',
                            borderRadius: 9999,
                            textTransform: 'lowercase',
                            display: 'inline-block',
                          }}>
                            {enabled ? 'active' : 'paused'}
                          </span>
                          <label style={{ position: 'relative', display: 'inline-block', width: 34, height: 20, cursor: 'pointer' }}>
                            <input
                              type="checkbox"
                              checked={enabled}
                              onChange={e => toggleGroupEnabled(gid, e.target.checked)}
                              style={{ opacity: 0, width: 0, height: 0, position: 'absolute' }}
                            />
                            <span style={{
                              position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, borderRadius: 9999,
                              background: enabled ? '#064e3b' : '#d1d5db',
                              transition: 'background 0.2s cubic-bezier(0.4, 0, 0.2, 1)',
                            }}>
                              <span style={{
                                position: 'absolute', width: 16, height: 16, borderRadius: '50%', background: '#fff', top: 2,
                                left: enabled ? 16 : 2, transition: 'left 0.2s cubic-bezier(0.4, 0, 0.2, 1)',
                                boxShadow: '0 1px 2px rgba(0,0,0,0.2)',
                              }} />
                            </span>
                          </label>
                        </div>
                      </td>

                      {/* Trigger Emoji */}
                      <td style={{ padding: '14px 14px' }}>
                        <span style={{
                          fontSize: 13,
                          padding: '3px 8px',
                          background: '#f9fafb',
                          border: '1px solid #e5e7eb',
                          borderRadius: 6,
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: 4,
                        }}>
                          {emoji}
                        </span>
                      </td>

                      {/* Direct Actions: configure bot & remove group */}
                      <td style={{ padding: '14px 12px', textAlign: 'right', whiteSpace: 'nowrap' }}>
                        <div style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                          <button
                            onClick={() => openConfig(gs)}
                            title="configure bot"
                            style={{
                              background: '#ffffff',
                              border: '1px solid #e5e7eb',
                              borderRadius: 9999,
                              padding: '4px 8px',
                              fontSize: 11,
                              color: '#6b7280',
                              cursor: 'pointer',
                              display: 'inline-flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              transition: 'all 0.15s',
                            }}
                            onMouseEnter={e => {
                              e.currentTarget.style.borderColor = '#111827'
                              e.currentTarget.style.color = '#111827'
                            }}
                            onMouseLeave={e => {
                              e.currentTarget.style.borderColor = '#e5e7eb'
                              e.currentTarget.style.color = '#6b7280'
                            }}
                          >
                            <Fa icon={icons.settings} style={{ fontSize: 10 }} />
                          </button>
                          <button
                            onClick={() => removeGroup(gid)}
                            title="remove group"
                            style={{
                              background: '#ffffff',
                              border: '1px solid #e5e7eb',
                              borderRadius: 9999,
                              padding: '4px 8px',
                              fontSize: 11,
                              color: '#6b7280',
                              cursor: 'pointer',
                              display: 'inline-flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              transition: 'all 0.15s',
                            }}
                            onMouseEnter={e => {
                              e.currentTarget.style.borderColor = '#dc2626'
                              e.currentTarget.style.color = '#dc2626'
                            }}
                            onMouseLeave={e => {
                              e.currentTarget.style.borderColor = '#e5e7eb'
                              e.currentTarget.style.color = '#6b7280'
                            }}
                          >
                            <Fa icon={icons.close} style={{ fontSize: 10 }} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* ── Configure Group Bot Modal (Interactive configuration view) ── */}
      {configGroup && (
        <Modal onClose={() => setConfigGroup(null)}>
          <div
            onClick={e => e.stopPropagation()}
            style={{
              background: '#ffffff',
              borderRadius: 18,
              width: '100%',
              maxWidth: 580,
              padding: '28px 32px',
              boxShadow: '0 20px 40px -15px rgba(0, 0, 0, 0.2)',
              position: 'relative',
              maxHeight: '90vh',
              overflowY: 'auto',
            }}
          >
            {/* Header */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 24 }}>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}>
                  <div style={{
                    width: 32,
                    height: 32,
                    borderRadius: 8,
                    background: '#f0fdf4',
                    color: '#15803d',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 13,
                  }}>
                    <Fa icon={icons.groups} />
                  </div>
                  <h2 style={{ fontSize: 19, fontWeight: 700, color: '#111827', margin: 0, textTransform: 'lowercase', letterSpacing: '-0.02em' }}>
                    configure bot — {(configGroup.group?.name || 'group').toLowerCase()}
                  </h2>
                </div>
                <p style={{ fontSize: 13, color: '#6b7280', margin: 0, textTransform: 'lowercase' }}>
                  customize how reyna captures, classifies, and syncs files in this whatsapp group.
                </p>
              </div>
              <button
                onClick={() => setConfigGroup(null)}
                style={{
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  color: '#9ca3af',
                  fontSize: 16,
                  padding: 4,
                }}
              >
                <Fa icon={icons.close} />
              </button>
            </div>

            {/* 1. Bot Enabled Switch */}
            <div style={{
              background: '#f9fafb',
              border: '1px solid #e5e7eb',
              borderRadius: 12,
              padding: '16px 18px',
              marginBottom: 20,
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
            }}>
              <div>
                <div style={{ fontSize: 14, fontWeight: 600, color: '#111827', textTransform: 'lowercase' }}>
                  bot active in this group
                </div>
                <div style={{ fontSize: 12, color: '#6b7280', marginTop: 2, textTransform: 'lowercase' }}>
                  {configEnabled
                    ? 'reyna is actively monitoring incoming files and responding to commands.'
                    : 'bot is paused. messages and files are ignored.'}
                </div>
              </div>
              <label style={{ position: 'relative', display: 'inline-block', width: 42, height: 24, cursor: 'pointer', flexShrink: 0 }}>
                <input
                  type="checkbox"
                  checked={configEnabled}
                  onChange={e => setConfigEnabled(e.target.checked)}
                  style={{ opacity: 0, width: 0, height: 0, position: 'absolute' }}
                />
                <span style={{
                  position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, borderRadius: 9999,
                  background: configEnabled ? '#064e3b' : '#d1d5db',
                  transition: 'background 0.2s',
                }}>
                  <span style={{
                    position: 'absolute', width: 20, height: 20, borderRadius: '50%', background: '#fff', top: 2,
                    left: configEnabled ? 20 : 2, transition: 'left 0.2s',
                    boxShadow: '0 1px 3px rgba(0,0,0,0.2)',
                  }} />
                </span>
              </label>
            </div>

            {/* 2. File Capture Strategy */}
            <div style={{ marginBottom: 20 }}>
              <div style={{ fontSize: 14, fontWeight: 600, color: '#111827', textTransform: 'lowercase', marginBottom: 6 }}>
                file capture strategy
              </div>
              <div style={{ fontSize: 12, color: '#6b7280', marginBottom: 12, textTransform: 'lowercase' }}>
                choose whether all shared media is staged automatically or only on reaction.
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                {/* Auto Mode Option Card */}
                <div
                  onClick={() => setConfigMode('auto')}
                  style={{
                    border: configMode === 'auto' ? '2px solid #0d3824' : '1px solid #e5e7eb',
                    background: configMode === 'auto' ? '#f0fdf4' : '#ffffff',
                    borderRadius: 12,
                    padding: '16px',
                    cursor: 'pointer',
                    transition: 'all 0.15s',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <Fa icon={icons.trackAll} style={{ color: configMode === 'auto' ? '#0d3824' : '#6b7280' }} />
                      <span style={{ fontSize: 13, fontWeight: 700, color: '#111827', textTransform: 'lowercase' }}>
                        track all files
                      </span>
                    </div>
                    {configMode === 'auto' && (
                      <span style={{ width: 8, height: 8, borderRadius: '50%', background: '#0d3824' }} />
                    )}
                  </div>
                  <p style={{ fontSize: 12, color: '#4b5563', margin: 0, lineHeight: 1.4, textTransform: 'lowercase' }}>
                    every PDF, doc, and image shared in this group is staged automatically.
                  </p>
                </div>

                {/* Reaction Mode Option Card */}
                <div
                  onClick={() => setConfigMode('reaction')}
                  style={{
                    border: configMode === 'reaction' ? '2px solid #0d3824' : '1px solid #e5e7eb',
                    background: configMode === 'reaction' ? '#f0fdf4' : '#ffffff',
                    borderRadius: 12,
                    padding: '16px',
                    cursor: 'pointer',
                    transition: 'all 0.15s',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <Fa icon={icons.trackReact} style={{ color: configMode === 'reaction' ? '#0d3824' : '#6b7280' }} />
                      <span style={{ fontSize: 13, fontWeight: 700, color: '#111827', textTransform: 'lowercase' }}>
                        reactions only
                      </span>
                    </div>
                    {configMode === 'reaction' && (
                      <span style={{ width: 8, height: 8, borderRadius: '50%', background: '#0d3824' }} />
                    )}
                  </div>
                  <p style={{ fontSize: 12, color: '#4b5563', margin: 0, lineHeight: 1.4, textTransform: 'lowercase' }}>
                    only files that get a reaction emoji are staged. everything else is ignored.
                  </p>
                </div>
              </div>
            </div>

            {/* 3. Reaction Trigger Emoji (if in reactions mode) */}
            <div style={{ marginBottom: 20 }}>
              <div style={{ fontSize: 14, fontWeight: 600, color: '#111827', textTransform: 'lowercase', marginBottom: 6 }}>
                reaction trigger emoji
              </div>
              <div style={{ fontSize: 12, color: '#6b7280', marginBottom: 10, textTransform: 'lowercase' }}>
                reacting to any document with this emoji queues it in your staging area.
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                {['📌', '⭐', '💾', '📥', '📎'].map(em => (
                  <button
                    key={em}
                    onClick={() => setConfigEmoji(em)}
                    style={{
                      width: 40,
                      height: 40,
                      borderRadius: 10,
                      border: configEmoji === em ? '2px solid #0d3824' : '1px solid #e5e7eb',
                      background: configEmoji === em ? '#f0fdf4' : '#ffffff',
                      fontSize: 18,
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      transition: 'all 0.15s',
                    }}
                  >
                    {em}
                  </button>
                ))}
                <input
                  type="text"
                  value={configEmoji}
                  onChange={e => setConfigEmoji(e.target.value.slice(0, 4))}
                  placeholder="custom"
                  style={{
                    width: 70,
                    height: 40,
                    borderRadius: 10,
                    border: '1px solid #e5e7eb',
                    textAlign: 'center',
                    fontSize: 16,
                    outline: 'none',
                  }}
                />
              </div>
            </div>

            {/* 4. Auto-Commit Interval */}
            <div style={{ marginBottom: 24 }}>
              <div style={{ fontSize: 14, fontWeight: 600, color: '#111827', textTransform: 'lowercase', marginBottom: 6 }}>
                auto-commit to google drive
              </div>
              <div style={{ fontSize: 12, color: '#6b7280', marginBottom: 10, textTransform: 'lowercase' }}>
                files in staging are automatically synced and organized in drive after this window.
              </div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                {[
                  { hours: 6, label: '6 hours' },
                  { hours: 12, label: '12 hours' },
                  { hours: 24, label: '24 hours' },
                  { hours: 48, label: '48 hours' },
                ].map(opt => (
                  <button
                    key={opt.hours}
                    onClick={() => setConfigHours(opt.hours)}
                    style={{
                      padding: '7px 16px',
                      borderRadius: 9999,
                      border: configHours === opt.hours ? '1px solid #0d3824' : '1px solid #e5e7eb',
                      background: configHours === opt.hours ? '#0d3824' : '#ffffff',
                      color: configHours === opt.hours ? '#ffffff' : '#374151',
                      fontSize: 12,
                      fontWeight: 500,
                      cursor: 'pointer',
                      fontFamily: 'inherit',
                      transition: 'all 0.15s',
                    }}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            </div>

            {/* 5. WhatsApp In-Chat Commands Cheatsheet */}
            <div style={{
              background: '#f9fafb',
              border: '1px solid #f3f4f6',
              borderRadius: 12,
              padding: '14px 16px',
              marginBottom: 24,
            }}>
              <div style={{ fontSize: 12, fontWeight: 600, color: '#374151', textTransform: 'lowercase', marginBottom: 6 }}>
                whatsapp in-chat bot commands:
              </div>
              <div style={{ fontSize: 12, color: '#6b7280', lineHeight: 1.6, textTransform: 'lowercase' }}>
                <code>/reyna commit</code> — pushes all staged files immediately to Drive<br />
                <code>/reyna status</code> — checks bot tracking mode and staged files count<br />
                <code>/reyna mode auto</code> or <code>/reyna mode reaction</code> — switch mode directly in WhatsApp
              </div>
            </div>

            {/* Action Buttons */}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
              <button
                onClick={() => setConfigGroup(null)}
                style={{
                  background: '#ffffff',
                  border: '1px solid #e5e7eb',
                  borderRadius: 10,
                  padding: '9px 18px',
                  fontSize: 13,
                  fontWeight: 600,
                  color: '#374151',
                  cursor: 'pointer',
                  textTransform: 'lowercase',
                }}
              >
                cancel
              </button>
              <button
                onClick={saveConfig}
                disabled={saving}
                style={{
                  background: '#0d3824',
                  border: '1px solid #0d3824',
                  borderRadius: 10,
                  padding: '9px 22px',
                  fontSize: 13,
                  fontWeight: 600,
                  color: '#ffffff',
                  cursor: 'pointer',
                  opacity: saving ? 0.6 : 1,
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 8,
                  textTransform: 'lowercase',
                }}
              >
                {saving && <Fa icon={icons.loading} spin style={{ fontSize: 12 }} />}
                save configuration
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}
