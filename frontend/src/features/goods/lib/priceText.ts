// Цена единицы, которую считает сам сайт (подбор под бюджет, «Вычесть у всех»):
// до decimals знаков после точки, без лишних нулей.
export function priceText(value: number, decimals = 4) {
  return value.toFixed(decimals).replace(/\.?0+$/, '') || '0'
}
