import { ReactNode, useId } from 'react'
import { CloseIcon } from '@/shared/components/icons'
import { useDialogFocus } from '@/shared/hooks/useDialogFocus'
import { useEscape } from '@/shared/hooks/useEscape'
import '@/shared/styles/modal.css'

type Props = {
  title: ReactNode
  subtitle?: ReactNode
  onClose: () => void
  // Кнопки внизу: «Отмена» слева, главная справа.
  footer?: ReactNode
  // Дополнительно в шапке справа от заголовка (например, миниатюры фото).
  headExtra?: ReactNode
  // Ширина окна в px (по умолчанию 560).
  width?: number
  className?: string
  children: ReactNode
}

// Окно по центру поверх затемнённой страницы: шапка с крестиком, прокручиваемое содержимое, кнопки внизу.
export function Modal({ title, subtitle, onClose, footer, headExtra, width = 560, className, children }: Props) {
  useEscape(onClose)
  const ref = useDialogFocus<HTMLDivElement>()
  const titleId = useId()

  return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <div className={className ? `modal ${className}` : 'modal'} style={{ width: `min(${width}px, 100%)` }} ref={ref} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby={titleId}>
      <header className="modal-head">
        <div className="modal-title"><h2 id={titleId}>{title}</h2>{subtitle && <p>{subtitle}</p>}</div>
        {headExtra}
        <button type="button" className="icon-button ghost" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)"><CloseIcon /></button>
      </header>
      <div className="modal-body">{children}</div>
      {footer && <footer className="modal-foot">{footer}</footer>}
    </div>
  </div>
}
