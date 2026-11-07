# Сбор актов для локальной базы «Документов для ввоза»

Дата сбора: 07.11.2026. Реестр: [acts_registry.json](acts_registry.json). Программа: `backend/cmd/legal-acts-collect` (тексты — в `data/acts/<код>/<дата редакции>.html`, не в git; в проде будут в Postgres).

## Итог

- Записей в реестре: 97; строк отчёта (основные коды и extra_codes): 115.
- ok: 104; needs_review: 4; not_found (нет кода — перечня не существует или он не найден): 7.
- Источник всех текстов — API adilet (`https://adilet.zan.kz/api/documents/by-ngr/<код>`), сбоев API нет; zakon.uchet.kz не понадобился. Объём — около 37 МБ (Решение КТС № 299 — 6 МБ).
- Дата редакции — у текущей версии в истории adilet (`/versions`, `is_current`), пересчитана из UTC в дату по Астане (adilet хранит полночь UTC+6 как 18:00 UTC предыдущего дня).
- 31 код с `code_verified=false` проверен: все существуют, номер и дата совпали с реестром (tr-031: в реестре была неверная дата, исправлено).

## Не найдено

- **tr-012** — ТР ТС 012/2011 оборудование для взрывоопасных сред — таможенный перечень не найден. Таможенный перечень не найден: перебор H12EK000001–H26EK000200 через API adilet и страница ТР на eec.eaeunion.org (07.11.2026).
- **tr-021** — ТР ТС 021/2011 пищевая продукция — таможенный перечень не найден (есть проект 2016 г.). Таможенный перечень не найден (проект 2016 г. не принят): перебор решений Коллегии через API adilet и страница ТР на eec.eaeunion.org (07.11.2026).
- **tr-026** — ТР ТС 026/2012 маломерные суда — таможенный перечень не найден. Таможенный перечень не найден; H13EK000293 — изменение Единого перечня, не таможенный перечень (07.11.2026).
- **tr-049** — ТР ЕАЭС 049/2020 магистральные трубопроводы — таможенный перечень не найден. Таможенный перечень не найден: перебор решений Коллегии через API adilet и страница ТР на eec.eaeunion.org (07.11.2026).
- **tr-047** — ТР ЕАЭС 047/2018 алкогольная продукция — вступает 01.01.2027, перечня пока нет. Перечня нет. Регламент вступает 01.01.2027 — Решение Совета № 62 от 20.05.2026 (H26EV000062); сводная редакция H18EV000098 на adilet ещё показывает 01.07.2026.
- **tr-053** — ТР ЕАЭС 053/2026 лакокрасочные материалы — вступает 01.01.2028. Перечня нет. ТР ЕАЭС 053/2026 «О безопасности лакокрасочных материалов», Решение Совета № 65 от 20.05.2026 (H26EV000065); для РК — с 01.01.2028.
- **tr-041** — ТР ЕАЭС 041/2017 химическая продукция — дата не установлена. Перечня нет. Дата не установлена: «с 2 июня 2021 г. при условии исполнения пункта 2» (H17EV000019); условие частично выполнено Решением Совета № 61 от 01.08.2025.

## Требует решения владельца

- **tr-051** (H24EK000146) — Утратило силу — переведено в watch, в поиск не включать
- **kz-ban-wheat-273** (V2600039292) — Статус «err» в adilet (zakon.uchet.kz — «Действующий»); нет конечной даты в тексте («на шесть месяцев» с 27.07.2026) — valid_to не заполнен
- **sec-poisons** (Z070000302_) — Закон «О безопасности химической продукции», а не о ядовитых веществах; правила для ядов не подтверждены
- **sec-culture** (V2000020601) — Приказ № 111 — правила свидетельства на ВЫВОЗ культурных ценностей: к ввозу не относится
- **tr-047** — сводная редакция Решения Совета № 98 (H18EV000098) на adilet ещё показывает вступление 01.07.2026; перенос на 01.01.2027 — отдельным Решением № 62 от 20.05.2026 (H26EV000062). При сравнении редакций учитывать, что adilet бывает не обновлён.
- **kz-marking-199** — дата введения перечня: 60 дней после публикации 02.06.2026 (≈ 02.08), новости пишут 03.08.2026 — valid_from не заполнен.
- **kz-marking-215 / 253** — изменяющие приказы; 253-ОД меняет приложение 215-НҚ. Сводная редакция 199-НҚ (09.09.2026) дословно совпадает с приложением 253-ОД, поэтому в строки разбирается только 199-НҚ (215 и 253 — `kind: law`).
- **kz-control-list-424** — с 01.09.2026 список изложен заново приказом МПС № 435 (G26IS000435); сводная редакция 424 уже его включает.
- **ТР ЕАЭС 048/2019** — перечень № 23 (H22EK000023) хранится с `valid_from: 2028-09-01` (Решение Совета № 114 в ред. 2025 г.: «не ранее 1 сентября 2028 г.»).
- **ТР ЕАЭС 039/2016** — перечень № 133 (H18EK000133) есть, но вступает вместе с регламентом, а регламент — не раньше ТР ЕАЭС 041/2017, у которого дата не установлена. Оставлен в `watch`.

