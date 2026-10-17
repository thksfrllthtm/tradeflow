ALTER TABLE tnved_agent_sessions
    DROP COLUMN vision_prompt_tokens,
    DROP COLUMN vision_cached_tokens,
    DROP COLUMN vision_completion_tokens,
    DROP COLUMN vision_cost_usd;
