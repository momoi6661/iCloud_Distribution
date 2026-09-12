import { useEffect, useState, type FormEvent } from 'react'

export default function Pagination({ page, totalPages, onChange, label }: { page: number; totalPages: number; onChange: (page: number) => void; label: string }) {
  const [draft, setDraft] = useState(String(page))

  useEffect(() => { setDraft(String(page)) }, [page])

  const commit = () => {
    const parsed = Number.parseInt(draft, 10)
    const next = Number.isFinite(parsed) ? Math.min(totalPages, Math.max(1, parsed)) : page
    setDraft(String(next))
    if (next !== page) onChange(next)
  }

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    commit()
  }

  return <nav className="pagination" aria-label={label}>
    <button className="button secondary" disabled={page === 1} onClick={() => onChange(page - 1)}>上一页</button>
    <form className="pagination-jump" onSubmit={submit}>
      <label htmlFor={`${label}-page`}>第</label>
      <input id={`${label}-page`} type="number" inputMode="numeric" min={1} max={totalPages} value={draft} onChange={(event) => setDraft(event.target.value)} onBlur={commit} aria-label="输入页码" />
      <span>/ {totalPages} 页</span>
    </form>
    <button className="button secondary" disabled={page === totalPages} onClick={() => onChange(page + 1)}>下一页</button>
  </nav>
}
