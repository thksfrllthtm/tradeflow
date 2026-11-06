import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { photosOf } from '@/shared/lib/itemPhotos'
import { formatCode } from '@/shared/tnved/codes'
import { autoNetFor, DEFAULT_PACKAGE_WEIGHT, effectivePackageWeight, netHint } from '../../lib/netWeight'
import { autoQuantity, autoQuantityHint, QuantityField } from '../../lib/quantities'
import { hasNomenclatureRates } from '../../lib/reviewStatus'
import { dutyRateText, fixedNumber, itemNames } from '../../lib/resultValues'
import { AgentRowStatus } from '../../agent/agentApi'
import { canSaveRowDecision } from '../../hooks/useRowDecisions'
import { SheetKey } from './sheetModel'
import type { SheetRowView } from './ReviewSheet'

// Разметка ячеек листа: номер с состоянием, фото, наименование, код со значками, авто-значения.
export type CellView = { html: string; title: string; className?: string; key?: string }

export const escape = (text: string) => text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
export const ru = (text: string) => text.replace(/(\d)\.(\d)/g, '$1,$2')
const auto = (text: string) => `<span class="xs-auto">${escape(text)}</span>`
// Количество — целое; дробное (места из количества ÷ штук в месте) — как есть, чтобы ошибку было видно.
export const quantityText = (value: number) => formatRuNumber(Math.abs(value - Math.round(value)) < 1e-9 ? String(Math.round(value)) : String(Number(value.toFixed(3))))
// Вес в ячейке — один знак после запятой («20 230,0»); точное значение — в строке значения над таблицей.
const weightCell = (value: string) => { const text = fixedNumber(value, 1); return text === '—' ? '' : text }

// Ячейка номера: номер, цветная точка состояния (ошибка — красная, предупреждение — жёлтая),
// флажок выбора строки — при наведении и у выбранных строк.
export function numberView(row: SheetRowView, y: number): CellView {
  const tone = row.problems.some((problem) => problem.tone === 'error') ? 'error' : row.problems.length > 0 ? 'warn' : ''
  const changed = row.status.tone === 'changed'
  const dot = tone ? `<i class="xs-dot ${tone}" aria-hidden="true"></i>` : changed ? '<i class="xs-dot changed" aria-hidden="true"></i>' : ''
  const pick = `<input type="checkbox" class="xs-pick" data-act="select"${row.selected ? ' checked' : ''} aria-label="Выбрать строку ${y + 1}">`
  const title = [row.status.label, ...row.problems.map((problem) => problem.text)].filter(Boolean).join('\n')
  return { html: `${pick}${dot}<span class="xs-num">${y + 1}</span>`, title, className: ['xs-index', row.selected ? 'xs-picked' : '', tone ? `xs-row-${tone}` : ''].filter(Boolean).join(' ') }
}

// Подсветка ячейки с проблемой: ошибка важнее предупреждения; текст проблемы — в подсказке.
function problemOf(row: SheetRowView, key: SheetKey) {
  const own = row.problems.filter((problem) => problem.column === key)
  if (own.length === 0) return { className: '', text: '' }
  return { className: own.some((problem) => problem.tone === 'error') ? 'xs-err' : 'xs-warn', text: own.map((problem) => problem.text).join('\n') }
}

