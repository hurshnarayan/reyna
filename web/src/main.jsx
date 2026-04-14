import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import './index.css'
import Landing from './pages/Landing'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Files from './pages/Files'
import Search from './pages/Search'
import Memory from './pages/Memory'
// import BotDemo from './pages/BotDemo'  // Removed — bot demo disabled
import NotFound from './pages/NotFound'
import Layout from './components/Layout'
import NotificationContainer from './components/Notifications'
import { JobsProvider } from './components/BackgroundJobs'

/* ══ Global Lusion button hover — radial gradient, no fill div ══ */
;(function() {
  const skip = (btn) => {
    if (btn.disabled) return true
    if (btn.closest('aside')) return true
    if (btn.closest('label')) return true
    if (btn.classList.contains('reyna-pill')) return true
    return false
  }

  const fillColor = (btn) => {
    const s = (btn.getAttribute('style') || '') + ' ' + (btn.className || '')
    if (s.includes('error') || s.includes('#e5c4c4') || s.includes('#dc2626') || s.includes('danger')) return '#dc2626'
    if (s.includes('reyna-accent') || s.includes('#25D366') || s.includes('primary')) return '#1a8a45'
    if (btn.closest('.reyna-settings-panel')) return 'rgba(255,255,255,0.15)'
    return '#1a1a1a'
  }

  let activeBtn = null

  document.addEventListener('mouseover', (e) => {
    const btn = e.target.closest?.('button')
    if (!btn || skip(btn)) return
    if (btn === activeBtn) return
    if (activeBtn && activeBtn !== btn) cleanupBtn(activeBtn)
    activeBtn = btn

    const rect = btn.getBoundingClientRect()
    const rx = ((e.clientX - rect.left) / rect.width * 100)
    const ry = ((e.clientY - rect.top) / rect.height * 100)

    // set gradient origin and color as CSS custom properties
    btn.style.setProperty('--rx', rx + '%')
    btn.style.setProperty('--ry', ry + '%')
    btn.style.setProperty('--fill', fillColor(btn))

    // add class — CSS handles color: #fff !important and background-image
    btn.classList.add('reyna-hover-active')

    // trigger expansion after a frame (so background-size transitions from 0% to 600%)
    requestAnimationFrame(() => btn.classList.add('reyna-hover-expanding'))
  }, true)

  document.addEventListener('mousemove', (e) => {
    if (!activeBtn) return
    const rect = activeBtn.getBoundingClientRect()
    const rx = ((e.clientX - rect.left) / rect.width * 100)
    const ry = ((e.clientY - rect.top) / rect.height * 100)
    activeBtn.style.setProperty('--rx', rx + '%')
    activeBtn.style.setProperty('--ry', ry + '%')
  }, true)

  document.addEventListener('mouseout', (e) => {
    const btn = e.target.closest?.('button')
    if (!btn || btn !== activeBtn) return
    if (e.relatedTarget && btn.contains(e.relatedTarget)) return
    cleanupBtn(btn)
    activeBtn = null
  }, true)

  function cleanupBtn(btn) {
    btn.classList.remove('reyna-hover-active', 'reyna-hover-expanding')
    btn.style.removeProperty('--rx')
    btn.style.removeProperty('--ry')
    btn.style.removeProperty('--fill')
    btn.style.removeProperty('--btn-text')
  }
})()

/* ══ Blob parallax — green & blue blobs follow cursor ══ */
;(function() {
  let tx = 0, ty = 0, cx = 0, cy = 0

  document.addEventListener('mousemove', (e) => {
    tx = (e.clientX / window.innerWidth - 0.5) * 2
    ty = (e.clientY / window.innerHeight - 0.5) * 2
  })

  function tick() {
    cx += (tx - cx) * 0.03
    cy += (ty - cy) * 0.03
    const b1 = document.querySelector('.reyna-blob-1')
    const b2 = document.querySelector('.reyna-blob-2')
    if (b1) b1.style.transform = `translate(${cx * 45}px, ${cy * 35}px)`
    if (b2) b2.style.transform = `translate(${cx * -30}px, ${cy * -25}px)`
    requestAnimationFrame(tick)
  }
  requestAnimationFrame(tick)
})()

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <BrowserRouter>
      <NotificationContainer />
      <JobsProvider>
        <Routes>
          <Route path="/" element={<Landing />} />
          <Route path="/login" element={<Login />} />
          <Route element={<Layout />}>
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/files" element={<Files />} />
            <Route path="/search" element={<Search />} />
            <Route path="/recall" element={<Search />} />
            <Route path="/memory" element={<Memory />} />
            {/* <Route path="/bot" element={<BotDemo />} /> */}
          </Route>
          <Route path="*" element={<NotFound />} />
        </Routes>
      </JobsProvider>
    </BrowserRouter>
  </React.StrictMode>
)
