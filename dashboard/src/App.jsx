import { useState, useEffect, useRef, useCallback } from 'react'
import { motion } from 'framer-motion'
import { Activity, ShieldAlert, Cpu, Database, Wifi } from 'lucide-react'
import { AuthModal } from './AuthModal'
import './App.css'

const API = '/api'

const SERVICE_ICONS = {
  'Ticketing API': <Database size={20} />,
  'VIP Wi-Fi': <Wifi size={20} />,
  'Food Court POS': <Cpu size={20} />
}

export default function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [loadingConfig, setLoadingConfig] = useState(true)

  const [services, setServices] = useState([])
  const [events, setEvents] = useState([])
  const [adminPhone, setAdminPhone] = useState('')
  const logsEndRef = useRef(null)

  // Fetch initial config
  useEffect(() => {
    fetch(`${API}/health`)
      .then(r => r.json())
      .then(data => {
        setAdminPhone(data.admin_phone)
        if (data.admin_phone && data.wa_logged_in) {
          setIsAuthenticated(true)
        }
        setLoadingConfig(false)
      })
      .catch(err => {
        console.error("Failed to fetch health check", err)
        setLoadingConfig(false)
      })
  }, [])

  // Poll services once authenticated
  const fetchServices = useCallback(() => {
    if (!isAuthenticated) return
    fetch(`${API}/services`)
      .then(res => res.json())
      .then(data => setServices(data || []))
      .catch(err => console.error("Error fetching services:", err))
  }, [isAuthenticated])

  useEffect(() => {
    fetchServices()
    const int = setInterval(fetchServices, 2000)
    return () => clearInterval(int)
  }, [fetchServices])

  // Setup SSE for real-time events exclusively when authenticated
  useEffect(() => {
    if (!isAuthenticated) return

    const evtSource = new EventSource(`${API}/events`)
    evtSource.onmessage = (e) => {
      const data = JSON.parse(e.data)
      // Skip WA auth events since we are already authenticated
      if (data.type.startsWith('WA_')) return 
      
      setEvents(prev => [...prev.slice(-49), data])
      fetchServices() // Trigger immediate refetch of service status on event
    }
    return () => evtSource.close()
  }, [isAuthenticated, fetchServices])

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [events])

  const injectFailure = async (name) => {
    await fetch(`${API}/services/${encodeURIComponent(name)}/fail`, { method: 'POST' })
    fetchServices()
  }

  const restoreService = async (name) => {
    await fetch(`${API}/services/${encodeURIComponent(name)}/restore`, { method: 'POST' })
    fetchServices()
  }

  // Derived metrics
  const total = services.length
  const up = services.filter(s => s.status === 'UP').length
  const down = services.filter(s => s.status === 'DOWN').length
  const healthPercent = total === 0 ? 0 : Math.round((up / total) * 100)

  if (loadingConfig) {
    return <div className="loading-screen">Starting Stadium Sentinel Orchestrator...</div>
  }

  if (!isAuthenticated) {
    return (
      <div className="app-container">
        <AuthModal onComplete={() => setIsAuthenticated(true)} />
      </div>
    )
  }

  return (
    <div className="app-container">
      <header className="header glass-panel">
        <div className="logo-container">
          <Activity size={32} className="text-blue bounce" />
          <div>
            <h1>Stadium Sentinel</h1>
            <p>Self-Healing Infrastructure Agent</p>
          </div>
        </div>
        <div className="status-badge">
          <div className="dot green-glow"></div>
          Agent Online
        </div>
      </header>

      <main className="dashboard">
        <div className="metrics-bar glass-panel">
          <div className="metric">
            <span className="metric-value text-green">{up}</span>
            <span className="metric-label">ONLINE</span>
          </div>
          <div className="metric">
            <span className={`metric-value ${down > 0 ? 'text-red pulse-red' : 'text-muted'}`}>{down}</span>
            <span className="metric-label">DOWN</span>
          </div>
          <div className="health-section">
            <div className="health-header">
              <span className="metric-value text-blue">{healthPercent}%</span>
              <span className="metric-label">HEALTH</span>
            </div>
            <div className="progress-bar">
              <motion.div 
                className={`progress-fill ${healthPercent < 100 ? 'bg-orange' : 'bg-green'}`} 
                initial={{ width: 0 }}
                animate={{ width: `${healthPercent}%` }}
                transition={{ duration: 0.5, type: 'spring' }}
              ></motion.div>
            </div>
          </div>
        </div>

        <div className="main-content">
          <section className="services-section">
            <div className="section-header">
              <ShieldAlert size={20} className="text-muted" />
              <h2>Infrastructure Matrix</h2>
            </div>
            
            <div className={`services-grid ${services.length === 0 ? 'empty' : ''}`}>
              {services.length === 0 ? (
                <div className="empty-state">
                  <Activity size={48} className="text-muted float" />
                  <p>Awaiting service telemetry...</p>
                </div>
              ) : (
                services.map(svc => (
                  <motion.div 
                    key={svc.name} 
                    className={`service-card glass-panel ${svc.status === 'DOWN' ? 'border-red glow-red' : 'border-green glow-green'}`}
                    layout
                    initial={{ opacity: 0, scale: 0.95 }}
                    animate={{ opacity: 1, scale: 1 }}
                  >
                    <div className="card-header">
                      <div className="service-title">
                        {SERVICE_ICONS[svc.name] || <Cpu size={20} />}
                        <h3>{svc.name}</h3>
                      </div>
                      <div className={`status-pill ${svc.status === 'DOWN' ? 'bg-red' : 'bg-green'}`}>
                        {svc.status}
                      </div>
                    </div>
                    
                    <div className="card-actions">
                      <button 
                        onClick={() => injectFailure(svc.name)}
                        className="btn-danger"
                        disabled={svc.status === 'DOWN'}
                      >
                        Inject Failure
                      </button>
                      <button 
                        onClick={() => restoreService(svc.name)}
                        className="btn-success"
                        disabled={svc.status === 'UP'}
                      >
                         Restore
                      </button>
                    </div>
                  </motion.div>
                ))
              )}
            </div>
          </section>

          <section className="logs-section glass-panel">
            <div className="section-header border-bottom">
              <div className="brain-icon pulse">🧠</div>
              <h2>Agent Event Stream</h2>
              <div className="log-count">{events.length} events</div>
            </div>
            <div className="logs-container">
              {events.length === 0 ? (
                <div className="empty-state">Waiting for agent actions...</div>
              ) : (
                events.map((evt, idx) => (
                  <motion.div 
                    key={idx} 
                    className={`log-entry ${evt.type.toLowerCase()}`}
                    initial={{ opacity: 0, x: -20 }}
                    animate={{ opacity: 1, x: 0 }}
                  >
                    <div className="log-time">{new Date(evt.time).toLocaleTimeString()}</div>
                    <div className="log-badge">{evt.type}</div>
                    <div className="log-msg"><strong>{evt.service}</strong>: {evt.message}</div>
                  </motion.div>
                ))
              )}
              <div ref={logsEndRef} />
            </div>
          </section>
        </div>
      </main>
    </div>
  )
}
