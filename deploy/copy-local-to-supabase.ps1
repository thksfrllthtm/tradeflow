# Переносит локальные данные (Docker Compose) в Supabase: базу и загруженные файлы.
#
# Запуск из корня репозитория:
#   powershell -ExecutionPolicy Bypass -File deploy\copy-local-to-supabase.ps1
#
# Нужны Docker Desktop и Go. Схема в Supabase должна быть уже создана: её создаёт сервис
# Render миграциями при запуске. Все данные в базе Supabase (кроме schema_migrations)
# заменяются локальными в одной транзакции: при ошибке база не меняется. Сессии не
# переносятся — после переноса все входят заново с локальными email и паролями.
#
# Строку подключения и ключ скрипт спрашивает скрытым вводом; их можно заранее положить
# в переменные окружения TARGET_DATABASE_URL, SUPABASE_URL и SUPABASE_SECRET_KEY.

$ErrorActionPreference = 'Stop'
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$container = 'import-calculator-db-1'
$filesVolume = 'import-calculator_private_files'

# Команды с базой выполняются в локальном контейнере PostgreSQL: там уже есть psql и pg_dump 17.
# Скрипт идёт через stdin, а строка подключения — через переменную окружения, чтобы она
# не попала в аргументы командной строки.
function Invoke-InDb([string]$Script) {
    # PowerShell дописывает в stdin завершающий CRLF: пусть "\r" попадёт в строку-комментарий.
    $out = (($Script -replace "`r", '') + "`n#") | docker exec -i -e TARGET_DATABASE_URL $container sh -eu
    if ($LASTEXITCODE -ne 0) { throw "Команда в контейнере $container завершилась с кодом $LASTEXITCODE" }
    $out
}

function Read-Secret([string]$EnvName, [string]$Prompt) {
    $value = [Environment]::GetEnvironmentVariable($EnvName)
    if (-not $value) {
        $secure = Read-Host $Prompt -AsSecureString
        $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
        try { $value = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr) }
        finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr) }
    }
    $value.Trim()
}

# Точное число строк в каждой таблице, кроме schema_migrations: "таблица количество".
$countSql = "select table_name || ' ' || (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %I.%I', table_schema, table_name), false, true, '')))[1]::text from information_schema.tables where table_schema = 'public' and table_type = 'BASE TABLE' and table_name <> 'schema_migrations' order by 1"
$local = 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At'
$target = 'psql "$TARGET_DATABASE_URL" -At'

Write-Host 'Запуск локальной базы и применение миграций...'
Push-Location $repoRoot
try {
    docker compose up -d db
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось запустить локальную базу (docker compose up -d db). Запущен ли Docker Desktop?' }
    $null | docker compose run --rm -T migrate
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось применить миграции к локальной базе (docker compose run --rm migrate).' }
} finally { Pop-Location }

$env:TARGET_DATABASE_URL = Read-Secret 'TARGET_DATABASE_URL' 'Строка подключения Supabase (Connect → Session pooler; ввод скрыт)'
if ($env:TARGET_DATABASE_URL -notmatch '^postgres(ql)?://') { throw 'Ожидается строка вида postgresql://...' }
# Как в entrypoint.sh: channel_binding не понимают используемые драйверы.
$env:TARGET_DATABASE_URL = $env:TARGET_DATABASE_URL -replace '([?&])channel_binding=[^&]*&?', '$1' -replace '[?&]$', ''
$env:SUPABASE_URL = Read-Secret 'SUPABASE_URL' 'Project URL Supabase (https://<ref>.supabase.co)'
$env:SUPABASE_SECRET_KEY = Read-Secret 'SUPABASE_SECRET_KEY' 'Secret key Supabase (sb_secret_...; ввод скрыт)'
$filesDir = Join-Path ([IO.Path]::GetTempPath()) ("tradeflow-files-" + [guid]::NewGuid().ToString('N'))

