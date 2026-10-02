-- Описание фото записи базы решений, составленное ИИ (вид товара, материал, форма, цвет,
-- особенности): по нему находится похожий товар на другом снимке. photo_features_at — когда
-- описание составлено или признано невозможным (тогда в photo_features пишется null).
ALTER TABLE code_decisions
    ADD COLUMN photo_features JSONB NOT NULL DEFAULT 'null'::jsonb,
    ADD COLUMN photo_features_at TIMESTAMPTZ;
CREATE INDEX code_decisions_photo_features_pending_idx ON code_decisions (id)
    WHERE photo_features_at IS NULL AND photos <> '[]'::jsonb;

-- Новый вид сообщения чата подбора: похожие товары по фото.
ALTER TABLE tnved_agent_messages DROP CONSTRAINT tnved_agent_messages_kind_check;
ALTER TABLE tnved_agent_messages ADD CONSTRAINT tnved_agent_messages_kind_check
    CHECK (kind IN ('input', 'text', 'tool', 'question', 'user', 'rerun', 'result', 'match', 'similar', 'error', 'notice'));
