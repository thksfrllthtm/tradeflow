-- Таблица смет товаров называлась по первой версии калькулятора (сметы перчаток).
-- Новое имя — goods_estimates; вместе с ней переименовываются последовательность, индексы
-- и ограничения со старым именем. Внешние ключи из goods_estimate_photos, code_decisions и
-- tnved_agent_sessions ссылаются на таблицу по OID и следуют за ней сами (в их именах gloves нет).
-- RLS (000020) включён без политик и при переименовании сохраняется.
ALTER TABLE gloves_copy_estimates RENAME TO goods_estimates;
ALTER SEQUENCE gloves_copy_estimates_id_seq RENAME TO goods_estimates_id_seq;
ALTER INDEX gloves_copy_estimates_user_updated_idx RENAME TO goods_estimates_user_updated_idx;
ALTER TABLE goods_estimates RENAME CONSTRAINT gloves_copy_estimates_pkey TO goods_estimates_pkey;
ALTER TABLE goods_estimates RENAME CONSTRAINT gloves_copy_estimates_user_id_fkey TO goods_estimates_user_id_fkey;
