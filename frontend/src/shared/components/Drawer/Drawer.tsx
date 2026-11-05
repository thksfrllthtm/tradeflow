import { FormEvent, ReactNode, useId } from 'react'
import { CloseIcon } from '@/shared/components/icons'
import { useDialogFocus } from '@/shared/hooks/useDialogFocus'
import { useEscape } from '@/shared/hooks/useEscape'
import '@/shared/styles/drawer.css'

type Props = {
  title: ReactNode
  subtitle?: ReactNode
  // Закрытие: крестик, Esc, клик по фону. Подтверждение несохранённых изменений — забота вызывающего.
  onClose: () => void
  // Кнопки внизу: «Отмена» слева, главная справа.
  footer?: ReactNode
  // С onSubmit панель — это <form>: Enter в поле и кнопка type="submit" вызывают его.
  onSubmit?: () => void
  className?: string
  children: ReactNode
}

// Боковая панель справа: шапка с крестиком, прокручиваемое содержимое, кнопки внизу.
export function Drawer({ title, subtitle, onClose, footer, onSubmit, className, children }: Props) {
  useEscape(onClose)
  const ref = useDialogFocus<HTMLFormElement & HTMLElement>()
  const titleId = useId()
  const Shell = onSubmit ? 'form' : 'aside'
  const submit = onSubmit && ((event: FormEvent) => { event.preventDefault(); onSubmit() })

  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <Shell className={className ? `drawer ${className}` : 'drawer'} ref={ref} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby={titleId} onSubmit={submit}>
      <header className="drawer-head">
        <div><h2 id={titleId}>{title}</h2>{subtitle && <p>{subtitle}</p>}</div>
        <button type="button" className="icon-button drawer-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)"><CloseIcon /></button>
      </header>
      <div className="drawer-body">{children}</div>
      {footer && <footer className="drawer-foot">{footer}</footer>}
    </Shell>
  </div>
}
