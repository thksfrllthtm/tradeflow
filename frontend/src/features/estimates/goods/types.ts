// Какие колонки Excel сопоставлены полям строки (значение — заголовок колонки).
export type GlovesMapping = {
  name: string
  code: string
  pairs: string
  places: string
  pairsPerPlace: string
  grossKg: string
  netKg: string
  // Цена за единицу из файла (USD); у строк без цены стоимость считается как обычно.
  pricePerPair: string
}
