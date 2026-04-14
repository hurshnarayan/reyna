/**
 * BackgroundJobs — global state + UI for long-running server-side tasks
 * (Drive sync, push staged to Drive). The dashboard polls /api/jobs/status
 * every 2 seconds while a job is active; this way the sticky progress
 * pill stays visible across route changes, the user can switch sidebar
 * tabs without cancelling anything, and "already syncing" is detectable
 * from any page without re-plumbing per-page state.
 */
import { createContext, useContext, useEffect, useState, useCallback, useRef } from 'react'
import { api, isLoggedIn } from '../lib/api'
import { Fa } from './icons'

const JobsContext = createContext({ jobs: {}, refresh: () => {} })

// Poll cadence — tight during active jobs (2s) so the pill feels live;
// longer during idle (15s) so we don't hammer the backend for nothing.
const ACTIVE_POLL_MS = 2000
const IDLE_POLL_MS = 15000

export function JobsProvider({ children }) {
  const [jobs, setJobs] = useState({}) // { ingest_drive?: Job, push_staged?: Job }
  const timerRef = useRef(null)
  const mountedRef = useRef(true)

  const fetchStatus = useCallback(async () => {
    if (!isLoggedIn()) return
    try {
      const res = await api.jobsStatus()
      if (!mountedRef.current) return
      setJobs(res || {})
    } catch {}
  }, [])

  const scheduleNext = useCallback((anyActive) => {
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(fetchStatus, anyActive ? ACTIVE_POLL_MS : IDLE_POLL_MS)
  }, [fetchStatus])

  useEffect(() => {
    mountedRef.current = true
    fetchStatus()
    return () => {
      mountedRef.current = false
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [fetchStatus])

  // Re-arm the timer every time `jobs` updates — active (running) state
  // triggers fast polling, terminal state throttles back.
  useEffect(() => {
    const running = Object.values(jobs).some(j => j?.state === 'running')
    scheduleNext(running)
  }, [jobs, scheduleNext])

  return (
    <JobsContext.Provider value={{ jobs, refresh: fetchStatus }}>
      {children}
      <JobsPill jobs={jobs} />
    </JobsContext.Provider>
  )
}

export function useJobs() {
  return useContext(JobsContext)
}

// JobsPill — sticky bottom-right pill that surfaces running / recently-
// finished jobs. Shows progress count + a short status line. Only rendered
// when there's something worth showing.
function JobsPill({ jobs }) {
  const active = Object.values(jobs).filter(j => j?.state === 'running')
  const justFinished = Object.values(jobs).filter(j =>
    j?.state === 'done' && j.finished_at && (Date.now() - new Date(j.finished_at).getTime()) < 8000
  )
  const toShow = [...active, ...justFinished]
  if (toShow.length === 0) return null

  return (
    <div style={{
      position: 'fixed', right: 16, bottom: 96, zIndex: 400,
      display: 'flex', flexDirection: 'column', gap: 8,
      pointerEvents: 'none',
    }}>
      {toShow.map((j) => (
        <div key={j.id} style={{
          background: '#fff',
          border: '1.5px solid ' + (j.state === 'running' ? 'var(--reyna-accent)' : '#ddd'),
          borderRadius: 12,
          padding: '10px 14px',
          minWidth: 260, maxWidth: 340,
          boxShadow: '0 4px 18px rgba(0,0,0,0.10)',
          pointerEvents: 'auto',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <Fa
              icon={jobIcon(j.kind, j.state)}
              spin={j.state === 'running'}
              style={{ fontSize: 11, color: 'var(--reyna-accent)' }}
            />
            <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-color)' }}>
              {jobLabel(j.kind)}
            </span>
            <span style={{ marginLeft: 'auto', fontSize: 11, color: 'var(--sub-color)' }}>
              {j.state === 'running'
                ? `${j.done || 0}${j.total ? '/' + j.total : ''}`
                : j.state}
            </span>
          </div>
          {j.total > 0 && (
            <div style={{ height: 4, background: '#eee', borderRadius: 2, overflow: 'hidden', marginBottom: 6 }}>
              <div style={{
                height: '100%',
                width: Math.min(100, Math.round(((j.done || 0) / j.total) * 100)) + '%',
                background: j.state === 'running' ? 'var(--reyna-accent)' : '#999',
                transition: 'width 400ms ease',
              }} />
            </div>
          )}
          <div style={{ fontSize: 11, color: '#666', lineHeight: 1.45, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {j.message || (j.state === 'running' ? 'working...' : '')}
          </div>
        </div>
      ))}
    </div>
  )
}

function jobLabel(kind) {
  if (kind === 'ingest_drive') return 'syncing from drive'
  if (kind === 'push_staged') return 'pushing to drive'
  return kind
}
function jobIcon(kind, state) {
  if (state === 'running') return 'fa-rotate'
  if (state === 'done') return 'fa-check'
  if (state === 'failed') return 'fa-triangle-exclamation'
  return 'fa-rotate'
}
