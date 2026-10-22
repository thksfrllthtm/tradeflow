import { ReactNode, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { User } from '@/shared/types/user'
import { UtilFeeInput, UtilFeeMatch, UtilFeeResult, UtilFeeSection, utilFeeApi } from './utilFeeApi'
import '@/shared/styles/list-page.css'
import './utilFee.css'

// МРП на 2026 год — значение по умолчанию; сотрудник может поменять.
const DEFAULT_MRP = '4325'

const kindLabels: Record<UtilFeeMatch['kind'], string> = {
  product: 'платится за товар',
  packaging: 'платится за упаковку',
  vehicle: 'транспорт и техника',
}

// Как платится — коротко, цветным текстом в колонке таблицы перечня (как статусы на этапах сметы).
const payLabels: Record<UtilFeeMatch['kind'], string> = { product: 'за товар', packaging: 'за упаковку', vehicle: 'транспорт, техника' }

const money = (value: string) => `${formatRuNumber(value)} ₸`
const digits = (code: string) => code.replace(/\s/g, '')
// Код любой длины — теми же группами, что полный (4-2-3-1): «8701 21 101», «4011 20», «8516 50 000 0».
const groupCode = (code: string) => [code.slice(0, 4), code.slice(4, 6), code.slice(6, 9), code.slice(9)].filter(Boolean).join(' ')

// Калькулятор утильсбора: код ТН ВЭД → позиции перечня № 689 и коэффициенты, затем сотрудник вводит массу
// товара, упаковки или категорию транспорта — сервер считает суммы по Методике № 448.
export function UtilFeePage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [input, setInput] = useState<UtilFeeInput>({ code: '', mrp: DEFAULT_MRP, massKg: '', packaging: [{ material: 'paper', massKg: '' }], vehicleCategory: '', fromBelarusRussia: false })
  const [result, setResult] = useState<UtilFeeResult | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const debounced = useDebouncedValue(input, 350)
  const codeReady = /^\d{4,10}$/.test(digits(debounced.code))

  useEffect(() => {
    if (!codeReady) { setResult(null); setError(''); return }
    let current = true
    setLoading(true)
    utilFeeApi.calculate(debounced)
      .then((next) => { if (current) { setResult(next); setError('') } })
      .catch((reason) => { if (current) setError(errorMessage(reason)) })
      .finally(() => { if (current) setLoading(false) })
    return () => { current = false }
  }, [debounced, codeReady])

  const [sections, setSections] = useState<UtilFeeSection[]>([])
  const [edition, setEdition] = useState('')
  useEffect(() => { utilFeeApi.list().then((list) => { setSections(list.sections); setEdition(list.edition) }).catch((reason) => setError(errorMessage(reason))) }, [])

  const set = (change: Partial<UtilFeeInput>) => setInput((value) => ({ ...value, ...change }))
  const setRow = (index: number, change: Partial<UtilFeeInput['packaging'][number]>) =>
    set({ packaging: input.packaging.map((row, rowIndex) => rowIndex === index ? { ...row, ...change } : row) })
  // Новый код — новый товар: введённые массы и категория к нему не относятся.
  const setCode = (code: string) => {
    if (digits(code) === digits(input.code)) return set({ code })
    set({ code, massKg: '', packaging: [{ material: 'paper', massKg: '' }], vehicleCategory: '', fromBelarusRussia: false })
  }

  const has = (kind: UtilFeeMatch['kind']) => result?.matches.some((match) => match.kind === kind) ?? false
  const productK = result?.matches.find((match) => match.kind === 'product')?.k

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="utilFee" onLogout={onLogout} />
    <section className="directory-page uf-page">
      <div className="dir-head">
        <div>
          <h1>Утильсбор</h1>
          <p>Введите код ТН ВЭД — программа покажет раздел перечня и коэффициент. Затем заполните массу или категорию — сумма посчитается сама.</p>
        </div>
      </div>
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}

      {/* Одна карточка: сверху поля, под ними перечень. Одно поле на всё: пока в нём меньше 4 цифр или слово —
          перечень фильтруется по нему; 4–10 цифр — перечень скрыт, ниже расчёт. Карточка не пересоздаётся —
          курсор в поле не слетает. */}
      <ListTable sections={sections} query={input.code} edition={edition} onPick={setCode}
        showList={!/^\d{4,}$/.test(digits(input.code)) && sections.length > 0}
        fields={<>
          <label className="uf-code">Код ТН ВЭД<input value={input.code} onChange={(event) => setCode(event.target.value)} placeholder="код (8516500000) или слово (шины)" autoFocus /></label>
          <label className="uf-mrp" title="Месячный расчётный показатель; по умолчанию — на 2026 год">МРП<input value={input.mrp} onChange={(event) => set({ mrp: event.target.value })} inputMode="decimal" /><span>₸</span></label>
        </>} />

      {/^\d{11,}$/.test(digits(input.code)) && <p className="uf-hint">В коде ТН ВЭД не больше 10 цифр.</p>}
      {codeReady && !result && loading && <p className="uf-hint">Ищем код в перечне…</p>}

      {result && result.code === digits(debounced.code) && <>
        {result.matches.length === 0 && result.possible.length === 0 &&
          <div className="uf-card uf-empty">Код <code>{groupCode(result.code)}</code> не входит в перечень — утильсбор не платится.</div>}
        {result.matches.length > 0 && <div className="uf-card">
          <h2>Код <code>{groupCode(result.code)}</code> в перечне</h2>
          <ul className="uf-matches">
            {result.matches.map((match) => <li key={match.section}>
              <strong>Раздел {match.section}, п. {match.item}</strong> <span className={`uf-kind uf-kind-${match.kind}`}>{kindLabels[match.kind]}</span>
              {match.k !== undefined && <span className="uf-k">k = {formatRuNumber(match.k)}</span>}
              <div className="uf-title">{match.title} · позиция <code>{groupCode(match.prefix)}</code></div>
              {match.note && <div className="uf-note">⚠ {match.note}</div>}
            </li>)}
          </ul>
        </div>}
        {result.possible.length > 0 && <div className="uf-card">
          <h2>Зависит от полного кода</h2>
          <p className="uf-formula">Эти позиции перечня начинаются с <code>{groupCode(result.code)}</code>. Введите код полностью или выберите позицию.</p>
          <ul className="uf-matches">
            {[...new Set(result.possible.map((match) => match.section))].map((sectionNumber) => {
              const items = result.possible.filter((match) => match.section === sectionNumber)
              return <li key={sectionNumber}>
                <strong>Раздел {sectionNumber}</strong> <span className={`uf-kind uf-kind-${items[0].kind}`}>{kindLabels[items[0].kind]}</span>
                {items[0].k !== undefined && <span className="uf-k">k = {formatRuNumber(items[0].k)}</span>}
                <div className="uf-title">{items[0].title}</div>
                <div className="uf-positions">
                  {items.map((match) => <button type="button" key={match.item} title={`п. ${match.item}`} onClick={() => setCode(match.prefix)}>{groupCode(match.prefix)}</button>)}
                </div>
              </li>
            })}
          </ul>
        </div>}

        {has('product') && <div className="uf-card">
          <h2>За товар</h2>
          <p className="uf-formula">масса (т) × МРП × k{productK !== undefined && ` (${formatRuNumber(productK)})`}</p>
          <div className="uf-row">
            <label>Масса нетто, кг<input value={input.massKg} onChange={(event) => set({ massKg: event.target.value })} inputMode="decimal" /></label>
            {result.product && <span className="uf-amount">{money(result.product.amount)}</span>}
          </div>
        </div>}

        {has('packaging') && <div className="uf-card">
          <h2>За упаковку</h2>
          <p className="uf-formula">масса упаковки (т) × МРП × k материала</p>
          {input.packaging.map((row, index) => {
            const amount = result.packaging.find((line) => line.index === index)
            return <div className="uf-row" key={index}>
              <label>Материал<select value={row.material} onChange={(event) => setRow(index, { material: event.target.value })}>
                {result.materials.map((material) => <option key={material.id} value={material.id}>{material.label} · k {formatRuNumber(material.k)}</option>)}
              </select></label>
              <label>Масса упаковки, кг<input value={row.massKg} onChange={(event) => setRow(index, { massKg: event.target.value })} inputMode="decimal" /></label>
              {amount && <span className="uf-amount">{money(amount.amount)}</span>}
              {input.packaging.length > 1 && <button type="button" className="uf-remove" aria-label="Удалить строку" onClick={() => set({ packaging: input.packaging.filter((_, rowIndex) => rowIndex !== index) })}>×</button>}
            </div>
          })}
          <button type="button" className="uf-add" onClick={() => set({ packaging: [...input.packaging, { material: 'polymer', massKg: '' }] })}>+ Ещё материал</button>
        </div>}

        {has('vehicle') && <div className="uf-card">
          <h2>Транспорт и техника</h2>
          <p className="uf-formula">50 МРП × коэффициент</p>
          <div className="uf-row">
            <label className="uf-wide">Категория<select value={input.vehicleCategory} onChange={(event) => set({ vehicleCategory: event.target.value })}>
              <option value="">— выберите —</option>
              {(result.vehicleCategories ?? []).map((category) => <option key={category.id} value={category.id}>
                {category.label} · коэф. {formatRuNumber(input.fromBelarusRussia ? category.coef2 : category.coef1)}
              </option>)}
            </select></label>
            <label className="uf-check"><input type="checkbox" checked={input.fromBelarusRussia} onChange={(event) => set({ fromBelarusRussia: event.target.checked })} />Ввоз из Беларуси или России</label>
            {result.vehicle && <span className="uf-amount">{money(result.vehicle.amount)}</span>}
          </div>
        </div>}

        {result.total && <div className="uf-card uf-total">Итого утильсбор: <strong>{money(result.total)}</strong></div>}
        <p className="uf-edition">{result.edition}</p>
      </>}
    </section>
  </main>
}

