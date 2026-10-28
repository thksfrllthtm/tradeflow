import { describe, expect, it } from 'vitest'
import type { AgentMessage, AgentResult } from '@/features/goods/agent/agentApi'
import { documentsCode, menuCommands, parseCommand, splitDocumentsArg } from './commands'

const result = (content: Partial<AgentResult>) => ({ id: 1, kind: 'result', createdAt: '', content: { description: '', name: '', notes: '', candidates: [], ...content } }) as unknown as AgentMessage

describe('commands', () => {
  it('меню — пока набрано только «/имя»', () => {
    expect(menuCommands('/').length).toBe(4)
    expect(menuCommands('/до').map((command) => command.name)).toEqual(['документы'])
    expect(menuCommands('/документы 8205')).toEqual([])
    expect(menuCommands('лопата')).toEqual([])
  })

  it('команда по сокращению, с аргументом; неизвестная — unknown', () => {
    expect(parseCommand('/док 8205 59 100 0')).toMatchObject({ command: { name: 'документы' }, arg: '8205 59 100 0' })
    expect(parseCommand('/Подбор  лопата\nстальная')).toMatchObject({ command: { name: 'подбор' }, arg: 'лопата\nстальная' })
    expect(parseCommand('/xyz')).toBe('unknown')
    expect(parseCommand('лопата / черенок')).toBeNull()
  })

  it('текст пузыря «Документы для ввоза · код» — команда документов, а не вопрос', () => {
    expect(parseCommand('Документы для ввоза · 3827 61 000 0')).toMatchObject({ command: { name: 'документы' }, arg: '3827 61 000 0' })
    expect(parseCommand('документы 2903450000')).toMatchObject({ command: { name: 'документы' }, arg: '2903450000' })
    expect(parseCommand('Документы для ввоза')).toMatchObject({ command: { name: 'документы' }, arg: '' })
    expect(parseCommand('какие документы для ввоза нужны?')).toBeNull()
  })

  it('код документов — указанный или лучший из последнего ответа о коде', () => {
    const messages = [
      result({ candidates: [{ code: '1111111111', verdict: 'fits' }, { code: '2222222222', verdict: 'no' }] as AgentResult['candidates'] }),
      result({ candidates: [{ code: '3333333333', verdict: 'no' }, { code: '4444444444', verdict: 'doubtful' }] as AgentResult['candidates'] }),
      result({ documentsCode: '4444444444', candidates: [] }),
    ]
    expect(documentsCode('', messages)).toBe('4444444444')
    expect(documentsCode('8205 59 100 0', messages)).toBe('8205591000')
    expect(documentsCode('', [])).toBe('')
    expect(documentsCode('Наждачный круг P120 6804 22 500 0', [])).toBe('6804225000')
  })

  it('/документы товар и код — код отдельно от товара', () => {
    expect(splitDocumentsArg('Наждачный шлифовальный круг / 砂布 6804225000')).toEqual({ code: '6804225000', product: 'Наждачный шлифовальный круг / 砂布' })
    expect(splitDocumentsArg('6804 22 500 0 круг P120')).toEqual({ code: '6804225000', product: 'круг P120' })
    expect(splitDocumentsArg('круг 125 мм')).toEqual({ code: '125', product: 'круг 125 мм' })
    expect(splitDocumentsArg('')).toEqual({ code: '', product: '' })
  })
})
