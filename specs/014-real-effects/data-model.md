# Data Model: B2 «Реальные эффекты `вызвать` / `уведомить` через HTTP-вебхук»

**Feature**: 014-real-effects | **Date**: 2026-06-17 | **Источник**: `docs/automation-model.md` §AU-4 (D-AU-2)

B2 вводит интерфейс драйвера, Option, два драйвера и нейтральный кодек-пакет. Никаких новых типов-значений языка, структур хранилища или persisted-сущностей (пустой дифф `store`). Ниже — структурное описание сущностей и их инвариантов.

## E-1. Интерфейс `ExternalCaller` (`internal/engine/caller.go`)

Драйвер внешних эффектов. Инжектируется Option'ом; единственная абстракция «куда уходит `вызвать`/`уведомить`».

```go
// ExternalCaller — драйвер реальных внешних эффектов вызвать/уведомить (B2, §AU-4.1).
// Дефолт-реализация — printCaller (печать-стаб §EN-7); реальная — webhookCaller (HTTP POST).
type ExternalCaller interface {
    Call(target string, args []value.Value) (value.Value, error)  // вызвать → результат
    Notify(target string, args []value.Value) error               // уведомить → эффект
}
```

**Инварианты**:
- `Call` под дефолт-стабом → `(value.None, nil)`; под HTTP → декодированный ответ (или `Пусто` при пустом теле).
- `Notify` под стабом → печать + `nil`; под HTTP → POST, ответ игнорируется (best-effort).
- Сигнатуры зеркалят методы шва движка `CallExternalResult`/`Notify` (target string, args []value.Value).

## E-2. Option `WithExternalCaller` + поле движка (`internal/engine/clock.go`, `engine.go`)

```go
// engine/clock.go (рядом с Option/WithClock:19-24)
func WithExternalCaller(c ExternalCaller) Option {
    return func(e *Engine) { e.caller = c }
}

// engine/engine.go — поле + дефолт в NewEngine (ДО применения opts):
type Engine struct {
    // … существующие поля …
    caller ExternalCaller
}
// в NewEngine: e.caller = printCaller{out: e.out}   // дефолт ДО opts...
```

**Инварианты**:
- Дефолт ставится ДО применения `opts ...Option` → конструкция БЕЗ `WithExternalCaller` даёт стаб (golden §EN-7 цел).
- DI через Option (Принцип V: без глобалов); `httpClient` — поле `webhookCaller`, не пакет-глобал.

## E-3. Дефолт-драйвер `printCaller` (`internal/engine/caller.go`)

```go
// printCaller — печать-стаб §AU-4.2: переносит текущую печать-логику движка
// (runtime.go:42-73) байт-в-байт. Держит golden §EN-7 (≥6 пинов).
type printCaller struct{ out io.Writer }

func (p printCaller) Call(target string, args []value.Value) (value.Value, error) {
    parts := make([]string, len(args))
    for k := range args { parts[k] = value.String(args[k]) }
    fmt.Fprintf(p.out, "[вызов] %s(%s)\n", target, strings.Join(parts, ", "))
    return value.None, nil
}
func (p printCaller) Notify(target string, args []value.Value) error {
    if len(args) == 0 { fmt.Fprintf(p.out, "[уведомление] %s\n", target); return nil }
    parts := make([]string, len(args))
    for k := range args { parts[k] = value.String(args[k]) }
    fmt.Fprintf(p.out, "[уведомление] %s: %s\n", target, strings.Join(parts, " "))
    return nil
}
```

**Инварианты (байт-точные форматы, §AU-4.2/§EN-7)**:
- `[вызов] %s(%s)\n` — аргументы через `", "`; без аргументов → `[вызов] имя()`.
- `[уведомление] %s\n` — `len(args)==0`, без двоеточия и хвостовых пробелов.
- `[уведомление] %s: %s\n` — `len(args)>=1`, аргументы через один пробел `" "`.
- `value.String(arg)` — тот же `repr` (Длительность → `3дн`).
- НИКОГДА не возвращает ошибку (Call → `(None, nil)`, Notify → `nil`).

## E-4. Реальный драйвер `webhookCaller` (`internal/engine/caller.go`)