// Перечень № 689 целиком — одной таблицей, как лист сметы. Разделы свёрнуты: строка раздела (название, как
// платится, k, число позиций) раскрывает его позиции — пункт, код, k, примечание. Поиск — по коду (в обе
// стороны: «8516500000» найдёт позицию «8516») или по словам названия раздела и примечания; найденные
// разделы раскрываются сами.
type ListProps = { sections: UtilFeeSection[]; query: string; edition: string; fields: ReactNode; showList: boolean; onPick: (code: string) => void }

function ListTable({ sections, query, edition, fields, showList, onPick }: ListProps) {
  const [open, setOpen] = useState<Set<number>>(new Set())
  const code = digits(query).replace(/\./g, '')
  const byCode = /^\d+$/.test(code)
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  const searching = words.length > 0
  const shown = sections.map((section) => {
    if (!searching) return section
    if (byCode) return { ...section, items: section.items.filter((item) => item.prefix.startsWith(code) || code.startsWith(item.prefix)) }
    if (words.every((word) => section.title.toLowerCase().includes(word))) return section
    return { ...section, items: section.items.filter((item) => item.note && words.every((word) => item.note!.toLowerCase().includes(word))) }
  }).filter((section) => section.items.length > 0)
  const positions = shown.reduce((sum, section) => sum + section.items.length, 0)
  const isOpen = (number: number) => searching || open.has(number)
  const allOpen = shown.every((section) => isOpen(section.number))
  const toggle = (number: number) => setOpen((current) => {
    const next = new Set(current)
    if (next.has(number)) next.delete(number)
    else next.add(number)
    return next
  })

  return <><section className={showList ? 'uf-sheet' : 'uf-sheet compact'} aria-label="Код и перечень продукции">
    <div className="uf-filterbar">
      {fields}
      {showList && <>
        <p className="uf-count"><b>{shown.length}</b> разделов · <b>{positions}</b> позиций</p>
        <span className="uf-hint uf-tip">Раздел — раскрыть коды; код — открыть расчёт</span>
        {!searching && <button type="button" className="uf-add" onClick={() => setOpen(allOpen ? new Set() : new Set(sections.map((section) => section.number)))}>{allOpen ? 'Свернуть все' : 'Раскрыть все'}</button>}
      </>}
    </div>
    {/* Своя прокрутка у таблицы: перечень — до низа окна, шапка и поля на месте. */}
    {showList && <div className="uf-scroll">
    <table className="sheet uf-table">
      {/* Одни колонки для раздела и его позиций: в строке раздела — название, как платится, k раздела и общее
          примечание; в строке позиции — пункт, код, k пункта и его примечание. */}
      <thead><tr>
        <th className="c-item">Раздел / пункт</th><th className="c-code">Название раздела / код ТН ВЭД</th><th className="c-kind">Платится</th><th className="c-count">Позиций</th>
        <th className="c-k">k</th><th className="c-note">Примечание</th><th className="c-go" aria-label="Расчёт" />
      </tr></thead>
      {shown.length === 0 && <tbody><tr><td colSpan={7} className="uf-none">Ничего не найдено. Код не из перечня — утильсбор не платится.</td></tr></tbody>}
      {shown.map((section) => {
        const ks = new Set(section.items.map((item) => item.k))
        const k = ks.size === 1 ? section.items[0].k : undefined
        // Примечание, одинаковое у всех позиций, — один раз в строке раздела, а не в каждой строке.
        const notes = new Set(section.items.map((item) => item.note ?? ''))
        const commonNote = notes.size === 1 ? section.items[0].note : undefined
        const expanded = isOpen(section.number)
        return <tbody key={section.number}>
          <tr className={expanded ? 'uf-section-row open' : 'uf-section-row'} onClick={() => { if (!searching) toggle(section.number) }}>
            <td className="c-item">
              <span className="uf-section-num">
                <button type="button" className="uf-chevron" aria-expanded={expanded} disabled={searching}
                  aria-label={expanded ? `Свернуть раздел ${section.number}` : `Раскрыть раздел ${section.number}`}
                  onClick={(event) => { event.stopPropagation(); toggle(section.number) }}>
                  <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M4.5 3l3 3-3 3" /></svg>
                </button>
                <b>Раздел {section.number}</b>
              </span>
            </td>
            <td className="c-code uf-section-title" title={section.title}>{section.title}</td>
            <td className="c-kind"><span className={`uf-pay ${section.kind}`}>{payLabels[section.kind]}</span></td>
            <td className="c-count">{section.items.length}</td>
            <td className="c-k">{k !== undefined ? formatRuNumber(k) : ks.size > 1 ? <span className="uf-hint">по пункту</span> : <span className="uf-hint">—</span>}</td>
            <td className="c-note" title={commonNote}>{commonNote ?? ''}</td>
            <td className="c-go" />
          </tr>
          {expanded && section.items.map((item) => <tr key={item.item} className="uf-item" tabIndex={0} title="Посчитать утильсбор по этому коду"
            onClick={() => onPick(item.prefix)} onKeyDown={(event) => { if (event.key === 'Enter') onPick(item.prefix) }}>
            <td className="c-item">{item.item}</td>
            <td className="c-code">{groupCode(item.prefix)}{item.prefix.length < 10 && <span className="uf-hint"> и все коды, что с него начинаются</span>}</td>
            <td className="c-kind" />
            <td className="c-count" />
            <td className="c-k">{item.k !== undefined ? formatRuNumber(item.k) : <span className="uf-hint">—</span>}</td>
            <td className="c-note" title={commonNote ? undefined : item.note}>{commonNote ? '' : item.note ?? ''}</td>
            <td className="c-go"><span>Посчитать →</span></td>
          </tr>)}
        </tbody>
      })}
    </table>
    </div>}
  </section>
  {/* По каким документам перечень и коэффициенты — под рамкой, как пометка под таблицей сметы. */}
  {showList && edition && <p className="uf-disclaimer">
    <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"><circle cx="8" cy="8" r="6.3" /><path d="M8 7.3v3.6" /><circle cx="8" cy="5" r=".4" fill="currentColor" /></svg>
    {edition}
  </p>}
  </>
}
