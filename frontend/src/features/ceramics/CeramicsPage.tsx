import { useCallback, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { PageHead } from '@/shared/components/PageHead/PageHead'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { User } from '@/shared/types/user'
import { ceramicsApi } from './ceramicsApi'
import { AIStatus, CeramicsType } from './types'
import { MatchTab } from './components/MatchTab'
import { BaseTab } from './components/BaseTab'
import '@/shared/styles/list-page.css'
import './ceramics.css'

// Подбор наименования и цены керамической посуды: марка + китайское название + фото →
// тип товара, размер и цена из нашей базы. Отдельный модуль: со сметами, пошлинами,
// НДС и доставкой не связан.
export function CeramicsPage({ user, tab, onLogout }: { user: User; tab: 'match' | 'base'; onLogout: () => void }) {
  const [types, setTypes] = useState<CeramicsType[]>([])
  const [ai, setAi] = useState<AIStatus | null>(null)

  const reloadTypes = useCallback(() => {
    ceramicsApi.types().then(setTypes).catch(() => { /* ошибку покажет вкладка при своих запросах */ })
  }, [])

  useEffect(() => {
    reloadTypes()
    ceramicsApi.status().then(setAi).catch(() => setAi(null))
  }, [reloadTypes])

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="ceramics" onLogout={onLogout} />
    <section className="directory-page cer-page">
      <PageHead title="Керамика: наименование и цена"
        subtitle="Марка + китайское наименование + фото → тип товара, размер и цена из нашей базы. Таможенных расчётов здесь нет." />
      <nav className="dir-seg cer-tabs" aria-label="Разделы керамики">
        <Link href={paths.ceramics} className={tab === 'match' ? 'active' : undefined} aria-current={tab === 'match' ? 'page' : undefined}>Подбор по файлу</Link>
        <Link href={paths.ceramicsBase} className={tab === 'base' ? 'active' : undefined} aria-current={tab === 'base' ? 'page' : undefined}>База марок{types.length > 0 && <span className="dir-count">{types.reduce((sum, item) => sum + item.productCount, 0)}</span>}</Link>
      </nav>
      {/* Подбор остаётся смонтированным: результаты файла не теряются при переходе в базу. */}
      <div className="cer-tab" hidden={tab !== 'match'}><MatchTab types={types} ai={ai} /></div>
      {tab === 'base' && <div className="cer-tab"><BaseTab user={user} types={types} onBaseChanged={reloadTypes} /></div>}
    </section>
  </main>
}
