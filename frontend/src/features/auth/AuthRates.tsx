import { mainRates, useTodayRates } from '@/shared/api/todayRates'
import { formatRuDate } from '@/shared/lib/dates'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'

// Курс НБРК на сегодня внизу карточки входа и регистрации. Курса нет — блока нет, вход от него не зависит.
export function AuthRates() {
  const all = useTodayRates()
  const rates = all ? mainRates(all) : null
  if (!rates || rates.length === 0) return null
  const date = rates[0].date
  return <section className="auth-rates" aria-label="Курс Национального банка РК" title="Официальный курс Национального банка РК">
    <span className="auth-rates-title">Курс НБРК{date && ` на ${formatRuDate(date)}`}</span>
    <div className="auth-rates-list">
      {rates.map((rate) => <span key={rate.currency} className="auth-rate"><small>1 {rate.currency}</small><b>{formatRuNumber(rate.rate)} ₸</b></span>)}
    </div>
  </section>
}
