-- Роли сотрудников: admin — полный доступ и управление сотрудниками; invoicer и
-- translator — калькуляторы и просмотр справочников. disabled_at — заблокированный
-- сотрудник не может войти, его сессии не действуют.
ALTER TABLE users
    ADD COLUMN role VARCHAR(16) NOT NULL DEFAULT 'invoicer'
        CHECK (role IN ('admin', 'invoicer', 'translator')),
    ADD COLUMN disabled_at TIMESTAMPTZ;

-- До появления ролей у каждого сотрудника был полный доступ: сохраняем его.
UPDATE users SET role = 'admin';
