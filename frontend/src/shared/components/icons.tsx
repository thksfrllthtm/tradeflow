// Маленькие иконки для таблиц справочников (16×16, цвет — currentColor).
const base = { width: 16, height: 16, viewBox: '0 0 16 16', fill: 'none', stroke: 'currentColor', strokeWidth: 1.5, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const, 'aria-hidden': true }

export const SearchIcon = () => <svg {...base}><circle cx="7" cy="7" r="4.5" /><path d="m10.5 10.5 3 3" /></svg>
export const EditIcon = () => <svg {...base}><path d="M10.5 2.5 13.5 5.5 6 13H3v-3z" /></svg>
export const CopyIcon = () => <svg {...base}><rect x="5.5" y="5.5" width="8" height="8" rx="1.5" /><path d="M10.5 5.5v-2a1 1 0 0 0-1-1h-6a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2" /></svg>
export const ArchiveIcon = () => <svg {...base}><rect x="2" y="3" width="12" height="3" rx="1" /><path d="M3 6v6.5a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1V6M6.5 9h3" /></svg>
export const RestoreIcon = () => <svg {...base}><path d="M3 8a5 5 0 1 0 1.5-3.5M3 2.5v2.5h2.5" /></svg>
export const CloseIcon = () => <svg {...base}><path d="m4 4 8 8M12 4l-8 8" /></svg>