export function cellView(key: SheetKey, row: SheetRowView, defaultPackageWeight: string, repeat: boolean): CellView | null {
  const { item } = row
  const problem = problemOf(row, key)
  const withProblem = (view: CellView): CellView => problem.className
    ? { ...view, title: [problem.text, view.title].filter(Boolean).join('\n'), className: [view.className, problem.className].filter(Boolean).join(' ') }
    : view
  switch (key) {
    case 'photo': {
      const photos = photosOf(item)
      const add = `<button type="button" class="xs-photo-add${photos.length ? '' : ' empty'}" data-act="upload" title="Добавить фото" aria-label="Добавить фото">+</button>`
      if (photos.length === 0) return { html: add, title: '' }
      const html = `<button type="button" class="xs-thumb" data-act="photos" title="Фото строки: посмотреть, удалить, перенести"><img src="${escape(photos[0])}" alt="">${photos.length > 1 ? `<b>${photos.length}</b>` : ''}</button>${add}`
      return { html, title: '', key: `${photos.length}:${photos[0].length}:${photos[0].slice(-24)}` }
    }
    case 'name': {
      // Русское название и через точку серым — китайское (или исходное из Excel).
      const names = itemNames(item, item)
      const other = (item.otherNames?.length ? `\nДругие наименования из файла: ${item.otherNames.join(' · ')}` : '')
        + (item.details?.length ? `\nДанные для ИИ: ${item.details.join(' · ')}` : '')
      const html = item.name ? `${escape(names.main)}${names.sub ? ` <span class="xs-zh">· ${escape(names.sub)}</span>` : ''}` : auto('Название товара')
      return withProblem({ html, title: item.name + other, className: repeat ? 'xs-repeat' : '' })
    }
    case 'code': {
      // Нет кода — тихая ссылка «Указать код» (справочник) и значок ИИ (чат подбора, с отметкой его состояния):
      // о проблеме говорят точка у номера и фон ячейки. Код — моноширинным, метка «ИИ», если его предложил
      // подбор; значки справочника, подбора ИИ и базы решений — у выбранной строки и при наведении.
      if (!item.code) {
        const empty = agentView(row.agent, '')
        const agentTitle = row.agent ? empty.title : 'Подобрать код ИИ'
        const html = item.excluded ? '' : '<span class="xs-set"><button type="button" class="button-link xs-set-code" data-act="picker" title="Выбрать товар или код ТН ВЭД из справочника">Указать код</button>'
          + `<button type="button" class="icon-button ghost sm xs-tool agent ${empty.tone}" data-act="agent" title="${escape(agentTitle)}" aria-label="${escape(agentTitle)}">${ICONS.agent}${empty.mark ? `<i>${escape(empty.mark)}</i>` : ''}</button></span>`
        return withProblem({ html, title: 'Выберите код в справочнике, подберите его с ИИ или введите (двойной щелчок или просто начните печатать)', className: 'xs-code-empty' })
      }
      const agent = agentView(row.agent, item.code)
      const saved = row.decision === 'saved' || typeof row.decision === 'number'
      const decision = !canSaveRowDecision(item) ? ''
        : saved
          ? `<span class="xs-tool saved" title="${typeof row.decision === 'number' ? `Этот товар уже сохранён в базу решений из строки ${row.decision}` : 'Строка сохранена в базу решений (ждёт проверки администратором)'}">${ICONS.saved}</span>`
          : `<button type="button" class="xs-tool" data-act="decision"${row.decision === 'saving' ? ' disabled' : ''} title="${row.decision === 'saving' ? 'Сохраняем в базу решений…' : 'В базу решений: код, наименование и фото строки (ждёт проверки администратором)'}">${ICONS.decision}</button>`
      const tools = `<span class="xs-tools">`
        + `<button type="button" class="xs-tool" data-act="picker" title="Выбрать товар или код ТН ВЭД из справочника">${ICONS.picker}</button>`
        + `<button type="button" class="xs-tool agent ${agent.tone}" data-act="agent" title="${escape(agent.title)}">${ICONS.agent}${agent.mark ? `<i>${escape(agent.mark)}</i>` : ''}</button>`
        + `${decision}</span>`
      const verified = hasNomenclatureRates(item)
      const ai = row.aiCode ? '<span class="xs-ai" title="Код предложил подбор ИИ">ИИ</span>' : ''
      const title = verified ? 'Ставки из номенклатуры ТН ВЭД ЕАЭС' : 'Кода нет в номенклатуре ТН ВЭД ЕАЭС или она не загружена'
      return withProblem({ html: `<span class="xs-code-cell"><span class="xs-code-text">${escape(formatCode(item.code))}</span>${ai}${tools}</span>`, title, className: ['xs-code', repeat ? 'xs-repeat' : ''].filter(Boolean).join(' ') })
    }
    case 'duty': {
      const rate = dutyRateText(item.directorySnapshot).replace(/^ЕТТ\s*/, '')
      return { html: rate ? escape(rate) : '—', title: rate ? `Пошлина ЕТТ из номенклатуры: ${rate}` : 'Ставки нет: код не указан или его нет в номенклатуре', className: ['xs-duty', repeat ? 'xs-repeat' : ''].filter(Boolean).join(' ') }
    }
    case 'places': case 'pairsPerPlace': case 'pairs': {
      if (item[key]) return withProblem({ html: escape(formatRuNumber(item[key])), title: '' })
      // Пустое поле, которое рассчитается: значение серым, формула — в подсказке.
      const hint = autoQuantityHint(item, key as QuantityField)
      const value = autoQuantity(item, key as QuantityField)
      return withProblem({ html: value !== null ? auto(quantityText(value)) : '', title: hint ? ru(hint.title.replace('Рассчитается автоматически', 'Рассчитано')) : '' })
    }
    case 'grossKg': {
      const need = !item.excluded && !item.grossKg.trim() && !item.netKg.trim()
      return withProblem({ html: escape(weightCell(item.grossKg)), title: need ? 'Укажите брутто: без него не посчитать авто-нетто' : '', className: need ? 'xs-need' : '' })
    }
    case 'packageWeightKg': {
      const own = item.packageWeightKg.trim() !== '' && item.packageWeightKg !== defaultPackageWeight
      const title = own ? `Своя упаковка места для этой строки, кг (общая — ${ru(defaultPackageWeight || DEFAULT_PACKAGE_WEIGHT)} кг)` : 'Вес упаковки одного места, кг; влияет только на авто-нетто'
      return { html: item.packageWeightKg ? escape(formatRuNumber(item.packageWeightKg)) : auto(ru(defaultPackageWeight || DEFAULT_PACKAGE_WEIGHT)), title, className: [own ? 'xs-own' : '', item.netKg ? 'xs-muted' : ''].filter(Boolean).join(' ') }
    }
    case 'netKg': {
      const netItem = { ...item, packageWeightKg: effectivePackageWeight(item, defaultPackageWeight) }
      const hint = netHint(netItem)
      const autoNet = autoNetFor(netItem)
      const html = item.netKg ? escape(weightCell(item.netKg)) : autoNet !== null ? auto(weightCell(String(autoNet))) : ''
      return withProblem({ html, title: ru(hint.text) + (item.netKg && item.netSource !== 'excel' ? '\nDelete — вернуть авто-нетто' : ''), className: hint.className === 'net-bad' && !problem.className ? 'xs-err' : '' })
    }
    default:
      return null
  }
}

