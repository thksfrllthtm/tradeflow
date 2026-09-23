import { AnchorHTMLAttributes, MouseEvent } from 'react'
import { navigate } from './router'

// Ссылка внутри приложения: обычный клик переходит без перезагрузки,
// Ctrl/⌘-клик и средняя кнопка открывают страницу в новой вкладке.
export function Link({ href, onClick, ...rest }: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) {
  function handleClick(event: MouseEvent<HTMLAnchorElement>) {
    onClick?.(event)
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    navigate(href)
  }
  return <a href={href} onClick={handleClick} {...rest} />
}
