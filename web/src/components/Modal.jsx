import { createPortal } from 'react-dom'

export default function Modal({ children, onClose, zIndex = 9999 }) {
  return createPortal(
    <div onClick={onClose} style={{
      position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh',
      background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center',
      justifyContent: 'center', zIndex, padding: 24,
    }}>
      {children}
    </div>,
    document.body
  )
}

export function FullScreenModal({ children, onClose, zIndex = 9999 }) {
  return createPortal(
    <div style={{
      position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh',
      background: 'rgba(0,0,0,0.85)', display: 'flex', flexDirection: 'column',
      zIndex,
    }}>
      {children}
    </div>,
    document.body
  )
}