// Значки кнопок в ячейке кода: справочник (лупа), подбор ИИ (искра), база решений (закладка).
const icon = (body: string) => `<svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`
const ICONS = {
  picker: icon('<circle cx="7" cy="7" r="4.2"/><path d="m10.2 10.2 3.3 3.3"/>'),
  agent: icon('<path d="M8 1.8 9.4 6.6 14.2 8 9.4 9.4 8 14.2 6.6 9.4 1.8 8 6.6 6.6Z"/>'),
  decision: icon('<path d="M4 2.5h8v11L8 10.8 4 13.5Z"/><path d="M8 5v3.4M6.3 6.7h3.4"/>'),
  saved: icon('<path d="M4 2.5h8v11L8 10.8 4 13.5Z" fill="currentColor" fill-opacity=".18"/><path d="m6 6.6 1.5 1.5 2.6-2.8"/>'),
}

// Кнопка чата подбора ИИ: отметка состояния (…, ?, !, ✓ или предложенный код).
function agentView(status: AgentRowStatus | undefined, code: string): { tone: string; mark: string; title: string } {
  if (status?.status === 'running') return { tone: 'running', mark: '…', title: 'Идёт подбор кода — открыть чат' }
  if (status?.status === 'searching') return { tone: 'running', mark: '…', title: 'Поиск в базе решений — открыть чат' }
  if (status?.status === 'ready') return { tone: 'idle', mark: '', title: 'Поиск в базе закончен — открыть чат и начать подбор' }
  if (status?.status === 'waiting') return { tone: 'waiting', mark: '?', title: 'Подбор ждёт ответа на вопрос — открыть чат' }
  if (status?.status === 'error') return { tone: 'error', mark: '!', title: 'Подбор прервался — открыть чат' }
  if (status?.status === 'done' && status.bestCode) {
    if (!code) return { tone: 'suggest', mark: '!', title: `Подбор предложил код ${formatCode(status.bestCode)} — открыть чат и применить` }
    return status.bestCode === code
      ? { tone: 'same', mark: '✓', title: 'Код совпадает с подбором — открыть чат' }
      : { tone: 'suggest', mark: '≠', title: `Подбор предлагает другой код ${formatCode(status.bestCode)} — открыть чат` }
  }
  return { tone: 'idle', mark: '', title: status ? 'Открыть чат подбора' : 'Подобрать код ТН ВЭД с ИИ' }
}
