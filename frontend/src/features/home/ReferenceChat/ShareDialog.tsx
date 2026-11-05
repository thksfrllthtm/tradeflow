import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { ShareUser, agentApi } from '@/features/goods/agent/agentApi'
import { errorMessage } from '@/shared/lib/errorMessage'

// Окно «Поделиться с сотрудником»: открыть вопрос коллегам (они увидят его в своей истории и только читают)
// и закрыть доступ тем, кому он уже открыт. Права проверяет сервер: открыть может автор или администратор.
export function ShareDialog({ sessionId, onClose }: { sessionId: number; onClose: () => void }) {
  const [users, setUsers] = useState<ShareUser[]>([])
  const [shares, setShares] = useState<ShareUser[]>([])
  const [selected, setSelected] = useState<number[]>([])
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    let active = true
    Promise.all([agentApi.shareUsers(), agentApi.shares(sessionId)])
      .then(([list, current]) => { if (active) { setUsers(list.items ?? []); setShares(current.items ?? []) } })
      .catch((reason) => { if (active) setError(errorMessage(reason)) })
      .finally(() => { if (active) setLoading(false) })
    searchRef.current?.focus()
    return () => { active = false }
  }, [sessionId])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  const sharedIds = useMemo(() => new Set(shares.map((user) => user.id)), [shares])
  const shown = useMemo(() => {
    const words = query.trim().toLowerCase()
    return users.filter((user) => !sharedIds.has(user.id) && (!words || user.name.toLowerCase().includes(words)))
  }, [users, sharedIds, query])

  function toggle(id: number) {
    setDone('')
    setSelected((current) => current.includes(id) ? current.filter((value) => value !== id) : [...current, id])
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (selected.length === 0 || busy) return
    setBusy(true); setError('')
    try {
      const result = await agentApi.share(sessionId, selected)
      setShares(result.items ?? [])
      setDone(selected.length === 1 ? 'Доступ открыт' : `Доступ открыт: ${selected.length}`)
      setSelected([])
      setQuery('')
    } catch (reason) { setError(errorMessage(reason)) } finally { setBusy(false) }
  }

  async function revoke(user: ShareUser) {
    setBusy(true); setError(''); setDone('')
    try {
      await agentApi.unshare(sessionId, user.id)
      setShares((current) => current.filter((value) => value.id !== user.id))
    } catch (reason) { setError(errorMessage(reason)) } finally { setBusy(false) }
  }

  return <div className="rc-share-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) onClose() }}>
    <form className="rc-share" role="dialog" aria-modal="true" aria-labelledby="rc-share-title" onSubmit={(event) => void submit(event)}>
      <header>
        <h3 id="rc-share-title">Поделиться с сотрудником</h3>
        <button type="button" className="rc-share-close" onClick={onClose} disabled={busy} aria-label="Закрыть">×</button>
      </header>
      <p className="rc-share-help">Сотрудник увидит вопрос в своей истории и сможет только читать: уточнять и перезапускать может только автор.</p>

      {shares.length > 0 && <div className="rc-share-current">
        <b>Доступ открыт</b>
        <ul>{shares.map((user) => <li key={user.id}>{user.name}
          <button type="button" onClick={() => void revoke(user)} disabled={busy} title={`Закрыть доступ: ${user.name}`} aria-label={`Закрыть доступ: ${user.name}`}>×</button>
        </li>)}</ul>
      </div>}

      <input ref={searchRef} type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск по имени" aria-label="Поиск сотрудника по имени" />
      <ul className="rc-share-list" aria-label="Сотрудники">
        {loading ? <li className="rc-share-empty">Загружаю список…</li>
          : shown.length === 0 ? <li className="rc-share-empty">{users.length === 0 ? 'Других сотрудников нет' : 'Никого не нашлось'}</li>
          : shown.map((user) => <li key={user.id}>
            <label><input type="checkbox" checked={selected.includes(user.id)} onChange={() => toggle(user.id)} />{user.name}</label>
          </li>)}
      </ul>

      {error && <p className="rc-error" role="alert">{error}</p>}
      <footer>
        <span role="status">{done}</span>
        <button type="button" className="rc-ghost" onClick={onClose} disabled={busy}>Готово</button>
        <button type="submit" className="rc-share-submit" disabled={busy || selected.length === 0}>
          {selected.length > 1 ? `Открыть доступ (${selected.length})` : 'Открыть доступ'}
        </button>
      </footer>
    </form>
  </div>
}
