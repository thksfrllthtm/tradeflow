import { ReactNode } from 'react'
import '@/shared/styles/list-page.css'

type Props = {
  title: ReactNode
  // Число рядом с заголовком (количество записей).
  count?: number
  // Одна строка пояснения под заголовком.
  subtitle?: ReactNode
  // Ссылка на родительскую страницу над заголовком («← Все документы»).
  back?: ReactNode
  // Кнопки справа: вторичные, главная — последней.
  actions?: ReactNode
}

// Шапка страницы: слева заголовок и пояснение, справа действия. Одинаковая на всех страницах.
export function PageHead({ title, count, subtitle, back, actions }: Props) {
  return <header className="dir-head">
    <div className="dir-head-text">
      {back && <div className="dir-head-back">{back}</div>}
      <h1>{title}{count !== undefined && <span className="dir-count">{count}</span>}</h1>
      {subtitle && <p>{subtitle}</p>}
    </div>
    {actions && <div className="dir-head-actions">{actions}</div>}
  </header>
}
