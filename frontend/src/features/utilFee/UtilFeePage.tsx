import { useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { User } from '@/shared/types/user'
import { UtilFeeInput, UtilFeeMatch, UtilFeeResult, utilFeeApi } from './utilFeeApi'
import '@/shared/styles/list-page.css'
import './utilFee.css'

// МРП на 2026 год — значение по умолчанию; сотрудник может поменять.
const DEFAULT_MRP = '4325'

const kindLabels: Record<UtilFeeMatch['kind'], string> = {
  product: 'платится за товар',
  packaging: 'платится за упаковку',
  vehicle: 'транспорт и техника',
}

const money = (value: string) => `${formatRuNumber(value)} ₸`
const digits = (code: string) => code.replace(/\s/g, '')

// Калькулятор утильсбора: код ТН ВЭД → позиции перечня № 689 и коэффициенты, затем сотрудник вводит массу
// товара, упаковки или категорию транспорта — сервер считает суммы по Методике № 448.
export function UtilFeePage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [input, setInput] = useState<UtilFeeInput>({ code: '', mrp: DEFAULT_MRP, massKg: '', packaging: [{ material: 'paper', massKg: '' }], vehicleCategory: '', fromBelarusRussia: false })
  const [result, setResult] = useState<UtilFeeResult | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const debounced = useDebouncedValue(input, 350)
  const codeReady = /^\d{10}$/.test(digits(debounced.code))

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

      <div className="uf-card uf-inputs">
        <label>Код ТН ВЭД<input value={input.code} onChange={(event) => setCode(event.target.value)} placeholder="10 цифр, например 8516500000" inputMode="numeric" autoFocus /></label>
        <label>МРП, ₸<input value={input.mrp} onChange={(event) => set({ mrp: event.target.value })} inputMode="decimal" /></label>
      </div>

      {!codeReady && digits(input.code) !== '' && <p className="uf-hint">Код должен состоять из 10 цифр.</p>}
      {codeReady && !result && loading && <p className="uf-hint">Ищем код в перечне…</p>}

      {result && result.code === digits(debounced.code) && <>
        {result.matches.length === 0
          ? <div className="uf-card uf-empty">Код <code>{result.code}</code> не входит в перечень — утильсбор не платится.</div>
          : <div className="uf-card">
            <h2>Код {result.code} в перечне</h2>
            <ul className="uf-matches">
              {result.matches.map((match) => <li key={match.section}>
                <strong>Раздел {match.section}, п. {match.item}</strong> <span className={`uf-kind uf-kind-${match.kind}`}>{kindLabels[match.kind]}</span>
                {match.k !== undefined && <span className="uf-k">k = {formatRuNumber(match.k)}</span>}
                <div className="uf-title">{match.title} · позиция <code>{match.prefix}</code></div>
                {match.note && <div className="uf-note">⚠ {match.note}</div>}
              </li>)}
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
