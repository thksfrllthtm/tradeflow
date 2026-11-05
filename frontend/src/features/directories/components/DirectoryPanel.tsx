import { ReactNode, useEffect } from 'react'
import { Drawer } from '@/shared/components/Drawer/Drawer'

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

  const footer = readOnly
    ? <>
      <span className="drawer-state">Только просмотр: менять справочник может администратор</span>
      <button type="button" className="secondary" onClick={close}>Закрыть</button>
    </>
    : <>
      {error ? <p className="drawer-error" role="alert">{error}</p> : <span className={dirty ? 'drawer-state dirty' : 'drawer-state'}>{saving ? 'Сохраняем…' : dirty ? 'Есть несохранённые изменения' : 'Без изменений'}</span>}
      <button type="button" className="secondary" onClick={close}>Отмена</button>
      <button type="submit" disabled={saving || !dirty}>{saving ? 'Сохраняем…' : 'Сохранить'}</button>
    </>

  return <Drawer title={title} subtitle={subtitle} onClose={close} footer={footer} onSubmit={() => { if (!saving && !readOnly) onSubmit() }}>
    <fieldset className="drawer-fields" disabled={readOnly}>{children}</fieldset>
  </Drawer>
}