```go
// webhookCaller — реальный HTTP-драйвер (§AU-4.3): POST JSON {"цель","данные"} на baseURL.
type webhookCaller struct {
    baseURL    string
    httpClient *http.Client   // конечный Timeout (напр. 10с)
}

func (w webhookCaller) Call(target string, args []value.Value) (value.Value, error) {
    // 1) тело = jsonval.EncodeBody(target, args) → {"цель":target,"данные":[args]}
    // 2) POST baseURL, Content-Type: application/json
    // 3) сетевой/HTTP-сбой → error (eval завернёт в ОшибкаВыполнения, R-5)
    // 4) пустое тело ответа → value.None (проверка ДО decodeValue)
    // 5) иначе jsonval.DecodeValue(resp.Body) → value.Value (объект→Запись, скаляр→Value)
}
func (w webhookCaller) Notify(target string, args []value.Value) error {
    // POST как Call; ответ игнорируется (best-effort); сбой → error
}
```

**Инварианты (§AU-4.3)**:
- Тело запроса — `{"цель": <логическое имя>, "данные": [<args как plain-JSON>]}`, заголовок `Content-Type: application/json`.
- Логическое имя цели — в payload (`.ladix` чист, URL приходит снаружи).
- Пустое тело ответа проверяется ДО `DecodeValue` (`decodeValue` на пустом потоке → ошибка) → `Пусто`.
- `Notify` ответ не декодирует.
- Реальный POST в тестах — только под `net/http/httptest`.

## E-5. Кодек `internal/jsonval` (НОВЫЙ нейтральный пакет)

**Декодер (лифт из `daemon/events.go:95-206`)** — пакетные функции без зависимости на `*Daemon`:

```go
package jsonval
func PayloadToRecord(payload string) (value.Запись, error)   // объект → Запись (B3)
func DecodeValue(dec *json.Decoder) (value.Value, error)     // одно значение любого типа (B2 ответ)
// + decodeObject/decodeArray/numberToValue (внутренние)
```

**Энкодер (НОВЫЙ, value → plain-JSON)** — для тела вебхука:

```go
func EncodeBody(target string, args []value.Value) ([]byte, error)  // {"цель":target,"данные":[...]}
func encodeValue(v value.Value) (json.RawMessage, error)            // НЕтегированный
```

| `value` тип | plain-JSON | прим. |
|---|---|---|
| `Целое` | число | int64 |
| `Дробное` | число | float64 |
| `Строка` | quoted-строка | |
| `Булево` | `true`/`false` | |
| `Пусто` | `null` | |
| `Список` | array | рекурсивно |
| `Запись` | object | ключи в порядке появления |
| `Дата`/`Длительность`/`Период` | строковая форма | **решение impl, задокументировать** (напр. ISO/`value.String`) |

**Почему пакет создаётся (а не вызов на месте)**: `engine` нуждается в `DecodeValue` (ответ вебхука §AU-4.3) + энкодере (тело), но НЕ может импортировать `daemon` — ребро `daemon→engine` уже есть (`daemon/daemon.go:19`, `restart.go:6`), обратный импорт = цикл. Нейтральный `internal/jsonval` импортируем обоими. **Пакет СОЗДАЁТ B2 (014)** (лифт декодера + новый энкодер); **B3 (015) его НЕ создаёт и НЕ лифтит повторно** — только потребитель (`PayloadToRecord` для `--данные`→`Запись`, §AU-5.3).

**Инварианты**:
- Энкодер НЕтегированный (в отличие от `store/codec.encodeValue` `{"т","зн"}` — несовместим с внешней системой).
- `jsonval` импортирует только `value`+stdlib (`bytes`/`encoding/json`/`fmt`/`strings`) → листовой-совместим (Принцип VII).
- Импортёры: `engine` (декод ответа + энкод тела), `daemon` (события, лифтнутый декодер), `cmd/ladix` (B3 `--данные`, вне scope B2). НЕ `eval`, НЕ `internal/value`.
- **Замок лифта (007b golden цел)**: после лифта `daemon/events.go` ДЕЛЕГИРУЕТ `jsonval` (единственный источник декодера, БЕЗ дублирующих функций в `daemon`); событийные тесты декодера 007b в `daemon/events_test.go` (`TestDrainEvents*` + `TestPayloadToRecordValueTypes`) и весь пакет `daemon` остаются ЗЕЛЁНЫМИ (golden событий байт-точен). *Инверсия:* оставить дублирующий декодер в `daemon` ИЛИ сломать делегирование → событийные тесты `daemon` краснеют.
- **Граничный замок (source_loader НЕ сливается)**: второй JSON→value декодер источников M1 (`eval/source_loader.go:119-216`, методы на `*Interpreter`) — ДРУГАЯ семантика (int-overflow → ОШИБКА §SM-9.B, даты не распознаются §9.4) — НЕ переезжает в `jsonval` и НЕ зовёт его. *Инверсия/проверка:* `jsonval` НЕ импортируется из `source_loader`; `eval/source_loader.go` — ПУСТОЙ дифф (не трогается).

