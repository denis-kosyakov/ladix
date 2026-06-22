# Контракт: ключ-билдер демона + интеграция (`internal/daemon`)

## `buildTriggerKeys(trig []*ast.TriggerDecl) []string`

**Назначение**: один проход — каноническая строка → группировка дубликатов → 0-based порядковый
номер → FNV-1a-64 → массив контентных ключей, выровненный по индексам `trig` (FR-001/004/005).

**Размещение**: `internal/daemon` — новый `keys.go` ЛИБО рядом с удалённым `triggerID` в `tick.go`.
Импорт `hash/fnv`, `strconv`, `fmt` (stdlib).

**Алгоритм** (нормативен):

```go
func buildTriggerKeys(trig []*ast.TriggerDecl) []string {
    keys := make([]string, len(trig))
    ordinals := map[string]int{}
    for idx, td := range trig {
        c := ast.CanonicalTriggerCondition(td.Spec)
        if c == "" { // событие/дедлайн — слот пуст, не читается
            continue
        }
        ord := ordinals[c]
        ordinals[c]++
        h := fnv.New64a()
        h.Write([]byte(c + "#" + strconv.Itoa(ord)))
        keys[idx] = "trg-" + fmt.Sprintf("%016x", h.Sum64())
    }
    return keys
}
```

**Контракт**:

| Свойство | Гарантия |
|----------|----------|
| Выравнивание | `len(keys) == len(trig)`; `keys[idx]` соответствует `trig[idx]`. |
| Не-ключевые слоты | событие/дедлайн → `keys[idx] == ""` (не читается). |
| Стабильность под перестановкой | уникальное условие → `ord=0` → ключ не зависит от позиции/соседей (SC-001). |
| Дизамбигуация дубликатов | идентичные условия → `ord` 0,1,… → разные ключи (SC-004). |
| Детерминизм | один список → один массив ключей (между прогонами). |
| Формат | `"trg-" + 16 hex-цифр` (`%016x` от `uint64`). |

## Интеграция в структуру/конструктор демона

- **Поле**: добавить `triggerKeys []string` в `type Daemon struct` (`daemon.go:25-33`).
- **Заполнение**: в `New` (`daemon.go:37-49`), после литерала структуры либо в литерале —
  `triggerKeys: buildTriggerKeys(interp.Triggers())`. **Сигнатура `New` НЕ меняется** (`interp` уже
  параметр); call-sites `serve.go:326` и 4 теста — **не трогаются**.

  ```go
  d := &Daemon{ st: st, eng: eng, interp: interp, clock: clock, interval: interval, out: out }
  d.triggerKeys = buildTriggerKeys(interp.Triggers())
  return d
  ```

## Замена call-sites чтения ключа

| Файл:строка | Было | Станет |
|-------------|------|--------|
| `metrics.go:38` | `id := triggerID(idx)` | `id := d.triggerKeys[idx]` |
| `schedule.go:47` | `id := triggerID(idx)` | `id := d.triggerKeys[idx]` |

- `idx` — из `for idx, td := range d.interp.Triggers()` (`metrics.go:33` / `schedule.go:42`).
- **Удалить** `triggerID(idx)` (`tick.go:43-45`) полностью (FR-005, SC-008 — нет захардкоженного
  `trg-%d` в прод-коде).
- `events.go`/`checkdeadlines.go` ключ **не зовут** — не трогаются.

## FR-010: прайм-без-срабатывания `checkAt` (schedule_at)

**Файл**: `schedule.go:105-133`. Добавить miss-ветку **ДО** существующего fire.

**Текущий код** (`:115-123`):
```go
ts, loadErr := d.st.LoadTriggerState(id)
if loadErr != nil && !stderrors.Is(loadErr, store.ErrTriggerStateNotFound) {
    d.logf(...); return // hard-error bail (не трогать)
}
alreadyToday := loadErr == nil && ts.LastFiredDate != nil && *ts.LastFiredDate == today
if alreadyToday || now.Before(target) {
    return
}
// … Save{LastFiredDate:today} + fire
```

**Дельта FR-010** (вставить после hard-error bail, до/в логику alreadyToday):
```go
miss := stderrors.Is(loadErr, store.ErrTriggerStateNotFound)
if miss && !now.Before(target) { // первое наблюдение, цель уже наступила: прайм, НЕ догонять тело
    day := today
    if saveErr := d.st.SaveTriggerState(&store.TriggerState{
        TriggerID: id, Kind: atKind, LastFiredDate: &day,
    }); saveErr != nil {
        d.logf("триггер '%s': сбой записи trigger_state: %s", id, saveErr.Error())
    }
    return // прайм без срабатывания (FR-010)
}
```

**Контракт**:

| Случай | До | После (FR-010) |
|--------|----|----|
| miss && `сейчас >= цель` | Save + **fire** (catch-up) | Save{LastFiredDate:today} + **return** (прайм) |
| miss && `сейчас < цель` | `now.Before(target)`→return | **без изменений** (штатно сработает в target) |
| есть строка, не сегодня, `сейчас >= цель` | Save + fire | **без изменений** (штатное срабатывание) |
| `alreadyToday` | return | **без изменений** |

- Save при прайме **идентичен** прежнему (`Kind: atKind`, `LastFiredDate: &today`); единственная
  разница — **тело не исполняется**.
- `checkEvery` (`schedule.go:58-102`) и метрика (`metrics.go`) **уже** праймят — не трогать.

## Замки (детально — в quickstart.md)

- **T5** (🔁, ЯДРО): стабильность ключа метрики под вставкой несвязанного триггера ПЕРЕД ней.
  🔁: вернуть позиционный `triggerID(idx)` → idx сдвинулся → ключи разные → краснеет.
- **T6**: правка условия → новый ключ → ре-прайминг (нет ложного фронта).
- **T8** (🔁): поведенческая нейтральность первого тика (метрика/`каждые`/`в` праймят, НЕ срабатывают;
  schedule_at с `сейчас>=цель`). 🔁: не привести checkAt → schedule_at срабатывает → краснеет.
- **T9**: паритет durable Memory/SQLite через `eachStore`.
