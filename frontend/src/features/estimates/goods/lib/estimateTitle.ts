// Название сметы для списка и заголовка: старые копии «Копия перчаток · …» показываются
// как «Смета товаров · …», пустое название — «Смета товаров».
export function estimateTitle(value: string) {
  const name = value.trim()
  const suffix = name.replace(/^Копия\s+(?:перчаток|товаров)\s*[·–-]?\s*/i, '')
  return suffix !== name ? `Смета товаров${suffix ? ` · ${suffix}` : ''}` : name || 'Смета товаров'
}