## E-6. Делегирование методов движка драйверу (`internal/engine/runtime.go`)

```go
func (e *Engine) CallExternalResult(target string, args []value.Value) (value.Value, error) {
    return e.caller.Call(target, args)
}
func (e *Engine) CallExternal(target string, args []value.Value) error {
    _, err := e.caller.Call(target, args)   // отброс значения (эффект РОВНО один раз)
    return err
}
func (e *Engine) Notify(target string, args []value.Value) error {
    return e.caller.Notify(target, args)
}
```

**Инварианты**:
- `CallExternal` = `Call` с отбросом значения (печать/POST происходит ровно один раз — не дублируется).
- `var _ eval.ProcessRuntime = (*Engine)(nil)` — движок по-прежнему удовлетворяет шву (8 методов, B1).
- Шов `ProcessRuntime` НЕ расширяется (остаётся 8); сигнатуры B1 байт-в-байт.

## E-7. Активация `runtimeErrWrap` (eval-сторона, `internal/eval/stmt.go`)

| Точка | Было | Стало | прим. |
|---|---|---|---|
| `evalCallAction` `:118` | `runtimeErr(c.Pos(), err.Error())` | `runtimeErrWrap(c.Pos(), err)` | + удалить TODO `:113-115` |
| `evalNotifyAction` `:138` | `runtimeErr(n.Pos(), err.Error())` | `runtimeErrWrap(n.Pos(), err)` | + удалить TODO `:133-135` |
| `evalExpr(*CallExternalExpr)` | уже `runtimeErrWrap` (B1) | без изменений | три точки → единая категория |

**Инварианты**:
- Все три точки → `errors.ОшибкаВыполнения` с цепочкой `Cause` (для `errors.As/Is`), ЕДИНАЯ категория сбоя внешнего вызова.
- Под дефолт-стабом ветка ошибки недостижима (стаб → nil) — поведение v1 не меняется.
- Сигнатуры eval/шва не меняются; правка — 2 строки + 2 удалённых комментария.

## E-8. CLI-сущность: источник вебхука (`cmd/ladix`)

```go
// общий хелпер (корень композиции CLI):
func openExternalCaller(webhookFlag string) (engine.ExternalCaller, error) {
    url := webhookFlag
    if url == "" { url = os.Getenv("LADIX_WEBHOOK") }
    if url == "" { return nil, nil }   // → движок берёт дефолт-стаб
    if !validURL(url) {
        return nil, fmt.Errorf("неверный URL вебхука '%s'", url)  // → ladix: <текст>, exit 2
    }
    return webhookCaller{baseURL: url, httpClient: &http.Client{Timeout: …}}, nil
}
```

**Инварианты**:
- Источники активации: флаг `--вебхук` приоритетнее env `LADIX_WEBHOOK`; оба пусты → `nil` → дефолт-стаб.
- Невалидный URL → CLI-ошибка `ladix: неверный URL вебхука '<URL>'` (stderr, exit 2, §AU-10.C); движок не строится, stdout пуст.
- Команды передают результат в `WithExternalCaller` ТОЛЬКО при `c != nil`.
- Под `serve` — результат проводится в ТОТ ЖЕ экземпляр движка, чьи `Notify`/`Call` зовёт догон дедлайнов и тело триггеров (§AU-4.5/§AU-12.C).
- env читается в корне композиции, передаётся параметром (Принцип V: без глобалов).
