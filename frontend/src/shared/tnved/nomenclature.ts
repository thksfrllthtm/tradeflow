// Код номенклатуры ТН ВЭД ЕАЭС с пошлиной ЕТТ, НДС, ставкой перечня РК и ссылкой на страницу
// кода — общий для справочников, смет, базы решений и подбора кода ИИ.

// combined_plus — «процент плюс сумма за единицу», такого типа в справочнике нет.
export type NomenclatureDuty = { type: 'unknown' | 'percentage' | 'specific' | 'combined_minimum' | 'combined_plus'; percent: string; amount: string; currency: string; unit: string }
// detailsUrl — страница кода в источнике («Подробнее» в выгрузке TWS), пусто, если ссылки нет.
// Ставка по перечню РК (ниже ЕТТ) и её действие на сегодня; other — форма, которой нет в справочнике.
export type KzRate = { rateText: string; duty: Omit<NomenclatureDuty, 'type'> & { type: 'percentage' | 'specific' | 'combined_minimum' | 'other' }; validFrom: string; validTo: string; active: boolean; status: string; notesText: string }
// Антидемпинговая мера ЕАЭС (реестр ЕЭК) для кода: rates — % для прочих производителей по стране (ISO),
// exact — по точному коду, иначе по товарной позиции (решает описание товара), suspended — не применяется.
export type AntiDumpingRef = { number: string; shortName: string; countries: string[]; countryNames: string; rates: Record<string, string>; exact: boolean; suspended: boolean; endDate: string; documentUrl: string }
export type NomenclatureCode = { code: string; name: string; tariffText: string; duty: NomenclatureDuty; vatPercent: string; detailsUrl: string; kz: KzRate | null; antiDumping?: AntiDumpingRef[]; sir?: { priceUsd: string; profile: string; file?: string } | null }
export type NomenclatureImport = { source: string; fileName: string; actualDate: string; codeCount: number; addedCount: number; removedCount: number; changedCount: number; importedAt: string; importedByName: string }
