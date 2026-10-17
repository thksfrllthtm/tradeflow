-- Агент ТН ВЭД: расход модели для фото (DeepSeek Flash) отдельно от общего. Общие prompt_tokens,
-- cached_tokens, completion_tokens и cost_usd — сумма обеих моделей; vision_* — доля модели для фото.
ALTER TABLE tnved_agent_sessions
    ADD COLUMN vision_prompt_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN vision_cached_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN vision_completion_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN vision_cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0;
