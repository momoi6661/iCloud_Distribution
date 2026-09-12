import { useEffect, useRef, useState, type ReactNode } from 'react'
import Icon from './Icon'

function useOverlayPresence(open: boolean) {
  const [mounted, setMounted] = useState(open)
  const [closing, setClosing] = useState(false)
  useEffect(() => {
    if (open) {
      setMounted(true)
      setClosing(false)
      return undefined
    }
    if (!mounted) return undefined
    setClosing(true)
    const timer = window.setTimeout(() => {
      setMounted(false)
      setClosing(false)
    }, 180)
    return () => window.clearTimeout(timer)
  }, [open, mounted])
  return { mounted, closing }
}

export function Dialog({ open, title, children, onClose, wide = false }: { open: boolean; title: string; children: ReactNode; onClose: () => void; wide?: boolean }) {
  const { mounted, closing } = useOverlayPresence(open)
  useEffect(() => {
    if (!open) return undefined
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])
  if (!mounted) return null
  return <div className={`overlay dialog-overlay ${closing ? 'closing' : ''}`} role="presentation" onPointerDown={(event) => { if (event.target === event.currentTarget) onClose() }}><section className={`dialog ${wide ? 'dialog-wide' : ''} ${closing ? 'closing' : ''}`} role="dialog" aria-modal="false" aria-labelledby="dialog-title"><div className="dialog-header"><h2 id="dialog-title">{title}</h2><button className="icon-button" aria-label="关闭对话框" onClick={onClose}><Icon name="close" /></button></div>{children}</section></div>
}

export function SidePanel({ open, title, children, onClose }: { open: boolean; title: string; children: ReactNode; onClose: () => void }) {
  const { mounted, closing } = useOverlayPresence(open)
  const panelRef = useRef<HTMLElement>(null)
  useEffect(() => {
    if (!open) return undefined
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])
  useEffect(() => {
    if (!open) return undefined
    const onPointerDown = (event: PointerEvent) => {
      if (!panelRef.current?.contains(event.target as Node)) onClose()
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [open, onClose])
  if (!mounted) return null
  return <div className={`overlay panel-overlay ${closing ? 'closing' : ''}`} role="presentation"><section ref={panelRef} className={`side-panel ${closing ? 'closing' : ''}`} role="dialog" aria-modal="false" aria-labelledby="panel-title"><div className="dialog-header"><h2 id="panel-title">{title}</h2><button className="icon-button" aria-label="关闭侧栏" onClick={onClose}><Icon name="close" /></button></div>{children}</section></div>
}
