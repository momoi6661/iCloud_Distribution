import { useEffect, useRef, useState, type ReactNode } from 'react'
import Icon from './Icon'

const focusableSelector = [
  'a[href]',
  'area[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function getFocusableElements(surface: HTMLElement) {
  return Array.from(surface.querySelectorAll<HTMLElement>(focusableSelector)).filter((element) => !element.hasAttribute('hidden') && element.getAttribute('aria-hidden') !== 'true')
}

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

function useOverlayFocus(open: boolean, surfaceRef: { current: HTMLElement | null }, onClose: () => void) {
  const openerRef = useRef<HTMLElement | null>(null)
  const activeRef = useRef(false)
  const restoreTimerRef = useRef<number | null>(null)

  useEffect(() => {
    if (!open) return undefined
    if (restoreTimerRef.current !== null) {
      window.clearTimeout(restoreTimerRef.current)
      restoreTimerRef.current = null
    }
    if (!activeRef.current) {
      openerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
      activeRef.current = true
    }
    const frame = window.requestAnimationFrame(() => {
      const surface = surfaceRef.current
      if (!surface) return
      const initialFocus = getFocusableElements(surface)[0] || surface
      initialFocus.focus({ preventScroll: true })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [open, surfaceRef])

  useEffect(() => {
    if (open || !activeRef.current) return undefined
    activeRef.current = false
    restoreTimerRef.current = window.setTimeout(() => {
      const opener = openerRef.current
      if (opener?.isConnected) opener.focus({ preventScroll: true })
      openerRef.current = null
      restoreTimerRef.current = null
    }, 180)
    return () => {
      if (restoreTimerRef.current !== null) {
        window.clearTimeout(restoreTimerRef.current)
        restoreTimerRef.current = null
      }
    }
  }, [open])

  useEffect(() => {
    if (!open) return undefined
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
        return
      }
      if (event.key !== 'Tab') return
      const surface = surfaceRef.current
      if (!surface) return
      const focusable = getFocusableElements(surface)
      if (!focusable.length) {
        event.preventDefault()
        surface.focus({ preventScroll: true })
        return
      }
      const currentIndex = focusable.indexOf(document.activeElement as HTMLElement)
      if (event.shiftKey && (currentIndex <= 0 || currentIndex === -1)) {
        event.preventDefault()
        focusable[focusable.length - 1].focus({ preventScroll: true })
      } else if (!event.shiftKey && (currentIndex === -1 || currentIndex === focusable.length - 1)) {
        event.preventDefault()
        focusable[0].focus({ preventScroll: true })
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose, surfaceRef])
}

export function Dialog({ open, title, children, onClose, wide = false }: { open: boolean; title: string; children: ReactNode; onClose: () => void; wide?: boolean }) {
  const { mounted, closing } = useOverlayPresence(open)
  const surfaceRef = useRef<HTMLElement>(null)
  useOverlayFocus(open, surfaceRef, onClose)
  if (!mounted) return null
  return <div className={`overlay dialog-overlay ${closing ? 'closing' : ''}`} role="presentation" onPointerDown={(event) => { if (event.target === event.currentTarget) onClose() }}><section ref={surfaceRef} className={`dialog ${wide ? 'dialog-wide' : ''} ${closing ? 'closing' : ''}`} role="dialog" aria-modal="false" aria-labelledby="dialog-title" tabIndex={-1}><div className="dialog-header"><h2 id="dialog-title">{title}</h2><button className="icon-button" aria-label="关闭对话框" onClick={onClose}><Icon name="close" /></button></div>{children}</section></div>
}

export function SidePanel({ open, title, children, onClose }: { open: boolean; title: string; children: ReactNode; onClose: () => void }) {
  const { mounted, closing } = useOverlayPresence(open)
  const panelRef = useRef<HTMLElement>(null)
  useOverlayFocus(open, panelRef, onClose)
  useEffect(() => {
    if (!open) return undefined
    const onPointerDown = (event: PointerEvent) => {
      if (!panelRef.current?.contains(event.target as Node)) onClose()
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [open, onClose])
  if (!mounted) return null
  return <div className={`overlay panel-overlay ${closing ? 'closing' : ''}`} role="presentation"><section ref={panelRef} className={`side-panel ${closing ? 'closing' : ''}`} role="dialog" aria-modal="false" aria-labelledby="panel-title" tabIndex={-1}><div className="dialog-header"><h2 id="panel-title">{title}</h2><button className="icon-button" aria-label="关闭侧栏" onClick={onClose}><Icon name="close" /></button></div>{children}</section></div>
}
