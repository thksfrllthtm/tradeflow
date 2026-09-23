import { TNVEDDirectoryEntry, TNVEDRate } from './types'

// Подписи для таблицы справочника ТН ВЭД.
export function rateLabel(rate: TNVEDRate, label: string) { if (rate.type === 'percentage' && rate.percent !== '') return `${rate.percent}%`; if (rate.type === 'specific' && rate.amount !== '') return `${rate.amount} ${rate.currency}/${rate.unit}`; if (rate.type === 'combined_minimum' && rate.percent !== '' && rate.amount !== '') return `${rate.percent}%, но не менее ${rate.amount} ${rate.currency}/${rate.unit}`; if (rate.type === 'exempt') return 'Освобождение'; return label === 'НДС' ? 'Не указан' : 'Не указана' }
// Вторая строка под описанием: название по-китайски и первый синоним (пусто, если нет ни того, ни другого).
export function entrySubline(item: TNVEDDirectoryEntry) { const synonym = [...item.synonymsRu, ...item.synonymsZh, ...item.synonymsEn][0]; return [item.nameZh, synonym && `синоним: ${synonym}`].filter(Boolean).join(' · ') }