try {
    $localVersion = Invoke-InDb "$local -c 'select version from public.schema_migrations where not dirty'"
    $targetVersion = Invoke-InDb "$target -c 'select version from public.schema_migrations where not dirty'"
    Write-Host "Версия схемы: локально $localVersion, в Supabase $targetVersion"
    if (-not $localVersion -or "$localVersion" -ne "$targetVersion") {
        throw 'Версии схемы не совпадают. Сначала задеплойте текущий код на Render: он применит миграции к Supabase.'
    }

    Write-Host 'Сейчас в Supabase:'
    Invoke-InDb "$target -c `"$countSql`"" | ForEach-Object { Write-Host "  $_" }
    $answer = Read-Host 'Заменить ВСЕ данные в Supabase локальными? Введите yes'
    if ($answer -ne 'yes') { Write-Host 'Отменено.'; return }

    Write-Host 'Перенос базы...'
    # Сначала целиком собираем SQL (очистка таблиц + данные), потом выполняем его одной
    # транзакцией. SET transaction_timeout убираем: его знает только PostgreSQL 17+.
    # Пустой search_path из выгрузки делаем локальным для транзакции: через пул соединений
    # настройка сессии могла бы достаться другим клиентам.
    Invoke-InDb @'
set -o pipefail
sql=/tmp/supabase-copy.sql
trap 'rm -f "$sql"' EXIT
cat > "$sql" <<'SQL'
DO $$ BEGIN
  EXECUTE (SELECT 'TRUNCATE ' || string_agg(format('%I.%I', schemaname, tablename), ', ') || ' RESTART IDENTITY CASCADE'
           FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations');
END $$;
SQL
pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --data-only --no-owner --no-privileges \
  --exclude-table-data=schema_migrations --exclude-table-data=sessions \
  | sed -e '/^SET transaction_timeout/d' \
        -e "/^SELECT pg_catalog.set_config('search_path', '', false);\$/s/false/true/" >> "$sql"
psql "$TARGET_DATABASE_URL" -q -o /dev/null -v ON_ERROR_STOP=1 --single-transaction -f "$sql"
'@

    $localCounts = Invoke-InDb "$local -c `"$countSql`""
    $targetCounts = Invoke-InDb "$target -c `"$countSql`""
    $diff = Compare-Object @($localCounts | Where-Object { $_ -notmatch '^sessions ' }) @($targetCounts | Where-Object { $_ -notmatch '^sessions ' })
    Write-Host 'Теперь в Supabase:'
    $targetCounts | ForEach-Object { Write-Host "  $_" }
    if ($diff) { throw "Число строк в Supabase отличается от локального:`n$($diff | Out-String)" }
    Write-Host 'База перенесена.' -ForegroundColor Green

    # Файлы: только те, на которые ссылается база (остальные — от удалённых смет).
    Write-Host 'Перенос файлов...'
    $paths = @(Invoke-InDb "$local -c 'select stored_path from estimate_imports union select stored_path from import_assets order by 1'")
    New-Item -ItemType Directory -Path $filesDir | Out-Null
    [IO.File]::WriteAllText((Join-Path $filesDir '.list'), (($paths -join "`n") + "`n"))
    $copyScript = @'
cd /data/uploads
while read -r p; do
  if [ -f "$p" ]; then echo "$p"; else echo "missing file: $p" >&2; fi
done < /out/.list > /tmp/list
rm /out/.list
tar -cf - -T /tmp/list | tar -xf - -C /out
'@
    (($copyScript -replace "`r", '') + "`n#") | docker run --rm -i -v "${filesVolume}:/data:ro" -v "${filesDir}:/out" alpine sh -eu
    if ($LASTEXITCODE -ne 0) { throw "Не удалось скопировать файлы из тома $filesVolume" }

    Push-Location (Join-Path $repoRoot 'backend')
    try {
        go run ./cmd/upload-files -dir $filesDir
        if ($LASTEXITCODE -ne 0) { throw 'Не удалось загрузить файлы в Supabase Storage.' }
    } finally { Pop-Location }
    Write-Host "Готово: база и файлы ($($paths.Count) в базе) перенесены в Supabase." -ForegroundColor Green
} finally {
    Remove-Item Env:TARGET_DATABASE_URL, Env:SUPABASE_URL, Env:SUPABASE_SECRET_KEY -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $filesDir -ErrorAction SilentlyContinue
}
