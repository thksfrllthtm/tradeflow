// Какие колонки Excel сопоставлены полям строки (значение — заголовок колонки).
export type GoodsMapping = {
  name: string
  // Ещё колонки с наименованием (например, китайское и русское): идут в подбор кода и поиск в базе.
  name2: string
  name3: string
  code: string
  pairs: string
  places: string
  pairsPerPlace: string
  grossKg: string
  netKg: string
  // Цена за единицу из файла (USD); у строк без цены стоимость считается как обычно.
  pricePerPair: string
}
