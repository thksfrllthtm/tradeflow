import { splitLinks } from '@/shared/lib/links'

// Текст ответа агента с кликабельными ссылками (открываются в новой вкладке).
export function LinkText({ text }: { text: string }) {
  return <>{splitLinks(text).map((part, index) => part.url
    ? <a key={index} href={part.url} target="_blank" rel="noreferrer">{part.text}</a>
    : part.text)}</>
}
