-- Возвращаем фото строк внутрь сметы (photoUrls и photoUrl), затем удаляем таблицу.
UPDATE gloves_copy_estimates e
SET input_snapshot = jsonb_set(e.input_snapshot, '{items}', (
    SELECT jsonb_agg(CASE WHEN p.photos IS NULL THEN t.item
                          ELSE t.item || jsonb_build_object('photoUrls', p.photos, 'photoUrl', p.photos->>0) END ORDER BY t.ord)
    FROM jsonb_array_elements(e.input_snapshot->'items') WITH ORDINALITY AS t(item, ord)
    LEFT JOIN goods_estimate_photos p ON p.estimate_id = e.id AND p.item_id = t.item->>'id'
))
WHERE jsonb_typeof(e.input_snapshot->'items') = 'array'
  AND jsonb_array_length(e.input_snapshot->'items') > 0;

DROP TABLE IF EXISTS goods_estimate_photos;
