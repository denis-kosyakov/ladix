# Модель надёжности Ladix — веха M3

> Якорь вехи **M3 «Надёжность»** конвейера v2 (`docs/v2-charter.md` §4). Источник истины для
> impl-чата: каркас миграций схемы Store, outbox/exactly-once доставка эффектов шага, единые часы
> во всех путях CLI, человеко-читаемое объяснение срабатывания триггера. Парный документ к
> `docs/engine-model.md` (§EN-*) и `docs/automation-model.md` (§AU-*); НЕ дублирует их, а ссылается.
>
> Скоуп — **минимально-честный** (D-C-1): закрываем ровно то, что делает усиленный сценарий §2
> зелёным и переживающим рестарт без потери/дублирования эффекта. Всё, что шире, — в §C-9 (бэклог).

---

## §C-0. Назначение, граница, инварианты

### C-0.1. Что закрывает M3

Поезд фич (топо-порядок, каждая = один автономный прогон impl-чата):

| Пункт | Фича | Размер | Закрывает |
|---|---|---|---|
| **(0)** | Усиление §2: реальный эффект **в теле шага** | S | DoD реально тестирует exactly-once (а не «почти-уже-идемпотентный» эффект эскалации) |
| **C2a** | Каркас forward-only миграций схемы Store (`PRAGMA user_version`) | S | Отзыв D-AU-9 (сброс→ALTER); фундамент для C2b и будущих расширений схемы |
| **C2b** | Outbox-леджер + дедуп эффектов шага per-(инстанс+шаг+индекс) | L | Тело шага доставляет эффект **exactly-once** через рестарт (POST ровно 1) |
| **C4** | Единые часы (`evalClockFromEngine`+`WithClock`) во всех путях CLI | M | Детерминизм метрики-стартующей-процесс-с-дедлайном; снимает «двойные часы» §8 |
| **C5** | Человеко-explain срабатывания (снимок значение↔порог) | M | Наблюдаемость DoD §2: оператор видит, *почему* сработал триггер |

### C-0.2. Несущие инварианты §5 (НЕ ломать — цели дрейф-аудита на M3-гейте)

Сверено вживую на master (база M2): см. `docs/v2-charter.md` §5.

- **Инвариант 1 — граница eval↔engine.** `eval.ProcessRuntime` = **ровно 8 методов**
  (`eval/runtime.go:9-45`), `eval` НЕ импортирует ни `store`, ни `engine` (подтверждено grep'ом —
  zero matches). **Правило M3:** outbox-дедуп живёт в `engine` (методы `CallExternal`/
  `CallExternalResult`/`Notify`), сигнатуры `ProcessRuntime` НЕ меняются; eval не трогается. Часы C4
  добавляют общий адаптер в `cmd/ladix`, не в eval/engine.
- **Инвариант 2 — контракт Store аддитивен и Value-ориентирован.** Store оперирует Go-структурами с
  `value.Value`; сериализация (type-tagged JSON) — внутреннее дело `SQLiteStore`. **Правило M3:**
  outbox = **новые методы** (`LoadOutbox`/`SaveOutbox`), Store растёт **16→18**; обе реализации
  (`MemoryStore`, `SQLiteStore`) + compile-замок обновляются.
- **Инвариант 3 — фронтенд изолирован.** Лексер/парсер/AST/семантика не зависят от движка/Store. M3
  не трогает фронтенд: усиление §2 использует ТОЛЬКО существующие конструкции (шаг без `исполнитель`,
  `после`, `присвоить`, `уведомить` в теле шага). Новых ключевых слов / SE-кодов / eval-кодов нет.

> **Поправка факта к промпту/памяти.** Store compile-замок — **ДВОЙНОЙ**, не тройной:
> `var _ Store = (*MemoryStore)(nil); var _ Store = (*SQLiteStore)(nil)` (`store.go:43-46`). Третьей
> прод-реализации нет. M3 добавляет два метода к обеим и сохраняет двойной замок.

### C-0.3. Решения вехи (залочены владельцем 2026-06-17 + производные архитектора)

