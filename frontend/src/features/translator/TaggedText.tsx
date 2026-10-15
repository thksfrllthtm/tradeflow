import { Fragment } from 'react'
import { parseTagged } from './tags'
import { TagView } from './translatorApi'

// Элементы Word без текста: значок и подсказка.
export const inlineMarks: Record<TagView['kind'], [string, string]> = {
  text: ['', ''],
  tab: ['⇥', 'Табуляция'],
  br: ['↵', 'Перенос строки'],
  image: ['▣', 'Картинка'],
  field: ['{ }', 'Поле Word (номер страницы, ссылка, оглавление)'],
  formula: ['ƒ', 'Формула — не переводится'],
  other: ['◆', 'Элемент Word'],
}

// Текст сегмента с форматированием тегов (жирный, курсив, подчёркивание) вместо самих тегов.
export function TaggedText({ text, tags }: { text: string; tags: Record<string, TagView> }) {
  return <>{parseTagged(text).map((part, index) => {
    if ('inline' in part) {
      const [mark, title] = inlineMarks[tags[part.inline]?.kind ?? 'other']
      return <Fragment key={index}><span className="tr-inline" title={title}>{mark}</span>{tags[part.inline]?.kind === 'br' && <br />}</Fragment>
    }
    const style = tags[part.tag]
    return <span key={index} style={{ fontWeight: style?.bold ? 700 : undefined, fontStyle: style?.italic ? 'italic' : undefined, textDecoration: style?.underline ? 'underline' : undefined }}>{part.text}</span>
  })}</>
}

// То же форматирование строкой CSS — для редактора (TipTap рисует теги сам).
export function tagStyle(style?: TagView) {
  return [style?.bold && 'font-weight:700', style?.italic && 'font-style:italic', style?.underline && 'text-decoration:underline'].filter(Boolean).join(';')
}
