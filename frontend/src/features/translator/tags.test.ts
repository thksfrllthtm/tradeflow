import { describe, expect, it } from 'vitest'
import { fromEditorDoc, parseTagged, toEditorDoc } from './tags'
import { skippedSummary } from './translatorApi'

describe('parseTagged', () => {
  it('splits base text, formatted fragments and inline elements', () => {
    expect(parseTagged('Цена <g1>100 USD</g1><x2/>за шт. &amp; &lt;налог&gt;')).toEqual([
      { text: 'Цена ', tag: 'g0' },
      { text: '100 USD', tag: 'g1' },
      { inline: 'x2' },
      { text: 'за шт. & <налог>', tag: 'g0' },
    ])
  })

  it('keeps broken tags as text', () => {
    expect(parseTagged('<g1>без конца')).toEqual([{ text: '<g1>без конца', tag: 'g0' }])
  })
})

describe('skippedSummary', () => {
  it('lists what stays untranslated', () => {
    expect(skippedSummary({ headers: 2, textboxes: 2, formulas: 1, footnotes: 1 })).toBe('2 надписи, колонтитулы, сноски, 1 формула')
    expect(skippedSummary({ sheets: 2, inline: 1 })).toBe('2 названия листов, текст, записанный прямо в ячейки')
    expect(skippedSummary({})).toBe('')
  })
})

describe('editor document', () => {
  it('round-trips tagged text', () => {
    const text = 'Цена <g1>100 USD</g1><x2/>за шт. &amp; &lt;налог&gt;'
    const doc = toEditorDoc(text)
    expect(doc.content?.[0].content).toEqual([
      { type: 'text', text: 'Цена ' },
      { type: 'text', text: '100 USD', marks: [{ type: 'g', attrs: { id: 'g1' } }] },
      { type: 'x', attrs: { id: 'x2' } },
      { type: 'text', text: 'за шт. & <налог>' },
    ])
    expect(fromEditorDoc(doc)).toBe(text)
  })

  it('merges split text with one mark and joins pasted paragraphs', () => {
    expect(fromEditorDoc({ type: 'doc', content: [
      { type: 'paragraph', content: [{ type: 'text', text: 'А', marks: [{ type: 'g', attrs: { id: 'g1' } }] }, { type: 'text', text: 'Б', marks: [{ type: 'g', attrs: { id: 'g1' } }] }] },
      { type: 'paragraph', content: [{ type: 'text', text: 'В' }] },
    ] })).toBe('<g1>АБ</g1> В')
  })
})