### Об источниках

- Отсутствие таможенных перечней к ТР 012, 021, 026, 049, 047, 053, 041 проверено перебором кодов решений Коллегии (H12EK…–H26EK…) через API adilet и страницами регламентов на eec.eaeunion.org. Adilet хранит не все решения ЕЭК, поэтому это сильный, но не абсолютный довод.
- Приказы министерств РК 2026 г. без регистрации в Минюсте имеют коды нового вида `G26<орган><номер>` (МТИ — `TI`, МПС — `IS`).
- API adilet неофициальное. Если оно перестанет отвечать или сменит формат, программа берёт текст с zakon.uchet.kz и пишет причину в `api_problem` — это будет предупреждением администратору на этапе 5.

## Все коды

| id | код | роль | статус | редакция | статус adilet | sha256 | примечание |
|---|---|---|---|---|---|---|---|
| eaeu-treaty | [Z1400000240](https://adilet.zan.kz/rus/docs/Z1400000240) | осн. | ok | 2025-02-10 | upd | `e94e1966e576dbbc03c52626b806b2e56bb6a720fa679012debcafd1dccef565` | сверено вручную: Закон о ратификации Договора о ЕАЭС; текст договора (с приложением № 9) входит в документ |
| tk-eaeu | [Z1700000115](https://adilet.zan.kz/rus/docs/Z1700000115) | осн. | ok | 2025-10-04 | upd | `f1108a0cb33a47ee0444c63bbbb70974953a9d788efc2a7253811ccf4afe4bb1` | сверено вручную: Закон о ратификации Договора о ТК ЕАЭС; текст договора входит в документ |
| kz-customs-code | [K1700000123](https://adilet.zan.kz/rus/docs/K1700000123) | осн. | ok | 2026-07-12 | upd | `1f170c49be916910b6190352185733b6dcdcd51bb91daa3b03f1524fa7342a30` |  |
| eec-30 | [H15EK000030](https://adilet.zan.kz/rus/docs/H15EK000030) | осн. | ok | 2026-05-10 | upd | `b9c33818a55b2500f0a729fc50c8ca47734c98950ae72a6f87e7c54dba48c4fc` |  |
| eec-council-125 | [H23EV000125](https://adilet.zan.kz/rus/docs/H23EV000125) | осн. | ok | 2024-11-29 | upd | `2e9748e635eb189fcca2c90b56cab6736c78f2a0cddf8857786e4e1d407afc04` |  |
| kz-pp-287 | [P1500000287](https://adilet.zan.kz/rus/docs/P1500000287) | осн. | ok | 2024-02-05 | upd | `97d0bf4e3edea54aebbe6dba02efc49337ef56d0501b88f9ac36439ad71716a5` |  |
| kz-trade-law | [Z040000544_](https://adilet.zan.kz/rus/docs/Z040000544_) | осн. | ok | 2026-08-25 | upd | `1851d1b10d8a18b37860669860450c2ef2cb4a09e41bad3d4bf051a44277ba6f` |  |
| kz-permits-law | [Z1400000202](https://adilet.zan.kz/rus/docs/Z1400000202) | осн. | ok | 2026-10-21 | upd | `89442acb296a1d427ff66d5e8087272634c43c02d72628ea803d1d2bde750538` |  |
| kz-pp-1083 | [P1500001083](https://adilet.zan.kz/rus/docs/P1500001083) | осн. | ok | 2023-07-28 | upd | `62893ecdf936ac08ca6e003bd66135bf818ef32866e801052bbcd89936ce2725` |  |
| eec-council-130 | [H21EV000130](https://adilet.zan.kz/rus/docs/H21EV000130) | осн. | ok | 2026-02-11 | upd | `1cf5c4bbdd6ba1bd76e4edac9dee939f83589837f225a7c30d7e2c10a8af55f4` |  |
| kts-526 | [H11T0000526](https://adilet.zan.kz/rus/docs/H11T0000526) | осн. | ok | 2024-06-13 | upd | `53db5062707698f24353d3c7f553e81726c508ea7a545ea842b1ea8990e9fa63` |  |
| kz-techreg-law | [Z2000000396](https://adilet.zan.kz/rus/docs/Z2000000396) | осн. | ok | 2026-07-12 | upd | `e6f397cd3bed41d4ca3d9df29166a9d46de707cb61bbc34e595ddaa0223d22d2` |  |
| tr-001 | [H24EK000046](https://adilet.zan.kz/rus/docs/H24EK000046) | осн. | ok | 2024-05-06 | new | `1fa98726ffd966465d401b8682fb1aab054024716a686d30cc44f9613029d1b2` |  |
| tr-002 | [H24EK000048](https://adilet.zan.kz/rus/docs/H24EK000048) | осн. | ok | 2024-05-06 | new | `bc650ce29c0c55ca26df4863428edebf7fc673afb73f6407a56e1a76259e5fa8` |  |
| tr-003 | [H24EK000047](https://adilet.zan.kz/rus/docs/H24EK000047) | осн. | ok | 2024-05-06 | new | `e2e67e7963b8b8f762fc096e35950d66c4e11d2497ce3e0ecc91deb75128adea` |  |
| tr-004 | [H13EK000091](https://adilet.zan.kz/rus/docs/H13EK000091) | осн. | ok | 2023-04-16 | upd | `3d9112d2ae8e0240f268a4ce29ae4684fcfa4d6b7abe706e43a52c1f955831d9` |  |
| tr-005 | [H13EK000047](https://adilet.zan.kz/rus/docs/H13EK000047) | осн. | ok | 2014-03-06 | upd | `17bee38082928cc3bfc69b71dbdc1418608cf5a54f6b14e3f4e4e6541936b5b3` |  |
| tr-006 | [H12EK000030](https://adilet.zan.kz/rus/docs/H12EK000030) | осн. | ok | 2012-04-19 | new | `21928872d26174ab4905cd33ddfbabc57adebd93d6597ef94bae1fe3c3fdcd73` |  |
| tr-007 | [H13EK000028](https://adilet.zan.kz/rus/docs/H13EK000028) | осн. | ok | 2020-01-24 | upd | `540e852a3cc3f2c67bfda4dcca1ed14f97f437d5f5da0e137044f1a106246cb1` |  |
| tr-008 | [H25EK000076](https://adilet.zan.kz/rus/docs/H25EK000076) | осн. | ok | 2025-08-26 | new | `dd397ad67fd942fdb32941d579d11e5dc21291e2cccb9f9a5b6d029ef0b1e54b` |  |
| tr-009 | [H13EK000012](https://adilet.zan.kz/rus/docs/H13EK000012) | осн. | ok | 2014-08-25 | upd | `56a5c3b7d5e36230d39fee7ec9c8404c614609c259a0c2cf02c56a1ec5a8ec79` |  |
| tr-010 | [H18EK000006](https://adilet.zan.kz/rus/docs/H18EK000006) | осн. | ok | 2022-01-01 | upd | `ace3b352204c9233d2748ad5a60b3f8519e861d3a1d94fee1c8d4a17407c14ed` |  |
| tr-011 | [H13EK000044](https://adilet.zan.kz/rus/docs/H13EK000044) | осн. | ok | 2013-03-19 | new | `cffd7dc7d250aab758ed9cd037fb257ba2b33e022d56f325ed702599a0694f2c` |  |
| tr-013 | [H12EK000298](https://adilet.zan.kz/rus/docs/H12EK000298) | осн. | ok | 2014-06-10 | upd | `a8060a59cd85eb2bfc3b6e46be6f2f13ab05adf11344d6942d322220a10b6db4` |  |
| tr-015 | [H14EK000020](https://adilet.zan.kz/rus/docs/H14EK000020) | осн. | ok | 2014-02-11 | new | `215d55468d43855f922e653352bff414696200387ef73c7e493379efeba19b99` |  |
| tr-016 | [H13EK000092](https://adilet.zan.kz/rus/docs/H13EK000092) | осн. | ok | 2017-06-18 | upd | `1a64e8556b131c848dad777ccd587501c9f10b77cb38653f49f0291db3e25cf2` |  |
| tr-017 | [H13EK000228](https://adilet.zan.kz/rus/docs/H13EK000228) | осн. | ok | 2022-11-06 | upd | `12e1a7eb0b3a2fd63fc4fc9254f90152d75f41250025f3869d4dfb21f5bd213d` |  |
| tr-018 | [H15EK000077](https://adilet.zan.kz/rus/docs/H15EK000077) | осн. | ok | 2022-01-29 | upd | `cdbc2cef3eb0aac281ffdc20023cfbaae8a98eff3d1d8f237e969a291a57c86a` |  |
| tr-019 | [H12EK000079](https://adilet.zan.kz/rus/docs/H12EK000079) | осн. | ok | 2022-01-22 | upd | `197836915189ebf40449686a56a44b25ea7ba8aa5fd44d81fec2e83a127cb0c2` |  |
| tr-020 | [H14EK000002](https://adilet.zan.kz/rus/docs/H14EK000002) | осн. | ok | 2023-04-16 | upd | `0f67854f7ef7923e374f3b912047c99a66755d49d5eed42ddbca138d7d946d28` |  |
| tr-023 | [H14EK000076](https://adilet.zan.kz/rus/docs/H14EK000076) | осн. | ok | 2022-01-01 | upd | `708b92a1a2510a4f9f828d81e3ae3144ee865811840828f3398c3bd062c1d22d` |  |
| tr-024 | [H24EK000039](https://adilet.zan.kz/rus/docs/H24EK000039) | осн. | ok | 2024-04-16 | new | `b4607c6d6f4fda755be8064ab72bcf459bda4b18ec27c50633844940cf814e65` |  |
| tr-025 | [H14EK000044](https://adilet.zan.kz/rus/docs/H14EK000044) | осн. | ok | 2022-01-29 | upd | `208747074785acb6a813eed1ae286763179a606676d6788ce97875cd0059055c` |  |
| tr-027 | [H18EK000166](https://adilet.zan.kz/rus/docs/H18EK000166) | осн. | ok | 2022-01-01 | upd | `d6426dba8eee0630b681067478f3adec5915b018d259d791157bd08a11ed2b1e` |  |
| tr-028 | [H14EK000070](https://adilet.zan.kz/rus/docs/H14EK000070) | осн. | ok | 2022-01-22 | upd | `e5bd9acda93d864c0b9ef7abab1697e24989e204e6aecba42c00e82d503fccfe` |  |
| tr-029 | [H19EK000172](https://adilet.zan.kz/rus/docs/H19EK000172) | осн. | ok | 2022-01-01 | upd | `573be9d8abcc0fd5d79893b3d0142d073de5ea66568a688eb733add419b94737` |  |
| tr-030 | [H14EK000037](https://adilet.zan.kz/rus/docs/H14EK000037) | осн. | ok | 2014-03-06 | new | `f24af6c4aef430654f944e7389c144e4030e910a875314dd4a6e845edcbd46be` |  |
| tr-031 | [H23EK000136](https://adilet.zan.kz/rus/docs/H23EK000136) | осн. | ok | 2023-09-05 | new | `c01ac3beeedcd59a436c3688a4e5b2344dbd94b58d62eef041fccbb64b919cad` |  |
| tr-032 | [H19EK000012](https://adilet.zan.kz/rus/docs/H19EK000012) | осн. | ok | 2022-04-03 | upd | `72361d721493dac34f04ce2cb171da713ca45fbe9c23cc97e0b008c719f8de7c` |  |
| tr-033 | [H23EK000170](https://adilet.zan.kz/rus/docs/H23EK000170) | осн. | ok | 2023-12-05 | new | `2670d471c31bdbca8038faffd74dbad8059b9bbb567f69f8b4820a8db219584c` |  |
| tr-034 | [H22EK000140](https://adilet.zan.kz/rus/docs/H22EK000140) | осн. | ok | 2022-10-04 | new | `ef67ab0a9329d5a96f0bf6aa4d5c85ac23fd17128b4c8934a51f742aa8866e8f` |  |
| tr-035 | [H16EK000066](https://adilet.zan.kz/rus/docs/H16EK000066) | осн. | ok | 2016-06-07 | new | `a3cdf81099f76b8329d07fdf31d751a454101652ef133fea0f5361b0e41032ed` |  |
| tr-036 | [H22EK000091](https://adilet.zan.kz/rus/docs/H22EK000091) | осн. | ok | 2022-06-14 | new | `d3ee60516fa4283d897876d5be4596a2d3799081d407e6868db6ed7ab8361a60` |  |
| tr-037 | [H18EK000167](https://adilet.zan.kz/rus/docs/H18EK000167) | осн. | ok | 2022-01-23 | upd | `ccaf182f77b54c8136a6980d92e9d4b30fb75d5bd3cd3589194b483c60ce550e` |  |
| tr-038 | [H19EK000003](https://adilet.zan.kz/rus/docs/H19EK000003) | осн. | ok | 2022-01-29 | upd | `48e8ff45c05da24af665c46d9cda3bb6b4e2e566cd5796e91ab842bc8fe01b14` |  |
| tr-040 | [H23EK000058](https://adilet.zan.kz/rus/docs/H23EK000058) | осн. | ok | 2023-05-11 | new | `6a511ae95812c5559c407d4dd6fbae994ff92cc4432018faf6de4723d16083ca` |  |
| tr-042 | [H18EK000091](https://adilet.zan.kz/rus/docs/H18EK000091) | осн. | ok | 2018-05-29 | err | `2462ec9b6616b4a5346f475f2a9a2f08ee3d1ef53d2d28476cffb8883aba75d4` | сверено вручную: Статус «err» — ошибка метаданных adilet; текст полный, редакция одна, действует с 01.07.2018 |
| tr-043 | [H19EK000170](https://adilet.zan.kz/rus/docs/H19EK000170) | осн. | ok | 2026-04-18 | upd | `cc7e685e09ff2f4f1ffaf280cceb3d05cd192c3db5628b2373e596973a208b9e` |  |
| tr-044 | [H18EK000075](https://adilet.zan.kz/rus/docs/H18EK000075) | осн. | ok | 2018-05-10 | new | `a34927b265acff2022ef6fee89ca7ba5fe7000b09a1239895a376b7c275ab961` |  |
| tr-046 | [H22EK000006](https://adilet.zan.kz/rus/docs/H22EK000006) | осн. | ok | 2022-01-11 | new | `e2c6c8b5f49e3d2ae79f5f5bbff5f1550c64696929448f196348a4eca9c8c001` |  |
| tr-050 | [H24EK000029](https://adilet.zan.kz/rus/docs/H24EK000029) | осн. | ok | 2024-03-26 | new | `2571b6d46190179ade6ff48543fc909b663136206632b86031e6320126366bb4` |  |
| tr-051 | [H24EK000146](https://adilet.zan.kz/rus/docs/H24EK000146) | осн. | needs_review | 2025-12-16 | yts | `119545e012ee5a864197bedba8c95ef550a84fa8901ae89bf12263a5a682ad1f` | Утратило силу — переведено в watch, в поиск не включать |
| tr-052 | [H24EK000045](https://adilet.zan.kz/rus/docs/H24EK000045) | осн. | ok | 2024-05-06 | new | `54266da8d54aa8679024860e9af0646dc0b51f6869974c2bda6247ad28fffd7a` |  |
| tr-048 | [H22EK000023](https://adilet.zan.kz/rus/docs/H22EK000023) | осн. | ok | 2022-02-08 | new | `6c3fa2ef88073fab0e5efabf5c8f1df5cec6051b39ee99904b1e2102658d6aa8` |  |
| tr-039 | [H18EK000133](https://adilet.zan.kz/rus/docs/H18EK000133) | осн. | ok | 2018-08-21 | new | `82711ebf7160c1f073b30a89554d458d83012997ca2754f87bea7c8a70b36f84` | сверено вручную: Перечень № 133 от 21.08.2018 к ТР ЕАЭС 039/2016 (подтверждено агентом и API) |
| kts-299 | [H10T0000299](https://adilet.zan.kz/rus/docs/H10T0000299) | осн. | ok | 2026-01-18 | upd | `4fdd6a7b2aacba9f91d7a1bacd704251a4a8309f90c06f39a88a731d446f70ef` |  |
| kts-317 | [H10T0000317](https://adilet.zan.kz/rus/docs/H10T0000317) | осн. | ok | 2025-07-22 | upd | `3d76ee8e83bdea855d0e8824cdbb04f947d0634fd0d92e5eb85d5485d4e9a4cd` |  |
| kts-318 | [H10T0000318](https://adilet.zan.kz/rus/docs/H10T0000318) | осн. | ok | 2026-01-30 | upd | `2cf129cfc28c4d93a6f08253529803f26463e832075b9092c49035ecc2266836` |  |
| eec-council-157 | [H16EV000157](https://adilet.zan.kz/rus/docs/H16EV000157) | осн. | ok | 2025-12-07 | upd | `a22b32fa68c36ba586b71e0280a46bc0cd283ab5bc41c9561cfba0ab489bd976` |  |
| kz-vet-law | [Z020000339_](https://adilet.zan.kz/rus/docs/Z020000339_) | осн. | ok | 2026-07-20 | upd | `a96b02888cf794b692a51820deaec65103b76124fad951cc3e891121e5827fcd` |  |
| kz-plant-quarantine-law | [Z990000344_](https://adilet.zan.kz/rus/docs/Z990000344_) | осн. | ok | 2026-09-08 | upd | `bd0acf125480071b33860d68d7be4128b14ad69b3669e72210acdda1c8697f36` |  |
| kz-health-code | [K2000000360](https://adilet.zan.kz/rus/docs/K2000000360) | осн. | ok | 2026-09-06 | upd | `3550d47dfb01e934c27b973ac740c1ecffe4bc107e441d2cf91ee63c46d1d435` |  |
| kz-vet-permit-rules | [V14F0010254](https://adilet.zan.kz/rus/docs/V14F0010254) | осн. | ok | 2026-05-29 | upd | `6c7b9260e9a00dbfa7fee4dd2f99abeca6c61813eefa994fe3397c9aa13c2176` |  |
| kz-vet-docs-rules | [V1500011898](https://adilet.zan.kz/rus/docs/V1500011898) | осн. | ok | 2026-07-12 | upd | `5c2583c07b66583469bf383e115331f40c715550e06b7a13d6045d5d2fb7b769` |  |
| kz-phyto-rules | [V1500012032](https://adilet.zan.kz/rus/docs/V1500012032) | осн. | ok | 2026-09-08 | upd | `0feefe826d17d503564cd0ce7eb3f3680343e4a4049c8f62139d7f6f7a70188e` |  |
| kz-sanepid-rules | [V2000022004](https://adilet.zan.kz/rus/docs/V2000022004) | осн. | ok | 2026-07-12 | upd | `90238ce8dda311e0a1819828f794eb2c3037344e3814c8b0449221f073412cb7` |  |
| kz-specific-goods-law | [Z2200000172](https://adilet.zan.kz/rus/docs/Z2200000172) | осн. | ok | 2026-07-12 | upd | `82358e689ff7668d2aa30a984998f163cbfda1489b92c0cda368e4015a7be8bf` |  |
| kz-control-list-424 | [V2300032767](https://adilet.zan.kz/rus/docs/V2300032767) | осн. | ok | 2026-09-01 | upd | `d81ad2a1f4cc9bd9e765057c605631e05c7a22ee4943d9f6f95e1ab6a6e9bd59` |  |
| kz-specific-goods-licensing | [V2300032759](https://adilet.zan.kz/rus/docs/V2300032759) | осн. | ok | 2026-09-21 | upd | `9209e09d3d154d63e32f86c7928989aa66586e97f00eef105171b220c1bdad6a` |  |
| kz-marking-199 | [G26TI000199](https://adilet.zan.kz/rus/docs/G26TI000199) | осн. | ok | 2026-09-09 | upd | `9dd45bf830c1dd59863969cb4154947a48a238b387efdcc5ea8ec44ffae69b25` |  |
| kz-marking-215 | [G26TI000215](https://adilet.zan.kz/rus/docs/G26TI000215) | осн. | ok | 2026-09-04 | upd | `da8172ded8922e9333862be81dbc8bcfa646dcd163554d45fa53769877dce2c2` |  |
| kz-marking-253 | [G26TI000253](https://adilet.zan.kz/rus/docs/G26TI000253) | осн. | ok | 2026-09-04 | new | `f3a08c30d340a929aedffc36b0481f3d067b026c542b2a62f3fb8410cf44b135` |  |
| kz-ban-wheat-273 | [V2600039292](https://adilet.zan.kz/rus/docs/V2600039292) | осн. | needs_review | 2026-07-11 | err | `ddb46c502aef1bc31e66380203b4c6e5bb5f216fa495ddbe8ca0ed4a15017202` | Статус «err» в adilet (zakon.uchet.kz — «Действующий»); нет конечной даты в тексте («на шесть месяцев» с 27.07.2026) — valid_to не заполнен |
| kz-ban-apples-326 | [V2600039803](https://adilet.zan.kz/rus/docs/V2600039803) | осн. | ok | 2026-09-01 | new | `bf0ce1713a9d1c6896845410af5a36065fcd9c813bb898fe8662e0416c4f01a6` |  |
| sec-ozone | [V2000020823](https://adilet.zan.kz/rus/docs/V2000020823) | осн. | ok | 2026-07-12 | upd | `c7abcde1d4b2d66bedbc7168c128293ba8c0ea982c3bb7904da4a27b7a8785b8` |  |
| sec-ozone | [V2100023580](https://adilet.zan.kz/rus/docs/V2100023580) | доп. | ok | 2026-07-12 | upd | `e95537bc1f33945d2b4b947bc86f23b7c4729af88ba59b6ae9abb71df2d7b87e` |  |
| sec-ozone | [K2100000400](https://adilet.zan.kz/rus/docs/K2100000400) | доп. | ok | 2026-09-08 | upd | `2352212bf87838180b7f3e28d3975edb5b89c0c6b58317f8ce93a4871e9bfc5a` | сверено вручную: Экологический кодекс РК |
| sec-pesticides | [V2000021494](https://adilet.zan.kz/rus/docs/V2000021494) | осн. | ok | 2026-07-12 | upd | `93376779fda9bd1278402cc6305707c431edd4d7e49cb543f0ae0244efbedd3c` |  |
| sec-pesticides | [V2000021404](https://adilet.zan.kz/rus/docs/V2000021404) | доп. | ok | 2026-07-12 | upd | `2b9b6bf446afd5ef446b270fc14f57787a812865037c07bdb7f58fdc6da730ee` |  |
| sec-pesticides | [Z020000331_](https://adilet.zan.kz/rus/docs/Z020000331_) | доп. | ok | 2026-09-08 | upd | `225e0ae1e63fc17762bbf93d0459f33e46407202b17a2d80ef3b0e3776e97f57` | сверено вручную: Закон «О защите растений» |
| sec-waste | [V2600038742](https://adilet.zan.kz/rus/docs/V2600038742) | осн. | ok | 2026-05-15 | new | `6d79c28cc89895005ff1bd32d731d990d39d05cc79c3489cd3826b19da175e03` |  |
| sec-wildlife | [V2000021082](https://adilet.zan.kz/rus/docs/V2000021082) | осн. | ok | 2026-07-12 | upd | `3f5f7142746904ee2e58bd70681370055731ac3c85d5365e30df05299ba03f3f` |  |
| sec-wildlife | [Z040000593_](https://adilet.zan.kz/rus/docs/Z040000593_) | доп. | ok | 2026-09-08 | upd | `738da905792054f94d3201ac791233399f6967fd28b3dc702d18650fc40c8b1d` | сверено вручную: Закон «Об охране, воспроизводстве и использовании животного мира» |
| sec-wildlife | [Z2300000183](https://adilet.zan.kz/rus/docs/Z2300000183) | доп. | ok | 2026-07-12 | upd | `2026592c85912b0f244b02b3def2bdb805b357f6458daf7ca21e9c8df9cf2dbc` | сверено вручную: Закон «О растительном мире» |
| sec-cites | [V1500011935](https://adilet.zan.kz/rus/docs/V1500011935) | осн. | ok | 2026-07-12 | upd | `8307117516d7f908a33bd744ad6631ff136e1fb22323a422737b680e02edb1e0` |  |
| sec-gems | [V2400035394](https://adilet.zan.kz/rus/docs/V2400035394) | осн. | ok | 2024-11-19 | new | `988d98d8136372965b67da8b3ac7a23605eb2ea9f16c02896f481a73c9de3964` |  |
| sec-gems | [Z1600000444](https://adilet.zan.kz/rus/docs/Z1600000444) | доп. | ok | 2026-07-12 | upd | `6483740f84ac66f7be0a24e3e646d8f8c32cb818d1a07a954615a3fa366bf051` | сверено вручную: Закон «О драгоценных металлах и драгоценных камнях» |
| sec-precious-metals | [V2000020611](https://adilet.zan.kz/rus/docs/V2000020611) | осн. | ok | 2026-09-17 | upd | `c57aa7cb2460a511deccefa7b5c770ac89ffc726a2357a2563e937318537d639` |  |
| sec-precious-metals | [V2000020907](https://adilet.zan.kz/rus/docs/V2000020907) | доп. | ok | 2026-09-10 | upd | `ec4ae607ad17339d8882460253a8df45baaafd07181a34858d96abddc5187a31` |  |
| sec-narcotics | [V2000020231](https://adilet.zan.kz/rus/docs/V2000020231) | осн. | ok | 2026-09-11 | upd | `30258212d31786a0def88bb652b007fb459b09c015f8d7c5cca224fba14065f3` |  |
| sec-narcotics | [P1400001315](https://adilet.zan.kz/rus/docs/P1400001315) | доп. | ok | 2019-07-05 | upd | `a8d1383844cd3f5789924256f2e8a19a0e63b3e8f977ac5748e77508444bb7d5` |  |
| sec-narcotics | [Z980000279_](https://adilet.zan.kz/rus/docs/Z980000279_) | доп. | ok | 2026-07-01 | upd | `15dad27aa2fd91e18aca484c780e625c204eb3f10b6bdbb108824a75c17eccb3` | сверено вручную: Закон «О наркотических средствах, психотропных веществах…» |
| sec-poisons | [Z070000302_](https://adilet.zan.kz/rus/docs/Z070000302_) | осн. | needs_review | 2024-06-08 | upd | `5486ded904a5cb877d1a77212d5c2ac7f2cef1146097f99f3466cc5cc27f9303` | Закон «О безопасности химической продукции», а не о ядовитых веществах; правила для ядов не подтверждены |
| sec-medicines | [V2000021749](https://adilet.zan.kz/rus/docs/V2000021749) | осн. | ok | 2025-07-01 | upd | `e5cd78bdebdde10f165a1bfdbe119d83643947ab7bbf10ed9f0204ad766ee6e0` |  |
| sec-radio | [V2000020563](https://adilet.zan.kz/rus/docs/V2000020563) | осн. | ok | 2026-08-14 | upd | `05fffb3aa05b47fcf95a7192e45a66c873039686cd74249817a520b7df6debe5` |  |
| sec-radio | [Z040000567_](https://adilet.zan.kz/rus/docs/Z040000567_) | доп. | ok | 2026-08-25 | upd | `c0705fda5ea0e04e9443f8d84c889e66a5e65d1269c5d58db7406f26cae56aa8` | сверено вручную: Закон «О связи» |
| sec-crypto-spec | [V2000020609](https://adilet.zan.kz/rus/docs/V2000020609) | осн. | ok | 2026-07-01 | upd | `2481cf4c5bbca02636ccd3e29bab979f264c3b73ab3706cb87ff9d8256dc14f8` |  |
| sec-crypto-spec | [Z1500000418](https://adilet.zan.kz/rus/docs/Z1500000418) | доп. | ok | 2026-08-25 | upd | `5ae64ff52371f1bde943b927eb9656681b3eed43ddebfac4672b174ccda5046c` | сверено вручную: Закон «О кибербезопасности» |
| sec-crypto-spec | [V2000020907](https://adilet.zan.kz/rus/docs/V2000020907) | доп. | ok | 2026-09-10 | upd | `ec4ae607ad17339d8882460253a8df45baaafd07181a34858d96abddc5187a31` | сверено вручную: Приказ МИИР № 374 — правила выдачи лицензий на импорт/экспорт отдельных видов товаров |
| sec-culture | [V2000020722](https://adilet.zan.kz/rus/docs/V2000020722) | осн. | ok | 2025-04-29 | upd | `79c2cec27680ad0860dd3a80e3756028aad957d75fd7f2e04ad3cfe86974770f` |  |
| sec-culture | [V2000020601](https://adilet.zan.kz/rus/docs/V2000020601) | доп. | needs_review | 2020-05-06 | new | `069215f351dbdf84279bfefea28971d1c2d66ed9ad25bbed12f2699b25847bd3` | Приказ № 111 — правила свидетельства на ВЫВОЗ культурных ценностей: к ввозу не относится |
| sec-culture | [Z1900000288](https://adilet.zan.kz/rus/docs/Z1900000288) | доп. | ok | 2026-08-25 | upd | `9b6e716fb91fe9dc7e12bd154f8f5d34c67ea781747b29522010cf41ab64f24a` | сверено вручную: Закон «Об охране и использовании объектов историко-культурного наследия» |
| sec-human-tissue | [V2000020556](https://adilet.zan.kz/rus/docs/V2000020556) | осн. | ok | 2026-10-07 | upd | `da6b34249ff6eb2dd308da7a13239bdb0ea01bff3a65cba639d74da91787593d` | сверено вручную: Приказ МЗ № ҚР ДСМ-43/2020 — лицензия на ввоз органов и тканей |
| sec-human-tissue | [V2000021592](https://adilet.zan.kz/rus/docs/V2000021592) | доп. | ok | 2026-10-07 | upd | `eb667b372423873eaa8f010b6e4d890dc06d5acc8caef2db588b049c0c8fded3` | сверено вручную: Приказ МЗ № ҚР ДСМ-177/2020 — заключения на ввоз |
| sec-weapons | [V2000020184](https://adilet.zan.kz/rus/docs/V2000020184) | осн. | ok | 2026-09-16 | upd | `43fe5ebf92dcb400ba74287843d3172cba80004f48ff03cccd29b105ad463530` |  |
| sec-weapons | [Z980000339_](https://adilet.zan.kz/rus/docs/Z980000339_) | доп. | ok | 2026-07-12 | upd | `c1e71c2afc231acd3300d70ae53359a4d6fe98d7cfab76849352930d1bd437a8` | сверено вручную: Закон «О государственном контроле за оборотом отдельных видов оружия» |
| sec-quotas | [V2000020135](https://adilet.zan.kz/rus/docs/V2000020135) | осн. | ok | 2026-03-26 | upd | `4b0c32914b7a9e47b1636d0f87a4c42216c47fc185498c2fc0616f61474cfe7a` |  |
| sec-quotas | [V2300033571](https://adilet.zan.kz/rus/docs/V2300033571) | доп. | ok | 2026-06-01 | upd | `3f3a5d897ca1416008f3d86c02c1e8975bc187baee1f155469ce9f1045e21430` |  |
