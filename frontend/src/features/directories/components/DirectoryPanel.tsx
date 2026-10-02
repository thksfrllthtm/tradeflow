import { FormEvent, ReactNode, useEffect } from 'react'
import '@/shared/styles/drawer.css'

type Props = {
  title: string
  subtitle?: string
  dirty: boolean
  saving: boolean
  error: string
  onClose: () => void
  onSubmit: () => void
  // Только просмотр (у сотрудников без прав администратора): поля недоступны, кнопки «Сохранить» нет.
  readOnly?: boolean
  children: ReactNode
}

// Боковая панель справочника: шапка, прокручиваемая форма и всегда видимые кнопки внизу.
// Esc, клик по фону и «Отмена» спрашивают подтверждение, если есть несохранённые изменения.
export function DirectoryPanel({ title, subtitle, dirty, saving, error, onClose, onSubmit, readOnly = false, children }: Props) {
  function close() { if (!dirty || window.confirm('Есть несохранённые изменения. Закрыть форму?')) onClose() }

  useEffect(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => { if (dirty) { event.preventDefault(); event.returnValue = '' } }
    window.addEventListener('beforeunload', beforeUnload)
    return () => window.removeEventListener('beforeunload', beforeUnload)
  }, [dirty])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape' && !event.defaultPrevented) close() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  })

  function submit(event: FormEvent) {
    event.preventDefault()
    if (!saving && !readOnly) onSubmit()
  }

  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) close() }}>
    <form className="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title" onSubmit={submit}>
      <header className="drawer-head">
        <div><h2 id="drawer-title">{title}</h2>{subtitle && <p>{subtitle}</p>}</div>
        <button type="button" className="drawer-close" onClick={close} aria-label="Закрыть" title="Закрыть (Esc)">×</button>
      </header>
      <div className="drawer-body"><fieldset className="drawer-fields" disabled={readOnly}>{children}</fieldset></div>
      {readOnly
        ? <footer className="drawer-foot">
          <span className="drawer-state">Только просмотр: менять справочник может администратор</span>
          <button type="button" className="drawer-cancel" onClick={close}>Закрыть</button>
        </footer>
        : <footer className="drawer-foot">
          {error ? <p className="drawer-error" role="alert">{error}</p> : <span className={dirty ? 'drawer-state dirty' : 'drawer-state'}>{saving ? 'Сохраняем…' : dirty ? 'Есть несохранённые изменения' : 'Без изменений'}</span>}
          <button type="button" className="drawer-cancel" onClick={close}>Отмена</button>
          <button type="submit" className="drawer-save" disabled={saving || !dirty}>{saving ? 'Сохраняем…' : 'Сохранить'}</button>
        </footer>}
    </form>
  </div>
}
