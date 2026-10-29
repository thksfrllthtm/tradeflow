-- Что изображено на логотипе ТРОИС («поезд, рельсы», «лев, корона») — для поиска похожих рисунков на фото
-- товара. Заполняет ИИ при чтении логотипа, поэтому уже прочитанные логотипы читаются заново (подписи
-- администратора при этом не меняются).
ALTER TABLE trademarks ADD COLUMN logo_picture VARCHAR(300) NOT NULL DEFAULT '';
UPDATE trademarks SET logo_read_at = NULL WHERE logo IS NOT NULL;
