import { TNVEDDirectoryEntry } from '@/features/directories/tnved/types'

// Короткие подписи ставок и статуса карточки ТН ВЭД для окон выбора кода.
export function rateText(rate: TNVEDDirectoryEntry['duty']) {
  if (rate.type === 'percentage') return `${rate.percent}%`
  if (rate.type === 'specific') return `${rate.amount} ${rate.currency}/${rate.unit}`
  if (rate.type === 'combined_minimum') return `${rate.percent}% / не менее ${rate.amount} ${rate.currency}`
  if (rate.type === 'exempt') return 'Освобождение'
  return 'Не указана'
}

export function directoryStatusLabel(entry: TNVEDDirectoryEntry) {
  return entry.verificationStatus === 'operator_verified' ? 'Проверено оператором' : 'Требует проверки'
}