| # | Решение | Источник |
|---|---|---|
| **D-C-1** | Скоуп M3 = минимально-честный (таблица C-0.1); широкое — в §C-9. | владелец |
| **D-C-2** | Отозвать D-AU-9: сброс схемы → **forward-only ALTER**, `PRAGMA user_version`, база=1 для БД с `user_version=0` (= «схема 006/007/018»). | владелец |
| **D-C-3** | Серверная модель — **(а) CLI-над-общим-SQLite + harden** (стресс конкуренции `serve`×`complete`/`emit`); демон+HTTP+`--json` → бэклог. | владелец |
| **D-C-4** | C2 раньше C1; дедуп **per-(instance_id + step_name + effect_index)**; хеш определения НЕ нужен. | владелец |
| **D-C-5** | §2 усиление — **split «захват + эффект»** (вариант A): авто-шаг `зафиксировать_итог` захватывает payload в durable-переменную, авто-шаг `уведомить_crm` делает эффект, читая durable-переменную → replay-safe. | владелец |
| **D-C-6** | C5 explain — **всегда-вкл при срабатывании** (run печатает, serve логирует); `--json`/audit → бэклог. | владелец |
| **D-C-7** | Дедуп НЕ чистый декоратор `ExternalCaller` (у него нет контекста инстанс/шаг) — живёт в effect-методах движка, где доступен `e.active`. Чистый декоратор возможен только с backref на движок; отвергнут как лишний слой. | архитектор (вынужденно фактом кода) |
| **D-C-8** | Outbox = **леджер идемпотентности** (консультируется при dispatch'е эффекта), НЕ FIFO-очередь с воркером-дренажём: переисполнение эффекта на рестарте драйвится существующим рестарт-сканом, отдельного дренажа не нужно. | архитектор |
| **D-C-9** | Транзакционный порядок — **deliver-then-record** с pre-check дедупа: остаточное окно «доставлено-но-не-помечено» = at-least-once (идемпотентность приёмника вне scope, как fault-ветка-3 `checkDeadlines`). | архитектор |

---

## §C-1. Усиление золотого сценария §2 (пункт 0)

### C-1.1. Что изменилось в `docs/v2-charter.md` §2

В процесс `эскалация_плана` добавлены **два авто-шага** (без `исполнитель` → исполняются движком при
`advance`, не создают человеческую задачу):

```ladix
процесс эскалация_плана(текущая_выручка):
    шаг связаться_с_клиентом:                            # человеческий шаг (исполнитель + срок)
        исполнитель: "менеджер"
        срок:        2дн
        присвоить факт = текущая_выручка
    шаг зафиксировать_итог после связаться_с_клиентом:   # авто-шаг: захват payload → durable
        присвоить итог = данные.итог
    шаг уведомить_crm после зафиксировать_итог:          # авто-шаг: реальный эффект в теле шага
        уведомить crm("итог звонка: " + итог)
```

### C-1.2. Почему именно split (захват + эффект), а не одношаговая форма

Все четыре факта подтверждены вживую (см. также §C-1.3):

1. **payload эфемерен.** `--данные '{"итог":"перезвонит"}'` парсится в `value.Запись` и биндится
   **единой записью** под именем `данные` (`const payloadName = "данные"`, `engine.go:26`), **НЕ**
   разворачивается в переменные; виден ТОЛЬКО первому шагу прогона `advance`, в `inst.Variables` НЕ
   пишется, на рестарте **пуст** (`ReactivateInstance` → `advance(inst, value.NewRecord(nil, nil))`,
   `engine.go:457`, комментарий §AU-5.3 / D-AU-3).
2. **доступ к полю отсутствующего ключа graceful.** `данные.итог` (`FieldExpr`) на пустой записи →
   `Запись.Get` возвращает `Пусто`/None, не ошибку (`value/record.go:24`).
3. **`+` строгий.** `evalAdd` (`eval/arith.go:105`): `Строка + Строка` конкатенирует; `Строка + Пусто`
   (и любой не-`Строка` RHS) **без коэрции** падает в `typeErr`: `'+' нельзя применить к Строка и Пусто`.
4. **аргумент эффекта вычисляется ДО dispatch'а.** `evalNotifyAction` (`eval/stmt.go:124`) зовёт
   `evalArgs` до `runtime.Notify`; ошибка арг-эвала проваливает шаг **до** всякой внешней доставки.

**Следствие — одношаговая форма СЛОМАНА.** Тело `уведомить crm("итог звонка: " + данные.итог)` в
одном авто-шаге на рестарте: `данные`=пуст → `данные.итог`=`Пусто` → `"…" + Пусто` = **ОшибкаТипа**
→ шаг **проваливается** (статус `провален`), а НЕ даёт дубль POST. Гейт «POST ровно 1 через рестарт»
так недостижим — на рестарте нет даже первой попытки POST.

**Split решает это.** `присвоить x = …` в теле шага **durable** (хук `AssignProcessVar` →
`inst.Variables[x]=v` → `SaveInstance`, `engine/runtime.go:25-42`; `eval/stmt.go:87-100`), переживает
рестарт. Эффект-шаг `уведомить_crm` читает durable-переменную `итог` (загружается в `processEnv` из
`inst.Variables` в начале каждого `advance`, `engine.go:251-253`), **не** перечитывает payload → на
рестарте арг-эвал успешен (`итог`="перезвонит"), эффект переисполняется, и его глушит outbox-дедуп.

### C-1.3. Жизненный цикл и точная локализация at-least-once зазора

Нормальный прогон (`complete … --данные '{"итог":"перезвонит"}'`):

1. `связаться_с_клиентом` (человеческий): `advance` → `status=running` (▼сохранён до тела,
   `engine.go:268`) → тело (`присвоить факт`) → `hasAssignee` → задача создана → `status=waiting`
   (▼) → `advance` вернулся. Инстанс спит.
2. `complete`: `advanceAfterComplete` (`engine.go:196`) — `MarkTaskCompleted`, `CurrentStep`=next
   (`зафиксировать_итог`, `engine.go:208`), `advance(inst, data={итог:перезвонит})`.
3. `зафиксировать_итог` (первый шаг этого прогона, видит payload): `присвоить итог = данные.итог` →
   `итог`="перезвонит" **durable** (▼`SaveInstance`). Нет `исполнитель` → next, `cur`→пустая запись
   (`engine.go:295`).
4. `уведомить_crm` (НЕ первый шаг прогона — `данные` уже пуст, но шаг его не читает): `status=running`
   (▼) → тело `уведомить crm("итог звонка: " + итог)`, `итог`="перезвонит" из `processEnv` → эффект
   → **outbox помечает** ключ `(p-NNN, уведомить_crm, 0)` доставленным. Нет next → `status=done` (▼).

**Зазор.** Между `status=running` (▼ в начале итерации `уведомить_crm`) и `status=done` (▼ терминала)
лежит сам POST. Краш здесь оставляет инстанс `running` на `уведомить_crm`. Рестарт-скан
(`daemon/restart.go:28`, статусы `running`/`created`) поднимает его → `ReactivateInstance` →
`advance(inst, ПУСТОЙ payload)`. Тело `уведомить_crm` переисполняется: `итог`="перезвонит" (durable,
пережил рестарт) → арг-эвал успешен → `engine.Notify` → **outbox видит ключ доставленным → POST
пропущен**. POST ровно 1. ✓

Это ровно тот зазор, который для эффекта **эскалации** уже закрыт durable-флагом `Escalated`
(`checkdeadlines.go:44`, доказано `m2_golden_test.go:282`); M3 переносит ту же гарантию на эффект
**тела шага** через outbox.

### C-1.4. Гейт-критерий усиленного §2

1. Усиленный §2 проходит зелёным end-to-end под `serve` (CSV-источник → метрика → триггер → процесс →
   `complete --данные` → авто-шаги → реальный POST `crm`).
2. **Рестарт mid-advance** (краш на `уведомить_crm` после POST, новый Store на той же `--db`,
   рестарт-скан → реактивация): эффект `crm` доставлен **exactly-once, POST ровно 1**.
3. Тест-замок зеркалит `driveServeToNoRepeat`/`TestDeadlineDurableRestart` (см. §C-2b.7).

**Честная граница (в §2 и здесь).** payload эфемерен (D-AU-3): краш в узком окне между завершением
человеческого шага и персистом `присвоить итог = данные.итог` (шаг 3) теряет payload — вне scope
outbox. Outbox гарантирует exactly-once для эффекта, читающего **уже сохранённую** переменную.

---

## §C-2a. Каркас миграций схемы Store (отзыв D-AU-9)

### C-2a.1. Что меняется относительно v1

Сегодня (`store/sqlite.go`): схема — единый `const ddl` (`:23-67`), исполняется
`db.Exec(ddl)` (`:94`) при **каждом** открытии в `NewSQLiteStore` (`:79-98`); все `CREATE … IF NOT
EXISTS`. **`PRAGMA user_version` нигде нет** (`const pragmas` `:69-74` = только `journal_mode=WAL`,
`busy_timeout=5000`, `foreign_keys=ON`) → любая БД (свежая и существующая) рапортует `user_version=0`.
Колонки/таблицы M2/007 добавлялись **редактированием `ddl` на месте** + политикой D-AU-9 «сброс БД».
Это работает для НОВЫХ таблиц (`IF NOT EXISTS` идемпотентен), но **новую колонку существующей
таблице** `CREATE TABLE IF NOT EXISTS` не добавит — латентный гэп, который D-AU-9 маскировал сбросом.

**D-C-2:** отзываем D-AU-9. Вводим **forward-only** версионирование схемы через `PRAGMA user_version`.

### C-2a.2. Модель версий

- `baselineVersion = 1` — схема, которую создаёт `const ddl` (`instances`/`tasks`/`counters`/
  `trigger_state`/`events` + индексы + сид счётчиков). Это «схема 006/007/018».
- `currentSchemaVersion = 2` — после M3. Миграция **1→2** = таблица `outbox` (§C-2b).
- БД с `user_version=0` (свежая ИЛИ до-версионная существующая) трактуется как **базовая (=1)**:
  `const ddl` уже создал её схему идемпотентно; миграции применяются от версии 2 и выше.

### C-2a.3. Точка встройки и алгоритм

В `NewSQLiteStore`, **после** `db.Exec(pragmas)` и `db.Exec(ddl)`, **до** `return`:

```go
if _, err := db.Exec(ddl); err != nil { db.Close(); return nil, err }
if err := migrate(db); err != nil { db.Close(); return nil, err }   // ◀ НОВОЕ
return &SQLiteStore{db: db}, nil
```

`migrate` — forward-only раннер; каждый шаг и бамп версии атомарны (шаблон `nextCounter` `:308-328`):

```go
// schemaMigrations[i] переводит схему с версии (baselineVersion+i) на (baselineVersion+i+1).
// Каждая — набор DDL-операторов; исполняется ОДИН раз, в транзакции вместе с бампом user_version.
var schemaMigrations = []string{
    // 1 → 2: outbox-леджер (§C-2b).
    `CREATE TABLE IF NOT EXISTS outbox (
        dedup_key    TEXT PRIMARY KEY,
        instance_id  TEXT NOT NULL,
        step_name    TEXT NOT NULL,
        effect_index INTEGER NOT NULL,
        kind         TEXT NOT NULL,
        target       TEXT NOT NULL,
        args_json    TEXT NOT NULL,
        result_json  TEXT,
        delivered    INTEGER NOT NULL DEFAULT 0,
        created_at   TEXT NOT NULL,
        delivered_at TEXT
    );
    CREATE INDEX IF NOT EXISTS idx_outbox_instance ON outbox(instance_id, step_name);`,
}

func migrate(db *sql.DB) error {
    var v int
    if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil { return err }
    if v == 0 { // свежая ИЛИ до-версионная БД: const ddl уже дал схему baselineVersion.
        v = baselineVersion
        if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, baselineVersion)); err != nil {
            return err
        }
    }
    target := baselineVersion + len(schemaMigrations)
    for v < target {
        stmt := schemaMigrations[v-baselineVersion] // v→v+1
        tx, err := db.Begin()
        if err != nil { return err }
        if _, err := tx.Exec(stmt); err != nil { _ = tx.Rollback(); return err }
        // user_version нельзя биндить ? — версия из доверенной int-константы (не из ввода).
        if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v+1)); err != nil {
            _ = tx.Rollback(); return err
        }
        if err := tx.Commit(); err != nil { return err }
        v++
    }
    return nil
}
```

**Идиомы/ловушки Go (для impl-чата):**
- `PRAGMA user_version = N` **нельзя** параметризовать `?` — подставляем `int`-константу через
  `fmt.Sprintf` (значение из доверенного литерала кода, не из пользовательского ввода — инъекции нет).
- `PRAGMA user_version` транзакционен в SQLite → бамп внутри `tx` атомарен с DDL шага.
- `db.SetMaxOpenConns(1)` (`sqlite.go:89`) сериализует доступ — `migrate` обязан использовать
  **тот же** `*sql.DB`, не открывать второе соединение (иначе busy/несогласованность user_version).
- Миграция 1→2 — `CREATE TABLE` standalone-таблицы **без FK** → классический «rebuild table» танец и
  `PRAGMA foreign_keys` НЕ нужны. *Если* будущая миграция тронет колонки таблицы с FK
  (`tasks.instance_id REFERENCES instances`, `sqlite.go:35`) — `PRAGMA foreign_keys` **нельзя** менять
  внутри транзакции (no-op в BEGIN/COMMIT); планировать вне tx. Для M3 неактуально.
- `MemoryStore` миграций не имеет (нет персиста схемы) — `migrate` чисто `SQLiteStore`. Контракт Store
  не расширяется ради миграций; это внутреннее дело реализации (Инвариант 2).

### C-2a.4. Тест-замки C2a

- `TestMigrateFreshDB`: новый файл → `NewSQLiteStore` → `PRAGMA user_version` == `currentSchemaVersion`
  (2); таблица `outbox` существует.
- `TestMigrateLegacyV0`: вручную создать только базовые таблицы + `PRAGMA user_version=0` (симуляция
  до-версионной БД) → `NewSQLiteStore` → версия==2, `outbox` появилась, **данные базовых таблиц целы**
  (вставить инстанс/задачу до миграции, проверить после).
- `TestMigrateIdempotent`: повторное открытие уже-мигрированной БД (версия 2) → `migrate` no-op,
  версия остаётся 2, без ошибок.
- **Инверсия (мутпроба):** убрать строку миграции 1→2 из `schemaMigrations` → `TestMigrateFreshDB`
  краснит (нет `outbox` / версия 1). Зафиксировать в комментарии теста.

---

## §C-2b. Outbox-леджер и exactly-once доставка эффектов шага

### C-2b.1. Модель (D-C-8): леджер идемпотентности, не очередь

Outbox — **durable леджер доставленных эффектов**, ключ = `(instance_id, step_name, effect_index)`.
Консультируется в момент dispatch'а эффекта тела шага. **Нет** отдельного воркера-дренажа: повторная
доставка драйвится существующим рестарт-сканом (`advance` переисполняет тело шага), а леджер решает
«доставлять или пропустить». Это лежит на уже доказанной механике 007b, не добавляет фоновых циклов.

> **Граница восстановления (liveness, не exactly-once).** Переисполнение недоставленного эффекта
> драйвит рестарт-скан (`daemon/restart.go:28`), который запускается **только** при подъёме `serve`.
> В чисто-CLI развёртывании без `serve` инстанс, упавший mid-auto-step (статус `running`), будет
> переисполнен лишь при следующем подъёме `serve` — это граница **liveness** (когда эффект доедет),
> не нарушение exactly-once (POST ≤ 1 всегда). Под D-C-3 серверная модель = serve-над-общим-SQLite,
> так что для сценария §2 (`serve`) восстановление автоматическое.

> Уточнение к kickoff-формулировке «клон events-FIFO». Совпадает **форма таблицы** (id-ключ +
> payload + флаг обработки + индекс), но **механизм иной**: events дренажится воркером тика; outbox —
> консультируется при dispatch'е. Поэтому методов меньше (Load/Save, без `ListUnprocessed`-дренажа).

### C-2b.2. Где живёт дедуп (D-C-7): effect-методы движка, не декоратор `ExternalCaller`

`ExternalCaller.Call/Notify` (`caller.go:18-21`) принимают только `(target, args)` — **нет** контекста
инстанс/шаг/индекс. Контекст есть у движка: стек `e.active` (`engine.go`, кадр `activeFrame{inst,
processEnv}`, толкается в `advance`). Поэтому дедуп — в `Engine.CallExternal`/`CallExternalResult`/
`Notify` (`engine/runtime.go:47-62`), которые:
- **funnel шага** — тело шага (`ExecStepBody`) зовёт `runtime.Notify`/`CallExternal*` при `len(e.active)>0`;
- **funnel не-шага** — тела триггеров/эскалации/`запустить процесс` top-level зовут те же методы при
  `len(e.active)==0` (нет активного кадра шага).

`ProcessRuntime` сигнатуры **не меняются** (Инвариант 1) — дедуп добавляет поведение, не интерфейс.

> **Факт-сноска (3 точки независимы).** В живом коде `engine.CallExternal` зовёт `e.caller.Call`
> **напрямую** (`engine/runtime.go:53-58`), НЕ делегирует `CallExternalResult` — вопреки устаревшему
> комменту интерфейса `eval/runtime.go:23`. Поэтому дедуп нужен в **каждом** из трёх методов
> независимо (`CallExternal`, `CallExternalResult`, `Notify`); нельзя обернуть только один и считать,
> что прочие наследуют.

**Дедуп НЕ требует пер-командной проводки.** Он живёт в effect-методах движка над `e.st` → активен
в **каждом** из 5 построенных движков автоматически: `run` (`main.go:255`), `complete` (`main.go:451`),
`start` (`start.go:138`), `serve` (`serve.go:210` через `buildServeDaemon`), `metric` (`main.go:319`).
Все пять делят инъектированный `st` (`engine.go:38/50`). `serve` использует свой **единственный** `eng`
из `buildServeDaemon` без нового шва — outbox «просто работает». Гарантия рестарта реальна там, где
Store durable (SQLite/`--db`); для `metric` (одноразовый, без рестарта/advance тела шага) дедуп
безвреден-холостой.

### C-2b.3. Граница применимости: только эффекты тела шага

Дедуп через outbox применяется **тогда и только тогда**, когда есть активный кадр шага
(`len(e.active) > 0`). Иначе (тело метрики-триггера `когда метрика … : уведомить …`; тело эскалации
`когда задача просрочена …`; расписание) — **delegate напрямую** в `e.caller`, как сегодня:
идемпотентность этих путей обеспечивают существующие durable-гарды (ребро `LastBool` триггера,
`Escalated` задачи). Это держит модели разделёнными и не дублирует гарантии.

| Источник эффекта | Активный кадр? | Дедуп | Идемпотентность |
|---|---|---|---|
| тело шага (`ExecStepBody`) | да | **outbox** (M3-C2b) | ключ инстанс+шаг+индекс |
| тело метрики-триггера | нет | — | ребро `LastBool` (007) |
| тело эскалации-триггера | нет | — | `Escalated`-флаг (M2) |
| расписание | нет | — | `last_fired_date`/`last_fire` (007) |

### C-2b.4. effect_index — детерминированный счётчик

`effect_index` = порядковый номер эффекта (`вызвать`/`уведомить`) в теле текущего шага, считается от 0
**при каждом** исполнении тела. `ExecStepBody` (`eval/exports.go:38`) идёт по `body []ast.Statement`
строго по порядку → на рестарте тот же эффект получает тот же индекс. Хранение: новое поле кадра,
сбрасывается движком в 0 в начале каждой итерации шага в `advance` (рядом с `status=running` ▼), и
инкрементируется в каждом effect-методе:

```go
// activeFrame получает поле effectIndex int.
// advance, в начале итерации шага (перед ExecStepBody):
frame.effectIndex = 0
// Engine.Notify (и CallExternal/CallExternalResult), при len(e.active)>0:
fr := e.active[len(e.active)-1]
idx := fr.effectIndex
fr.effectIndex++
key := outboxKey(fr.inst.ID, fr.inst.CurrentStep, idx) // fmt.Sprintf("%s|%s|%d", …)
```

`step_name` = `fr.inst.CurrentStep` (в момент `ExecStepBody` он равен исполняемому шагу — `advance`
ставит его до тела). Аргументы (`args`) уже вычислены eval'ом и переданы в метод.

### C-2b.5. Протокол dispatch'а (D-C-9: deliver-then-record + pre-check)

В каждом effect-методе движка при активном кадре шага:

```
1. key := (inst.ID, CurrentStep, effect_index++)
2. rec, err := e.st.LoadOutbox(key)
   - err==nil && rec.Delivered:           // уже доставлено в прошлой жизни инстанса
       → НЕ звать e.caller; вернуть сохранённый результат
         (CallExternalResult → rec.Result; CallExternal/Notify → nil). СТОП.
   - ErrOutboxNotFound (или не delivered): продолжаем.
3. v, derr := e.caller.Call|Notify(target, args)   // реальная доставка (POST/печать)
4. derr != nil → вернуть derr (шаг провалится, D-14); outbox НЕ помечаем delivered.
5. derr == nil → e.st.SaveOutbox(&OutboxRecord{key, inst, step, idx, kind, target, args, v,
                  Delivered:true, CreatedAt:now, DeliveredAt:&now})  (upsert)
6. вернуть v (или nil для statement-формы).
```

**Зачем `result_json`:** `CallExternalResult` (B1, выражение-форма `вызвать`) захватывает результат в
переменную. На пропуске-по-дедупу метод обязан вернуть **тот же** результат, иначе логика процесса
разойдётся → результат сериализуется в `result_json` (type-tagged value-кодек Store) и декодируется
при пропуске. `уведомить`/statement-`вызвать` результат отбрасывают → на пропуске `nil`/`None`.

**Честная граница (D-C-9, в §C-9 бэклог):** окно между успешным POST (шаг 3) и `SaveOutbox` (шаг 5).
Краш здесь → на рестарте ключ ещё не `delivered` → повторный POST = **at-least-once**. Закрыть его до
конца можно лишь идемпотентностью **приёмника** (мы не владеем `crm`) — вне scope. Это ровно то же
окно, что fault-ветка-3 `checkDeadlines` (fire-then-persist `Escalated`, §C-2b.8). Гейт §2 крашится
**после** `SaveOutbox` → POST ровно 1; узкое окно гейт не трогает (как `m2_golden` крашится после
успешного `SaveTask`).

### C-2b.6. Новые методы Store (16→18, аддитивно)

Зеркалят `LoadTriggerState`/`SaveTriggerState` (Load→ошибка-не-найдено, Save=upsert):

```go
// --- outbox-леджер (M3-C2b, аддитивно §AU-2 16→18) ---
LoadOutbox(dedupKey string) (*OutboxRecord, error) // не найдено → ErrOutboxNotFound
SaveOutbox(rec *OutboxRecord) error                // upsert по dedup_key
```

Тип (в `store/types.go`, рядом с `Event`/`TriggerState`; Value-ориентирован — Инвариант 2):

```go
type OutboxRecord struct {
    DedupKey    string
    InstanceID  string
    StepName    string
    EffectIndex int
    Kind        string         // "вызвать" | "уведомить"
    Target      string
    Args        []value.Value  // сериализуются type-tagged JSON внутри SQLiteStore
    Result      value.Value    // вызвать → результат; уведомить → value.None
    Delivered   bool
    CreatedAt   time.Time
    DeliveredAt *time.Time
}
```

- `ErrOutboxNotFound` — новый sentinel в `store/errors.go` рядом с `ErrTriggerStateNotFound`.
- `MemoryStore`: `map[string]*OutboxRecord`. Дедуп на Memory **эфемерен** (нет рестарт-гарантии — тест
  рестарта гейта §2 гоняется на `SQLiteStore`/`--db`). Глубокая копия при Save/Load (как `copyTask`)
  **обязана** копировать срез `Args` в новый `[]value.Value` (как `copyTask` копирует указатели
  времён), иначе мутация значений в движке протечёт в леджер.
- `SQLiteStore`: `SELECT … WHERE dedup_key=?` → `ErrOutboxNotFound` на `sql.ErrNoRows`; `INSERT …
  ON CONFLICT(dedup_key) DO UPDATE` (как `SaveTask` `sqlite.go:161`). Сериализация через
  **существующий** value-кодек (`store/codec.go`): `Args` → `encodeList(value.NewList(args))` (нет
  готового хелпера для голого `[]value.Value` — заворачиваем в `Список`); `Result` → `encodeValue`
  (`None` → tagged-`Пусто` blob, `codec.go:88/250` — **НЕ** SQL `NULL`). Колонка `result_json` хранит
  этот blob; для statement-форм (`уведомить`/statement-`вызвать`) `Result = value.None` даёт
  детерминированный tagged-`Пусто`. Декод на пропуске-по-дедупу — обратными `decodeList`/`decodeValue`.
- **Compile-замок остаётся ДВОЙНЫМ** (`MemoryStore`, `SQLiteStore`); обе реализации обязаны иметь оба
  метода, иначе сборка падает — это и есть замок.

### C-2b.7. Тест-замки C2b (exactly-once)

- **`TestStepEffectExactlyOnceRestart` (гейт §2, durable):** прогнать §2 до `уведомить_crm` (POST 1),
  открыть новый Store на той же `--db`, `RunRestartScan` → реактивация, прогнать тики → счётчик POST
  **остался 1**. Зеркалит `driveServeToNoRepeat` (`m2_golden_test.go:234`) и `TestDeadlineDurableRestart`.
- **`TestOutboxLedgerSkipsDelivered` (Go-API):** дважды позвать `Engine.Notify` под одним активным
  кадром+ключом (через `ExecStepBody` или прямой драйв) → `e.caller` вызван **один** раз; второй —
  пропуск по `LoadOutbox.Delivered`.
- **`TestOutboxResultReplay`:** `CallExternalResult` под дедупом возвращает сохранённый `Result` без
  повторного `Call`.
- **Инверсия (мутпробы):**
  - снять pre-check (`if rec.Delivered → skip`) → POST дважды → `TestStepEffectExactlyOnceRestart`
    краснит.
  - сломать `effect_index` (всегда 0 при ≥2 эффектах в шаге) → коллизия ключей → второй эффект
    «съеден» → тест на два-эффекта-в-шаге краснит.

**Стратегия примера и golden-churn (обязательна — иначе сломаются M2-замки).** Гейт-тест
`TestStepEffectExactlyOnceRestart` — **inline-const** источник сценария (по образцу `m2CLISrc` /
`m2_golden_test.go`), самодостаточен и **изолирован** от golden'ов файлов-примеров. Усиленный §2 как
**демо** — это эволюция канонического `examples/контроль_плана.ladix` (добавить два авто-шага); тогда
**обязательно** переснять его замки: `main_test.go:137` (`TestCLIGoldenDeadlineEscalation`, exact
stdout — добавится строка эффекта `crm`) и строку `examples/MANIFEST.md:151`; арность процесса не
меняется (`start_golden_test.go:46` `TestStartArityMismatch` не затронут — параметр один, как прежде).
Рекомендация: гейт-тест на inline-const (низкий риск), демо-файл — эволюция с переснятыми замками.

### C-2b.8. Реальные fault-тесты `checkDeadlines` (3 ветки)

`checkDeadlines` (`daemon/checkdeadlines.go:22`) корректен, но 3 fault-ветки **не покрыты** (есть только
happy/no-trigger/completed/durable). M3 превращает их в реальные тесты надёжности (инъекция
fault-Store; нет файла `*fault*` в `internal/daemon` сегодня):

- **Ветка 1 (`:38-41`)** `ListPendingTasks` error → лог `"checkDeadlines: листинг задач: %s"` + ранний
  `return`, демон жив. Тест: Store, чей `ListPendingTasks` возвращает ошибку → фаза не паникует,
  лог-строка присутствует, тик продолжается.
- **Ветка 2 (`:50-53`)** `LoadInstance` error → `continue` (задача пропущена, прочие обрабатываются).
  Тест: Store, чей `LoadInstance` падает для одной задачи → нет эскалации этой задачи, нет паники,
  остальные задачи обработаны.
- **Ветка 3 (`:63-65`)** `SaveTask`(Escalated) error → лог `"checkDeadlines: персист Escalated задачи
  %s: %s"`; **тело уже сработало** (POST отправлен) → следующий тик/рестарт **перешлёт** = at-least-once.
  Тест: Store, чей `SaveTask` падает после fire → лог-строка присутствует; **зафиксировать в комментарии
  теста**, что это известное окно fire-then-persist (пара к §C-2b.5/§C-9), а не дефект.

---

## §C-4. Единые часы во всех путях CLI

### C-4.1. Проблема (развилка §8 «двойные часы»)

Две намеренно разные абстракции часов (D-2): `engine.Clock` (`Now() time.Time`, lifecycle —
`clock.go:5-31`) и `eval.Clock` (`Now() value.Дата`, дата метрик — `eval/clock.go:12-27`). В **`serve`**
они уже едины: один инъектированный `engine.Clock` фанится в интерпретатор (через адаптер
`evalClockFromEngine`, `serve.go:32-38`), движок (`WithClock`) и демон (`buildServeDaemon`,
`serve.go:201-223`); залочено `serve_golden_test.go:216`. В прочих путях — **независимые** `SystemClock{}`:

| Команда | Функция (`cmd/ladix`) | Текущее состояние часов | Действие M3 |
|---|---|---|---|
| **serve** | `buildServeDaemon` | **едины** (инъекция фанится в interp+engine+daemon) | **НЕ трогать** (залочено) |
| **run** | `runFile` (`main.go:251/255/277`) | ТРИ независимых: `eval.SystemClock{}` interp + дефолт engine + `engine.SystemClock{}.Now()` для сводки задач | один `engine.Clock` → adapter+`WithClock`+`clock.Now()` для сводки |
| **start** | `startMain` (`start.go:133/138`) | ДВА независимых: `eval.SystemClock{}` interp + дефолт engine | один `engine.Clock` → adapter+`WithClock` |
| **complete** | `completeTask` (`main.go:446/451`) | ДВА независимых; **engine-часы штампуют `MarkTaskCompleted`/`UpdatedAt`** (наблюдаемо) | один `engine.Clock` → adapter+`WithClock` |
| **tasks** | `listTasks` (`main.go:559`) | сырой `engine.SystemClock{}.Now()` для `FormatTaskLine` | инъекция `engine.Clock`-параметра в `listTasks` |
| **metric** | `runMetric` (`main.go:296/319`) | eval-часы инъектируемы; engine-часы — дефолт (не связан) | добавить `WithClock` тех же часов (полнота; эффект латентный) |
| **emit** | `emitEvent` (`emit.go:58`) | `engine.Clock` уже инъектируем | без изменений |

> Поправка путей к промпту: `run.go`/`tasks.go`/`metric.go` НЕ существуют — всё в `cmd/ladix/main.go`.
> «Два `time.Now()`» в `start` неточно: `start` не содержит литерального `time.Now()` — это два
> независимых `SystemClock{}` (eval `:133` + дефолт engine через `:138`), чьи `.Now()` дают два разных
> момента. Гэп реален; формулировка — «два источника `SystemClock`».

### C-4.2. Паттерн (вынести `evalClockFromEngine` в shared)

`evalClockFromEngine` (адаптер `engine.Clock`→`eval.Clock`, усекает момент до Y/M/D в Local) живёт
неэкспортированным в `serve.go:32-38`. Вынести в общий файл `cmd/ladix` (напр. `main.go` или
`clock_adapter.go`), чтобы run/start/complete/tasks/metric использовали тот же тип. `serve` продолжает
им пользоваться без изменений (его golden — `serve_golden_test.go:216`, fake `fixedClock` engine.Clock
`:21-23` — обязан компилироваться дальше).

Канонический рецепт на команду (как `buildServeDaemon`): принять один `clock engine.Clock` (прод
`engine.SystemClock{}`); `interp := eval.NewInterpreter(out, depth, evalClockFromEngine{clock})`;
`eng := engine.NewEngine(st, interp, out, append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)...)`;
любой «сейчас» для сводок = `clock.Now()`.

> **Реализационная пометка (M3-гейт, C4-1).** Путь `metric` исторически входит через
> `runMetric(clock eval.Clock)` (eval-часы, не engine), поэтому импл проволочил engine-часы обратным
> адаптером `engineClockFromEval` вместо forward `evalClockFromEngine{engine.Clock}`. Инвариант «один
> источник времени на вызов» **сохранён** (engine.Clock выводится из того же eval.Clock); отклонение — в
> направлении адаптера, не в семантике. Forward-рецепт остаётся каноном для остальных путей.

### C-4.3. Тест-замки C4

- На команду (run/start/complete/tasks/metric): инъектировать `fixedClock{2026-…}` (engine.Clock fake)
  и проверить, что и дата метрик интерпретатора, и lifecycle-штампы движка следуют ему (для `complete`
  — `MarkTaskCompleted`/`UpdatedAt` детерминированы; golden может маскировать момент или фиксировать).
  Зеркало паттерна `serve_golden_test.go:216` (`TestServeMetricDateFollowsSchedulerClock`).
- `tasks` и `run`-сводка: добавить замок инъектируемости (сегодня его нет — сырой `SystemClock{}`).
- **Инверсия:** вернуть независимый `SystemClock{}` в команде → дата/штамп расходятся с инъекцией →
  тест краснит.

### C-4.4. Граница — монотонные часы в бэклог

Монотонные часы **фундаментально несовместимы** с durable-рестартом: RFC3339 (формат персиста времён)
отбрасывает монотонную компоненту `time.Time`. M3 даёт **единые wall-clock** часы во всех путях; моно
→ §C-9.

---

## §C-5. Человеко-explain срабатывания (наблюдаемость)

### C-5.1. Что уже есть, что протянуть

Снимок значения метрики **уже вычисляется** в обоих путях; ничего не вычисляем заново — **печатаем**:

- **run** (`eval/trigger_run.go:78-92`, fire-if-true): в точке fire в области видимости **всё**:
  `spec.Metric.Name`, `metricVal` (снимок), `threshVal` (порог), `ast.BinOp(spec.Op)`
  (`BinOp.String()` → `==`/`<`/…), `fired`. Печать explain — без протяжки. **Писатель:** `i.out`
  (туда же падает печать тела/снимка), **НЕ** параметр `w` из `RunTriggers(w io.Writer)` (`w` —
  только канал заглушек событий/расписания + сводка; `metric_window_golden_test.go:102-104` это
  различает: тело→`out`(=`i.out`), `RunTriggers`-сводка→`w`). Протянуть `i.out` в `runMetricTrigger`
  (или эмитить прямо через `i.out`).
- **serve**: снимок считается на `daemon/metrics.go:39` (`EvalMetricCondition`), ребро-детект
  `fired := ts.LastBool!=nil && !*ts.LastBool && cur` на `metrics.go:72`, **точка печати explain** —
  ветка `if fired {` на `metrics.go:82` (до `safeFire`/`fireBody`). **Порог `threshVal` выбрасывается**
  внутри `EvalMetricCondition` (`eval/trigger_daemon.go:41`, локальная, не возвращается). **Единственная
  протяжка M3:** расширить сигнатуру:

  ```go
  // было: (cur bool, snapshot value.Value, ok bool, err error)
  func (i *Interpreter) EvalMetricCondition(spec *ast.MetricTrigger)
      (cur bool, snapshot value.Value, threshold value.Value, ok bool, err error)
  ```
  Возвращать `threshVal` во всех ветках (на пустой метрике / несравнимых типах — `nil`/`None`,
  объяснение тогда не печатается, это «нет данных», FR-009). Обновить вызов `daemon/metrics.go:39`.

> Поправка путей: файл — `internal/eval/trigger_daemon.go` (НЕ `engine/`); `EvalMetricCondition`
> `:31-52`. Точка fire run — `eval/trigger_run.go:78-92` (естественный якорь explain для run).

### C-5.2. Формат explain (D-C-6: всегда-вкл при срабатывании)

Эмитится **только** в момент fire (редкое событие → малошумно), **до** исполнения тела. Числа
рендерятся `value.String` (`value/repr.go:20`), оператор — `BinOp.String()` (`ast/op.go:35`).

**run** → stdout, одна строка:
```
триггер 'выручка_30д < 3000000' сработал: выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина
```

**serve** → через `d.logf` (`daemon.go:53`), с упоминанием **ребра** (иначе вводит в заблуждение на
тике, где `cur` истина, но fire не произошёл — уже-истина, нет ребра):
```
триггер 'выручка_30д < 3000000' сработал (ребро ложь→истина): выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина
```

> Семантическое различие run/serve обязательно: run = fire-if-true (`fired = metric op threshold`),
> serve = edge (`LastBool ложь → cur истина`). Сообщение serve упоминает ребро; run — нет.

«Где инстанс / история эскалации» (вторая половина наблюдаемости DoD §2) **уже** покрыта `inspect`
(B6): `printInspect` (`inspect.go:85-98`) печатает статус/шаг/переменные/задачи, `inspectTaskLine`
(`:106-117`) — срок + `эскалирована`. M3 `inspect` **не меняет** (минимально-честно); explain
добавляет «почему» в момент fire. `inspect` остаётся engine/eval-free (INV-1, `inspect.go:20-21`).

### C-5.3. Golden-таблица explain (точные строки — замок exact-match)

| Путь | Канал | Строка |
|---|---|---|
| run | `i.out` | `триггер '<имя> <оп> <порог>' сработал: <имя> = <снимок> (снимок) <оп> <порог> (порог) → истина` |
| serve | `logf` | `триггер '<имя> <оп> <порог>' сработал (ребро ложь→истина): <имя> = <снимок> (снимок) <оп> <порог> (порог) → истина` |

`<порог>` и `<снимок>` — `value.String` соответствующих значений (числа без разделителей-подчёркиваний:
`3000000`, не `3_000_000` — подчёркивание есть только в исходнике). `<оп>` — `BinOp.String()`.

### C-5.4. Тест-замки C5

- `TestRunTriggerExplain` / `TestServeTriggerExplain`: golden exact-match строк §C-5.3.
- Замок на **тишину при НЕ-fire**: тик без ребра (уже-истина) → нет новой explain-строки.
- **Инверсия:** не протянуть `threshVal` (вернуть `nil`/мусор) → serve-explain печатает неверный/пустой
  порог → `TestServeTriggerExplain` краснит.
- C5 НЕ вводит новых SE-кодов / eval-кодов (explain — не ошибка).

### C-5.5. Инвентарь golden-churn (ОБЯЗАТЕЛЕН — always-on explain ломает exact-match)

Поскольку explain эмитится при **каждом** срабатывании, существующие exact-match golden'ы, прогоняющие
fire, **получают новую строку** и обязаны быть обновлены co-land с C5, иначе гейт `go test` (§C-1.4 /
§C-8) недостижим. Импл-чат обязан обновить ровно эти (и только эти):

**serve (exact-match `out.String()`, ребро+тело в один writer — `daemon.go:53-58`):**
- `daemon/tick_test.go:109` (`TestTickPhaseOrderAllThreeFire`, ждёт `"E\nM\nS\n"`).
- `daemon/tick_test.go:160` (`TestTickFourPhasesOrder`, ждёт `"E\nM\nS\nD\n"`).

**run (exact-match stdout/`i.out`):**
- `cmd/ladix/trigger_golden_test.go`: `TestRunTriggerFiresGolden`, `TestRunTriggerDBRepeatEphemeral`,
  `TestRunTriggerMultiMetricOrderGolden` (два fire), `TestRunTriggerMixedKindsOrderGolden`,
  `TestRunTriggerBodyReadShadowGolden` (два fire).
- `eval/metric_window_golden_test.go`: `TestWindowMetricTriggerFires` (ждёт `"оконная метрика: 23\n"`
  И `stubs.Len()==0` — explain должен идти в `i.out`, не в `w`, иначе ломает обе проверки).
- `cmd/ladix/golden_test.go`: `TestCLIGoldenMetrics`, `TestCLIGoldenSourceCSV` (витринные `run`-примеры с
  фаером триггера — explain-строка приземляется на каждый `… → истина`). **Добавлено по итогу M3-гейта**
  (CHURN-1): исходный инвентарь их упускал; фактический churn co-land с C5 на них корректен.

**НЕ затронуты (фиксируем явно — не трогать):** count()/contains()-тесты демона (`metrics_test.go`,
`schedule_test.go`, `daemon_test.go` MFIRE, `m2_endtoend` `sink.count`); no-fire/error-тесты
(`source_negatives` `runtimeForceTrigger`, `TestWindowMetricTriggerSilent`, events-FIFO `want=A\nB\nC`).

> Тест-замки §C-5.4 (`TestRunTriggerExplain`/`TestServeTriggerExplain`) — **новые** и фиксируют формат;
> перечисленные выше — **существующие**, обновляются под новую строку. Различать в коммите.

---

## §C-6. Сводка изменений (реестры)

### C-6.1. Контракт Store: 16 → 18

| # | Метод | Тип | Зеркало |
|---|---|---|---|
| 17 | `LoadOutbox(dedupKey string) (*OutboxRecord, error)` | read | `LoadTriggerState` |
| 18 | `SaveOutbox(rec *OutboxRecord) error` | upsert | `SaveTriggerState` |

+ тип `OutboxRecord`, sentinel `ErrOutboxNotFound`, реализации в Memory+SQLite, **двойной** compile-замок.

### C-6.2. ProcessRuntime: 8 → 8 (без изменений)

Дедуп — поведение effect-методов `engine` (`CallExternal`/`CallExternalResult`/`Notify`), сигнатуры
`eval.ProcessRuntime` целы. `activeFrame` получает поле `effectIndex int` (внутреннее, не интерфейс).

### C-6.3. Схема БД: версия 1 → 2

`PRAGMA user_version`: база=1 (схема 006/007/018), миграция 1→2 = таблица `outbox` + индекс.

### C-6.4. Прочее

- `EvalMetricCondition`: +1 возвращаемое значение (`threshold value.Value`) — eval-внутреннее, не
  пересекает швы eval↔engine↔store.
- `evalClockFromEngine`: вынесен из `serve.go` в shared `cmd/ladix`.
- Новые строки вывода: explain (run/serve, §C-5.3); fault-логи `checkDeadlines` уже существуют.
- Новых ключевых слов, SE-кодов, eval-кодов, builtins — **нет**.

---

## §C-7. §5-инварианты под M3 — чек-лист дрейф-аудита (M3-гейт)

1. `eval.ProcessRuntime` = ровно 8 методов, сигнатуры байт-целы; `eval` не импортирует `store`/`engine`
   (grep — zero matches). ✓ ожидаемо.
2. Store: **двойной** compile-замок; +2 метода аддитивны; базовые 16 сигнатур целы; сериализация —
   внутри `SQLiteStore`.
3. Фронтенд не тронут: усиление §2 = существующие конструкции; новых KW/SE/eval-кодов нет; v1-программы
   валидны.
4. Дедуп НЕ меняет границу: контекст из `e.active`, не из нового шва; `ExternalCaller` сигнатура цела.
5. `serve` clock-путь не сломан (`serve_golden_test.go:216` зелёный).
6. Мутпробы §C-2a.4/§C-2b.7/§C-4.3/§C-5.4 — все красят при снятии гарантии.
7. Golden-churn §C-5.5 обновлён co-land с C5 (перечисленные exact-match serve/run тесты гейт-зелёные),
   незатронутые count()/no-fire тесты НЕ тронуты. Без этого гейт `go test` недостижим (не §5-инвариант,
   но условие выхода).

---

## §C-8. Сводка тест-замков на инверсию

| Гарантия | Замок | Мутпроба → красит |
|---|---|---|
| миграция применяется | `TestMigrateFreshDB`/`LegacyV0` | убрать строку миграции 1→2 |
| exactly-once шага | `TestStepEffectExactlyOnceRestart` | снять pre-check дедупа → POST×2 |
| детерминизм индекса | тест двух-эффектов-в-шаге | `effect_index`≡0 → коллизия ключей |
| единые часы | `TestRunClockUnified` и т.п. | вернуть независимый `SystemClock{}` |
| explain-порог | `TestServeTriggerExplain` | не протянуть `threshVal` |
| fault checkDeadlines | 3 fault-теста | (фиксируют поведение; ветка-3 — known window) |

---

## §C-9. Бэклог (явно вне M3 — фиксируем границу)

| Пункт | Почему отложено |
|---|---|
| **C1 хеш-версионирование определений** | дедуп per-(инстанс+шаг+индекс) хеша не требует (D-C-4); хеш — для дрейфа определения между рестартами, не нужен сценарию. |
| **Стабильные ключи триггеров** | ключ `trg-<N>` позиционный → соседний триггер сдвигает индекс → durable `trigger_state` наследует чужой `LastBool` (латентный баг сохранности, **не косметика**). Дёшево вытащить отдельно; не блокирует §2. |
| **C3-(б) демон+HTTP+`--json`** | D-C-3 = вариант (а) CLI+harden; сетевой API переписывает 6-7 CLI-команд (XL). |
| **C4 монотонные часы** | несовместимы с durable RFC3339 (§C-4.4). |
| **C5 `--json` / audit-журнал** | пара к C3-(б)/outbox-аудиту; человеко-explain (D-C-6) самодостаточен для DoD. Audit-журнал = pending-строки outbox (поля `delivered=0`/`created_at` уже заложены в §C-2a.3). |
| **delivered-but-unmarked окно (C2b)** | exactly-once до конца требует идемпотентности **приёмника** (не владеем `crm`); §C-2b.5/D-C-9. |
| **payload-loss окно (D-AU-3)** | эфемерность payload; крах до захвата в durable-переменную (§C-1.4). |
| **checkDeadlines ветка-3 fire-then-persist** | то же окно, что C2b; эскалация — НЕ эффект тела шага → вне outbox; durable-флаг `Escalated` закрывает после commit. |
| **harden конкуренции serve×complete/emit (D-C-3 «а»)** | стресс-тест общего SQLite под `SetMaxOpenConns(1)` + `busy_timeout` — отдельный тест-набор; прод-механика уже верна, не блокирует §2. |
| **`запустить процесс` в теле шага (вложенный спавн)** | НЕ один из 3 дедуплицируемых effect-методов (`вызвать`/`уведомить`); рестарт ре-исполняет тело → ре-спавн дочернего инстанса с новым ID → его эффекты вне outbox = at-least-once. Вне DoD §2 (золотой сценарий не вкладывает спавн в шаг-с-эффектом); зафиксировано M3-гейтом (C2B-NESTED-1). |
| **future-version БД (downgrade)** | `migrate` forward-only поднимает старые БД, но БД с `user_version` > текущей открывается молча (без ошибки/отказа). Сценарий = откат бинаря на более старую версию; вне M3-scope; зафиксировано M3-гейтом (C2A-1). |
| **engine-часы metric-пути не залочены тестом** | инвариант единого источника соблюдён (engine.Clock выводится из того же eval.Clock), но lifecycle-штампы движка на metric-пути не покрыты поведенческим замком (§C-4.3, пункт для metric); низкий риск — `metric` есть read-only вычисление. C4-2. |

---

## §C-10. Карта файлов (стартовые швы — перепроверить против живого кода)

- **C2a:** `store/sqlite.go` — `const ddl` `:23-67`, `const pragmas` `:69-74` (нет `user_version`),
  `NewSQLiteStore` `:79-98` (точка встройки после `db.Exec(ddl)` `:94`, до `return :98`),
  `SetMaxOpenConns(1)` `:89`, `nextCounter` `:308-328` (tx-шаблон).
- **C2b:** `engine/caller.go:18-21` (`ExternalCaller`), `engine/runtime.go:47-62` (effect-методы —
  точка дедупа), `engine/engine.go:249` (`advance`, кадр+`effectIndex`), `:196` (`advanceAfterComplete`),
  `:450` (`ReactivateInstance`), `store/store.go:13-46` (контракт 16, замок `:43-46`),
  events-FIFO `sqlite.go:444-510` (форма таблицы), `daemon/checkdeadlines.go:22` (fault-ветки
  `:38-41`/`:50-53`/`:63-65`), `m2_golden_test.go:234` (`driveServeToNoRepeat` — шаблон гейт-теста).
- **C4:** `engine/clock.go:5-31`, `eval/clock.go:12-27`, `serve.go:32-38` (`evalClockFromEngine`),
  `serve.go:201-223` (паттерн), `start.go:133/138`, `main.go:251/255/277` (run), `:296/319` (metric),
  `:559` (tasks), `emit.go:58`; замок `serve_golden_test.go:216/21-23`.
- **C5:** `eval/trigger_daemon.go:31-52` (`EvalMetricCondition`, протяжка `threshVal`),
  `daemon/metrics.go` (`:39` снимок / `:72` ребро / `:82` ветка `if fired` = точка печати explain),
  `eval/trigger_run.go:78-92` (run fire, писатель `i.out`),
  `inspect.go:85-117` (B6 «где», не меняем), `ast/op.go:35` (`BinOp.String`), `value/repr.go:20`
  (`value.String`).

---

*Конец якоря M3. Источник истины сценария — `docs/v2-charter.md` §2; модель движка/Store —
`docs/engine-model.md`; модель автоматизации — `docs/automation-model.md` (отзыв D-AU-9 — см. doc-sync).*
