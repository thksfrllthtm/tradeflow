import { ReactNode, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatMoney, formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { moneyText } from '@/shared/lib/rounding'
import { useTodayRates } from '@/shared/api/todayRates'
import { User } from '@/shared/types/user'
import { UtilFeeInput, UtilFeeMatch, UtilFeeMaterial, UtilFeeResult, UtilFeeSection, utilFeeApi } from './utilFeeApi'
import { DEFAULT_MRP } from './feeRows'
import { UtilFeeRows } from './UtilFeeRows'
import '@/shared/styles/list-page.css'
import './utilFee.css'

// Как платится — коротко, цветным текстом в колонке таблицы перечня (как статусы на этапах сметы).
const payLabels: Record<UtilFeeMatch['kind'], string> = { product: 'за товар', packaging: 'за упаковку', vehicle: 'транспорт, техника' }
// Вывод по коду: «утильсбор платится за товар и за упаковку» и что заполнить для суммы.
const verdictLabels: Record<UtilFeeMatch['kind'], string> = { product: 'за товар', packaging: 'за упаковку', vehicle: 'как за транспорт или технику' }
const needLabels: Record<UtilFeeMatch['kind'], string> = { product: 'массу товара', packaging: 'материал и массу упаковки', vehicle: 'категорию транспорта' }

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
  const [materials, setMaterials] = useState<UtilFeeMaterial[]>([])
  // Режим запоминается: таблица хранится в браузере, после обновления страницы открывается она же.
  const [mode, setModeState] = useState<'one' | 'table'>(() => { try { return localStorage.getItem('utilFee.mode') === 'table' ? 'table' : 'one' } catch { return 'one' } })
  const setMode = (next: 'one' | 'table') => { setModeState(next); try { localStorage.setItem('utilFee.mode', next) } catch { /* без сохранения */ } }
  useEffect(() => { utilFeeApi.list().then((list) => { setSections(list.sections); setEdition(list.edition); setMaterials(list.materials) }).catch((reason) => setError(errorMessage(reason))) }, [])

  const set = (change: Partial<UtilFeeInput>) => setInput((value) => ({ ...value, ...change }))
  const setRow = (index: number, change: Partial<UtilFeeInput['packaging'][number]>) =>
    set({ packaging: input.packaging.map((row, rowIndex) => rowIndex === index ? { ...row, ...change } : row) })
  // Новый код — новый товар: введённые массы и категория к нему не относятся.
  const setCode = (code: string) => {
    if (digits(code) === digits(input.code)) return set({ code })
    set({ code, massKg: '', packaging: [{ material: 'paper', massKg: '' }], vehicleCategory: '', fromBelarusRussia: false })
  }

  const productK = result?.matches.find((match) => match.kind === 'product')?.k
  // Сумма в долларах — только для показа, по курсу НБРК на сегодня (как «≈ USD» в смете); считает сервер в тенге.
  const usdRate = useTodayRates()?.find((rate) => rate.currency === 'USD')
  const inUsd = (kzt: string) => {
    const rate = Number(usdRate?.rate)
    const value = Number(kzt)
    return rate > 0 && Number.isFinite(value) ? `≈ ${formatMoney(moneyText(value / rate), 'USD')}` : ''
  }

  const has = (kind: UtilFeeMatch['kind']) => result?.matches.some((match) => match.kind === kind) ?? false
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="utilFee" onLogout={onLogout} />
    <section className="directory-page uf-page">
      <div className="dir-head">
        <div>
          <h1>Утильсбор</h1>
          <p>{mode === 'one'
            ? 'Введите код ТН ВЭД — программа покажет раздел перечня и коэффициент. Затем заполните массу или категорию — сумма посчитается сама.'
            : <>1. В Excel выделите колонку кодов и нажмите Ctrl+C. 2. Щёлкните первую ячейку «Код ТН ВЭД» и нажмите Ctrl+V.
              3. Так же вставьте брутто и, если есть, нетто. Суммы посчитаются сами.</>}</p>
        </div>
        <div className="uf-tabs" role="tablist" aria-label="Как считать">
          <button type="button" role="tab" aria-selected={mode === 'one'} className={mode === 'one' ? 'active' : undefined} onClick={() => setMode('one')}
            title="Подробный расчёт по одному коду: товар, упаковка из нескольких материалов, транспорт">Один товар</button>
          <button type="button" role="tab" aria-selected={mode === 'table'} className={mode === 'table' ? 'active' : undefined} onClick={() => setMode('table')}
            title="Много строк сразу: вставьте коды и вес из Excel поставщика">Таблица из Excel</button>
        </div>
      </div>
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}

      {mode === 'table' && <UtilFeeRows materials={materials} usdRate={usdRate?.rate} />}
      {mode === 'one' && <>

      {/* Одна карточка: сверху поля, под ними перечень. Одно поле на всё: пока в нём меньше 4 цифр или слово —
          перечень фильтруется по нему; 4–10 цифр — перечень скрыт, ниже расчёт. Карточка не пересоздаётся —
          курсор в поле не слетает. */}
      <ListTable sections={sections} query={input.code} onPick={setCode}
        showList={!/^\d{4,}$/.test(digits(input.code)) && sections.length > 0}
        fields={<>
          <label className="uf-code">Код ТН ВЭД<input value={input.code} onChange={(event) => setCode(event.target.value)} placeholder="код (8516500000) или слово (шины)" autoFocus /></label>
          <label className="uf-mrp" title="Месячный расчётный показатель; по умолчанию — на 2026 год">МРП<input value={input.mrp} onChange={(event) => set({ mrp: event.target.value })} inputMode="decimal" /><span>₸</span></label>
        </>} />

      {/^\d{11,}$/.test(digits(input.code)) && <p className="uf-hint">В коде ТН ВЭД не больше 10 цифр.</p>}
      {codeReady && !result && loading && <p className="uf-hint">Ищем код в перечне…</p>}

      {/* Результат — как этап «Результат» сметы: слева ответ словами (платится ли и за что), справа — панель
          расчёта: итог крупно и блоки «за товар / за упаковку / транспорт». */}
      {result && result.code === digits(debounced.code) && <>
        <div className={result.matches.length > 0 ? 'uf-result' : 'uf-result single'}>
          {/* Ответ словами, а не таблицей: платится ли утильсбор и за что, по какому разделу и пункту — полностью,
              без обрезки. «Зависит от полного кода» — отдельной жёлтой плашкой, коды — кнопками. */}
          <section className="uf-answer" aria-label="Код в перечне">
            {result.matches.length > 0
              ? <div className="uf-verdict yes">
                <h2>Код <code>{groupCode(result.code)}</code> — утильсбор платится {[...new Set(result.matches.map((match) => match.kind))].map((kind) => verdictLabels[kind]).join(' и ')}</h2>
                <ul className="uf-why">{result.matches.map((match) => <li key={`${match.section}-${match.item}`}>
                  <span className={`uf-pay ${match.kind}`}>{payLabels[match.kind]}</span>
                  <div>
                    <b>Раздел {match.section}, пункт {match.item}{match.k !== undefined && <> · k = {formatRuNumber(match.k)}</>}</b>
                    <p>{match.title}</p>
                    <small>Позиция перечня: <code>{groupCode(match.prefix)}</code>{match.prefix.length < 10 && ' и все коды, которые с неё начинаются'}</small>
                    {match.note && <p className="uf-note-line">⚠ {match.note}</p>}
                  </div>
                </li>)}</ul>
                <p className="uf-next">Сумма — справа: заполните {[...new Set(result.matches.map((match) => match.kind))].map((kind) => needLabels[kind]).join(' и ')}.</p>
              </div>
              : result.possible.length > 0
                ? <div className="uf-verdict maybe">
                  <h2>Код <code>{groupCode(result.code)}</code> — зависит от полного кода</h2>
                  <p>Утильсбор платится, только если полный код — один из кодов ниже. Выберите его или введите код полностью.</p>
                </div>
                : <div className="uf-verdict no">
                  <h2>Код <code>{groupCode(result.code)}</code> не входит в перечень</h2>
                  <p>Утильсбор за этот товар и его упаковку не платится.</p>
                </div>}

            {result.possible.length > 0 && <div className="uf-possible">
              {result.matches.length > 0 && <h3>Может платиться ещё — зависит от полного кода</h3>}
              {[...new Set(result.possible.map((match) => match.section))].map((sectionNumber) => {
                const items = result.possible.filter((match) => match.section === sectionNumber)
                const k = new Set(items.map((item) => item.k)).size === 1 ? items[0].k : undefined
                return <div className="uf-possible-group" key={sectionNumber}>
                  <p>Если полный код {items.length > 1 ? 'один из этих' : 'такой'} — ещё <b>{payLabels[items[0].kind]}</b>: раздел {sectionNumber}{k !== undefined && <>, k = {formatRuNumber(k)}</>}</p>
                  <small>{items[0].title}</small>
                  <div className="uf-picks">{items.map((match) => <button type="button" key={match.item} onClick={() => setCode(match.prefix)} title="Посчитать по этому коду">
                    <code>{groupCode(match.prefix)}</code><small>п. {match.item}</small>
                  </button>)}</div>
                </div>
              })}
            </div>}
          </section>

          {result.matches.length > 0 && <aside className="uf-calc" aria-label="Расчёт утильсбора">
            <div className="uf-calc-total">
              <span>Итого утильсбор</span>
              <strong>{result.total ? money(result.total) : '—'}</strong>
              {result.total && inUsd(result.total) && <small className="uf-usd" title={usdRate ? `Курс НБРК ${formatRuNumber(usdRate.rate)} ₸ за 1 USD` : undefined}>{inUsd(result.total)}{usdRate && ` · курс ${formatRuNumber(usdRate.rate)} ₸`}</small>}
              {!result.total && <small>Заполните поля ниже — сумма посчитается сама</small>}
            </div>

            {has('product') && <div className="uf-calc-block">
              <div className="uf-calc-head"><b>За товар</b>{result.product && <span className="uf-amount">{money(result.product.amount)}<small>{inUsd(result.product.amount)}</small></span>}</div>
              <p className="uf-formula">масса (т) × МРП × k{productK !== undefined && ` (${formatRuNumber(productK)})`}</p>
              <label className="uf-calc-field">Масса нетто, кг<input value={input.massKg} onChange={(event) => set({ massKg: event.target.value })} inputMode="decimal" placeholder="0" /></label>
            </div>}

            {has('packaging') && <div className="uf-calc-block">
              <div className="uf-calc-head"><b>За упаковку</b></div>
              <p className="uf-formula">масса упаковки (т) × МРП × k материала</p>
              <div className="uf-pack-row uf-pack-head" aria-hidden="true"><span>Материал</span><span>Масса, кг</span><span>Сумма</span></div>
              {input.packaging.map((row, index) => {
                const amount = result.packaging.find((line) => line.index === index)
                return <div className="uf-pack-row" key={index}>
                  <select value={row.material} onChange={(event) => setRow(index, { material: event.target.value })} aria-label="Материал упаковки">
                    {result.materials.map((material) => <option key={material.id} value={material.id}>{material.label} · k {formatRuNumber(material.k)}</option>)}
                  </select>
                  <input value={row.massKg} onChange={(event) => setRow(index, { massKg: event.target.value })} inputMode="decimal" placeholder="0" aria-label="Масса упаковки, кг" />
                  <span className="uf-amount" title={amount ? inUsd(amount.amount) : undefined}>{amount ? money(amount.amount) : ''}</span>
                  {input.packaging.length > 1 && <button type="button" className="uf-remove" aria-label="Удалить строку" onClick={() => set({ packaging: input.packaging.filter((_, rowIndex) => rowIndex !== index) })}>×</button>}
                </div>
              })}
              <button type="button" className="uf-add" onClick={() => set({ packaging: [...input.packaging, { material: 'polymer', massKg: '' }] })}>+ Ещё материал</button>
            </div>}

            {has('vehicle') && <div className="uf-calc-block">
              <div className="uf-calc-head"><b>Транспорт и техника</b>{result.vehicle && <span className="uf-amount">{money(result.vehicle.amount)}<small>{inUsd(result.vehicle.amount)}</small></span>}</div>
              <p className="uf-formula">50 МРП × коэффициент</p>
              {/* Категории — сразу списком (их немного и сервер присылает только подходящие к коду): название
                  слева, коэффициент справа, выбранная — на зелёном. Повторный щелчок снимает выбор. */}
              <div className="uf-calc-field">Категория
                <div className="uf-options" role="radiogroup" aria-label="Категория транспорта">
                  {(result.vehicleCategories ?? []).map((category) => {
                    const chosen = input.vehicleCategory === category.id
                    return <button key={category.id} type="button" role="radio" aria-checked={chosen} className={chosen ? 'active' : undefined}
                      onClick={() => set({ vehicleCategory: chosen ? '' : category.id })}>
                      <span>{category.label}</span>
                      <b>коэф. {formatRuNumber(input.fromBelarusRussia ? category.coef2 : category.coef1)}</b>
                    </button>
                  })}
                </div>
              </div>
              <label className="uf-check"><input type="checkbox" checked={input.fromBelarusRussia} onChange={(event) => set({ fromBelarusRussia: event.target.checked })} />Ввоз из Беларуси или России</label>
            </div>}
          </aside>}
        </div>
      </>}
      </>}

      {/* По каким документам перечень и коэффициенты — одна строка внизу страницы, и при перечне, и при расчёте. */}
      {(edition || result?.edition) && <p className="uf-disclaimer uf-foot">
        <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"><circle cx="8" cy="8" r="6.3" /><path d="M8 7.3v3.6" /><circle cx="8" cy="5" r=".4" fill="currentColor" /></svg>
        {edition || result?.edition}
      </p>}
    </section>
  </main>
}

// Перечень № 689 целиком — одной таблицей, как лист сметы. Разделы свёрнуты: строка раздела (название, как
// платится, k, число позиций) раскрывает его позиции — пункт, код, k, примечание. Поиск — по коду (в обе
// стороны: «8516500000» найдёт позицию «8516») или по словам названия раздела и примечания; найденные
// разделы раскрываются сами.
type ListProps = { sections: UtilFeeSection[]; query: string; fields: ReactNode; showList: boolean; onPick: (code: string) => void }

function ListTable({ sections, query, fields, showList, onPick }: ListProps) {
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
  </>
}
