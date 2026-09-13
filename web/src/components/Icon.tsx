import type { SVGProps } from 'react'

const paths: Record<string, string[]> = {
  grid: ['M4 4h6v6H4z', 'M14 4h6v6h-6z', 'M4 14h6v6H4z', 'M14 14h6v6h-6z'],
  archive: ['M4 7h16', 'M6 7v12h12V7', 'M8 4h8l2 3H6l2-3z', 'M9 11h6'],
  plus: ['M12 5v14', 'M5 12h14'],
  settings: ['M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7Z', 'm19.4 15 .1.2-1.5 2.6-.3-.1a2 2 0 0 0-2.2.2l-.2.2a2 2 0 0 0-.7 2v.3h-3l-.1-.3a2 2 0 0 0-1.3-1.5l-.3-.1a2 2 0 0 0-2.1-.2l-.3.1-1.5-2.6.2-.2a2 2 0 0 0 .1-2.2l-.1-.3a2 2 0 0 0-1.7-1.2H4v-3h.3a2 2 0 0 0 1.7-1.2l.1-.3a2 2 0 0 0-.1-2.2l-.2-.2 1.5-2.6.3.1a2 2 0 0 0 2.1-.2l.3-.1A2 2 0 0 0 11.6 2l.1-.3h3V2a2 2 0 0 0 .7 2l.2.2a2 2 0 0 0 2.2.2l.3-.1 1.5 2.6-.1.2a2 2 0 0 0-.1 2.2l.1.3a2 2 0 0 0 1.7 1.2h.3v3h-.3a2 2 0 0 0-1.7 1.2Z'],
  logout: ['M10 17l5-5-5-5', 'M15 12H4', 'M20 4v16'],
  arrow: ['M5 12h13', 'm13 6 6 6-6 6'],
  search: ['m20 20-4.5-4.5', 'M10.5 17a6.5 6.5 0 1 1 0-13 6.5 6.5 0 0 1 0 13Z'],
  check: ['m5 12 4 4L19 6'],
  alert: ['M12 9v4', 'M12 17h.01', 'M10.3 3.8 2.6 17a2 2 0 0 0 1.7 3h15.4a2 2 0 0 0 1.7-3L13.7 3.8a2 2 0 0 0-3.4 0Z'],
  trash: ['M4 7h16', 'M10 11v6', 'M14 11v6', 'M6 7l1 13h10l1-13', 'M9 7V4h6v3'],
  restore: ['M4 12a8 8 0 1 0 2.3-5.7L4 8.6', 'M4 4v4.6h4.6'],
  menu: ['M4 7h16', 'M4 12h16', 'M4 17h16'],
  close: ['M6 6l12 12', 'M18 6 6 18'],
  copy: ['M8 8h10v12H8z', 'M6 16H4V4h10v2'],
  mail: ['M4 6h16v12H4z', 'm4 7 8 6 8-6'],
  link: ['M10 13.5a4 4 0 0 0 5.7.2l2-2a4 4 0 0 0-5.7-5.7l-1.1 1.1', 'M14 10.5a4 4 0 0 0-5.7-.2l-2 2a4 4 0 0 0 5.7 5.7l1.1-1.1'],
  back: ['M19 12H5', 'm11 18-6-6 6-6'],
  eye: ['M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z', 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z'],
  'eye-off': ['m3 3 18 18', 'M10.6 6.2A10.8 10.8 0 0 1 12 6c6 0 9.5 6 9.5 6a17.3 17.3 0 0 1-3.1 3.7', 'M6.2 6.2C3.8 7.8 2.5 12 2.5 12s3.5 6 9.5 6a10.8 10.8 0 0 0 3.4-.6', 'M9.9 9.9a3 3 0 0 0 4.2 4.2'],
  refresh: ['M20 11a8 8 0 0 0-14.8-4L3 10', 'M3 4v6h6', 'M4 13a8 8 0 0 0 14.8 4L21 14', 'M21 20v-6h-6'],
}

export default function Icon({ name, size = 18, ...props }: { name: keyof typeof paths; size?: number } & SVGProps<SVGSVGElement>) {
  return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>{paths[name].map((path) => <path key={path} d={path} />)}</svg>
}
