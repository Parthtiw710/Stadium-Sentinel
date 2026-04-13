import React, { useState, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { QRCodeSVG } from 'qrcode.react'
import { Smartphone, ScanLine, CheckCircle2, AlertCircle, ArrowRight } from 'lucide-react'

export function AuthModal({ onComplete }) {
  const [phone, setPhone] = useState('')
  const [step, setStep] = useState('phone') // phone -> qr -> success
  const [qrCode, setQrCode] = useState(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    let evtSource
    if (step === 'qr' || step === 'phone') {
      evtSource = new EventSource('/api/events')
      evtSource.onmessage = (e) => {
        const data = JSON.parse(e.data)
        if (data.type === 'WA_QR') {
          setQrCode(data.message)
          setStep('qr')
        } else if (data.type === 'WA_READY') {
          setStep('success')
          setTimeout(onComplete, 1500)
        } else if (data.type === 'WA_ERROR') {
          setError(data.message)
          setStep('phone')
        }
      }
    }
    return () => {
      if (evtSource) evtSource.close()
    }
  }, [step, onComplete])

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!phone) return
    setError(null)
    setStep('qr') // Optimistic UI
    
    try {
      const res = await fetch('/api/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ phone })
      })
      if (!res.ok) throw new Error('Setup failed')
    } catch (err) {
      setError(err.message)
      setStep('phone')
    }
  }

  return (
    <div className="modal-overlay">
      <motion.div 
        className="modal-content glass-panel"
        initial={{ opacity: 0, scale: 0.9, y: 20 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ type: "spring", bounce: 0.4, duration: 0.6 }}
      >
        <AnimatePresence mode="wait">
          
          {step === 'phone' && (
            <motion.div 
              key="phone"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="modal-step"
            >
              <div className="icon-ring">
                <Smartphone size={32} className="text-blue" />
              </div>
              <h2>Connect WhatsApp</h2>
              <p>Register your personal number to receive critical self-healing alerts.</p>
              
              <form onSubmit={handleSubmit} className="auth-form">
                <div className="input-group">
                  <span className="prefix">+</span>
                  <input 
                    type="tel" 
                    placeholder="1234567890" 
                    value={phone}
                    onChange={e => setPhone(e.target.value.replace(/[^0-9]/g, ''))}
                    autoFocus
                  />
                </div>
                {error && <div className="error-msg"><AlertCircle size={14}/> {error}</div>}
                <button type="submit" disabled={!phone} className="btn-primary">
                  Continue <ArrowRight size={18} />
                </button>
              </form>
            </motion.div>
          )}

          {step === 'qr' && (
            <motion.div 
              key="qr"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="modal-step"
            >
              <div className="icon-ring pulse">
                <ScanLine size={32} className="text-green" />
              </div>
              <h2>Scan to Link Device</h2>
              <p>Open WhatsApp on your phone <strong>Settings &gt; Linked Devices</strong> and point your camera at the screen.</p>
              
              <div className="qr-container">
                {qrCode ? (
                  <QRCodeSVG value={qrCode} size={256} className="qr-svg" />
                ) : (
                  <div className="qr-placeholder">Generating secure QR code...</div>
                )}
              </div>
            </motion.div>
          )}

          {step === 'success' && (
            <motion.div 
              key="success"
              initial={{ opacity: 0, scale: 0.8 }}
              animate={{ opacity: 1, scale: 1 }}
              className="modal-step success"
            >
              <CheckCircle2 size={64} className="text-green fade-in-up" />
              <h2 className="mt-4 fade-in-up delay-1">Device Linked</h2>
              <p className="text-muted fade-in-up delay-2">Redirecting to Sentinel Dashboard...</p>
            </motion.div>
          )}

        </AnimatePresence>
      </motion.div>
    </div>
  )
}
