-- Сравнение ответов по «Документам для ввоза»: сошлись ли основная модель и тень по набору мер «нужно»
-- для кода documents_code. NULL — документы не запрашивали или ответа ещё нет у одной из сторон.
ALTER TABLE tnved_agent_shadow_runs
    ADD COLUMN documents_code VARCHAR(10) NOT NULL DEFAULT '',
    ADD COLUMN documents_agree BOOLEAN;
