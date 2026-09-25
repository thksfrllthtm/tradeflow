-- Фото строк сметы — отдельно от самой сметы. Раньше фото (base64) лежали внутри
-- input_snapshot, и каждое сохранение и даже список смет гоняли их целиком. Теперь смета
-- сохраняется частями, а фото строки записывается, только когда оно изменилось.
CREATE TABLE goods_estimate_photos (
    estimate_id BIGINT NOT NULL REFERENCES gloves_copy_estimates(id) ON DELETE CASCADE,
    item_id VARCHAR(128) NOT NULL,
    -- Массив data URL (JPG/PNG в base64), первое — главное фото строки.
    photos JSONB NOT NULL,
    PRIMARY KEY (estimate_id, item_id)
);
ALTER TABLE goods_estimate_photos ENABLE ROW LEVEL SECURITY;

-- Строкам без id даём id: по нему строка находит свои фото.
UPDATE gloves_copy_estimates e
SET input_snapshot = jsonb_set(e.input_snapshot, '{items}', (
    SELECT jsonb_agg(CASE WHEN COALESCE(t.item->>'id', '') = '' THEN t.item || jsonb_build_object('id', 'row-' || t.ord) ELSE t.item END ORDER BY t.ord)
    FROM jsonb_array_elements(e.input_snapshot->'items') WITH ORDINALITY AS t(item, ord)
))
WHERE jsonb_typeof(e.input_snapshot->'items') = 'array'
  AND jsonb_array_length(e.input_snapshot->'items') > 0
  AND EXISTS (SELECT 1 FROM jsonb_array_elements(e.input_snapshot->'items') AS i(item) WHERE COALESCE(i.item->>'id', '') = '');

-- Переносим фото: photoUrls, а если его нет — photoUrl.
INSERT INTO goods_estimate_photos (estimate_id, item_id, photos)
SELECT e.id, t.item->>'id',
       CASE WHEN jsonb_typeof(t.item->'photoUrls') = 'array' AND jsonb_array_length(t.item->'photoUrls') > 0
            THEN t.item->'photoUrls'
            ELSE jsonb_build_array(t.item->>'photoUrl') END
FROM gloves_copy_estimates e
CROSS JOIN LATERAL jsonb_array_elements(e.input_snapshot->'items') AS t(item)
WHERE jsonb_typeof(e.input_snapshot->'items') = 'array'
  AND ((jsonb_typeof(t.item->'photoUrls') = 'array' AND jsonb_array_length(t.item->'photoUrls') > 0)
       OR COALESCE(t.item->>'photoUrl', '') <> '')
ON CONFLICT (estimate_id, item_id) DO NOTHING;

-- Убираем фото из самой сметы.
UPDATE gloves_copy_estimates e
SET input_snapshot = jsonb_set(e.input_snapshot, '{items}', (
    SELECT jsonb_agg((t.item - 'photoUrls') || jsonb_build_object('photoUrl', '') ORDER BY t.ord)
    FROM jsonb_array_elements(e.input_snapshot->'items') WITH ORDINALITY AS t(item, ord)
))
WHERE jsonb_typeof(e.input_snapshot->'items') = 'array'
  AND jsonb_array_length(e.input_snapshot->'items') > 0;
