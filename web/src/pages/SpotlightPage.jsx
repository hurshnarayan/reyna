import { useNavigate } from 'react-router-dom'
import Spotlight from '../components/Spotlight'

export default function SpotlightPage() {
  const navigate = useNavigate()
  return (
    <div style={{ minHeight: '100vh', background: '#0f172a' }}>
      <Spotlight isOpen={true} onClose={() => navigate('/dashboard')} />
    </div>
  )
}
