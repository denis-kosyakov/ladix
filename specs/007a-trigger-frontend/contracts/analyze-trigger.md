# Контракт: семантический проход триггеров

**Фаза**: 1 (design) | **Якорь**: `docs/trigger-model.md §TR-4, §TR-5` | **Решения**: D-TR-1, D-25 | **FR**: FR-007…011, FR-022, FR-025

> Тело триггера обходится существующим `checkStmts(..., inFunction=false, inStep=false, ...)` (как top-level) плюс **два новых взаимоисключающих флага** `inMetricTrigger`/`inEventTrigger` — этим переиспользуются гарды действий-шага/`вернуть` и добавляется контекст-гард `значение`/`событие`.

## Назначение

Расширение семпрохода `eval` (`internal/eval/analyze.go`) на триггеры: регистрация `TriggerDecl` в `Analyze`, `checkTrigger` (резолв метрики, проверка порога, обход тела по виду), case-ветки `ValueExpr`/`EventExpr` в `checkExpr` с контекст-гардом, переиспользование существующих гардов (действия-шага, арность процесса, `вернуть`). Тип ошибки — `errors.СемантическаяОшибка` через `semErr(pos, msg)`; формат — двухстрочный канон. Точные тексты — diagnostics.md §TR-7.A…D.

**Размещение**: `analyze.go` — ИЗМЕНЯЕТСЯ (регистрация в `Analyze` Шаг 1; `checkTrigger`; флаги в `checkStmt`/`checkStmts`/`checkElse`/`checkExpr`); `interpreter.go` — ИЗМЕНЯЕТСЯ (поле реестра триггеров, если выбран реестр; иначе обход `prog.Items`).

## Регистрация TriggerDecl в Analyze (Шаг 1)

В цикле Шага 1 (`analyze.go:28-73`) `TriggerDecl` **НЕ участвует** в namespace-проверке имён (у триггера нет имени верхнего уровня — `default: continue` его пропускает, как `Statement`). Регистрация — отдельным под-проходом (зеркало Шага 1b метрик / Шага 1c процессов): собрать триггеры и проверить каждый через `checkTrigger`. Реестр триггеров — либо поле `Interpreter` (аналог `i.metrics`/`i.processes`, нужен fire-if-true-проходу §TR-6), либо повторный обход `prog.Items` в run-проходе.

```
// Шаг 1d (НОВЫЙ) — семантическая валидация триггеров (зеркало Шага 1b/1c):
for _, item := range prog.Items:
    if td, ok := item.(*ast.TriggerDecl); ok:
        if err := i.checkTrigger(td); err != nil:
            return err
```

Регистрация в реестр (для §TR-6) — здесь же: `i.triggers = append(i.triggers, td)` в порядке `prog.Items` (детерминизм stdout и порядка процессов, FR-012).

## checkTrigger(td) — диспетчер по виду

```
checkTrigger(td *ast.TriggerDecl):
    switch spec := td.Spec.(type):
      case *ast.MetricTrigger:
          // (1) резолв метрики против i.metrics
          if md, ok := i.metrics[spec.Metric.Name]; !ok:
              // различающий текст: имя занято не-метрикой → TR-MET-NOTMETRIC, иначе TR-MET-UNDECL
              if isDeclaredNonMetric(spec.Metric.Name):
                  return semErr(spec.Metric.Pos(), fmt.Sprintf("'%s' — не метрика", spec.Metric.Name)) // TR-MET-NOTMETRIC
              return semErr(spec.Metric.Pos(), fmt.Sprintf("метрика '%s' не объявлена", spec.Metric.Name)) // TR-MET-UNDECL
          // (8) проверка выражения-порога обычным checkExpr (резолв имён, арность вызовов)
          if err := i.checkExpr(spec.Threshold, vars(нет локалов)); err != nil:
              return err
          // обход тела с inMetricTrigger=true
          return i.checkTriggerBody(td.Body, inMetric=true, inEvent=false)
      case *ast.EventTrigger:
          // событие-имя в 007a НЕ резолвится против реестра (очередь событий — 007b);
          // тело валидируется полностью (FR-022)
          return i.checkTriggerBody(td.Body, inMetric=false, inEvent=true)
      case *ast.ScheduleTrigger:
          // расписание: содержимое строки "ЧЧ:ММ" НЕ анализируется (FR-005, отложено 007b);
          // в теле ни значение, ни событие не допустимы → оба флага false
          return i.checkTriggerBody(td.Body, inMetric=false, inEvent=false)
```

`checkTriggerBody(body, inMetric, inEvent)` вызывает обход тела с `inFunction=false`, `inStep=false`, `loopDepth=0` и двумя новыми флагами:

```
checkTriggerBody(body, inMetric, inEvent):
    return i.checkStmts(body.Stmts, vars{}, /*inFunction*/false, /*inStep*/false, /*loopDepth*/0, inMetric, inEvent)
```

> **Различающий текст метрики** (TR-MET-UNDECL vs TR-MET-NOTMETRIC) — образец `checkMetricDecl` (analyze.go:145-152): если имя резолвится в источник/процесс/функцию (не метрику) → «`X` — не метрика»; иначе «метрика `X` не объявлена». Текст `метрика '<имя>' не объявлена` — зеркало `функция 'f' не объявлена`/`процесс '<P>' не объявлен`.
>
> **Порог** (правая часть сравнения) проверяется как обычное выражение через `checkExpr` (резолв имён вызовов, арность); тип-чек «число vs число» — рантайм (FR-022, зеркало того, что `если`/`пока` тип условия не статизируют до рантайма).

## Сигнатура обхода — два новых флага

Расширить цепочку `checkStmt → checkStmts → checkElse → checkExpr` флагами `inMetricTrigger`/`inEventTrigger` (зеркало существующего `inStep bool`). Оба `false` на top-level/в функции/в шаге; не меняются при нисходе в `если`/`пока`/`для` (как `inStep`).

```go
// было: checkStmts(stmts, vars, inFunction, inStep, loopDepth)
// стало: checkStmts(stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger)
```

`checkExpr` сегодня принимает `(e, vars)` (analyze.go:398) — для гарда `ValueExpr`/`EventExpr` ему нужны флаги вида триггера. Пробросить их в `checkExpr` (новая сигнатура `checkExpr(e, vars, inMetricTrigger, inEventTrigger)`) либо завести узкий `ctx`-struct.

> **TODO-FACT.** FACTPACK показывает в обходе только `inStep bool` + `inFunction bool` + `loopDepth int`. Точное место добавления двух флагов (отдельные параметры vs объединение в маленький `ctx`-struct) импл-чат выберет по факту сигнатур `checkStmt`/`checkStmts`/`checkElse`/`checkExpr`. Архитектурно — два булевых флага, проброшенных как `inStep`.

## Контекст-гард значение/событие — case в checkExpr (проверка 3)

Зеркало гарда действий-шага (analyze.go:365-373). Новые case в `checkExpr`:

```go
case *ast.ValueExpr:
	if !inMetricTrigger {
		return semErr(ex.Pos(), msgValueCtx) // TR-VAL-CTX
	}
	return nil
case *ast.EventExpr:
	if !inEventTrigger {
		return semErr(ex.Pos(), msgEventCtx) // TR-EVT-CTX
	}
	return nil
```

- `значение` (`ValueExpr`) допустимо **только** в теле метрика-триггера (`inMetricTrigger=true`); вне → `TR-VAL-CTX`.
- `событие` (`EventExpr`) допустимо **только** в теле событие-триггера (`inEventTrigger=true`); вне → `TR-EVT-CTX`.
- Доступ `событие.поле` идёт через `FieldExpr.Target` (analyze.go:419-420: `checkExpr(ex.Target)`), который вернёт `*ast.EventExpr` → тот же гард.

Этим закрывается изоляция видов (§TR-5): в событие-теле `значение` → `TR-VAL-CTX`; в метрика-теле `событие` → `TR-EVT-CTX`; в расписание-теле любое из них → соответствующая `TR-*-CTX` (оба флага false).

## Переиспользуемые гарды (без нового кода)

| Конструкция | Механизм | id (diagnostics.md) |
|---|---|---|
| Действия-шага (`AssignAction`/`CallAction`/`NotifyAction`) | существующий гард `inStep=false` (analyze.go:365-373) → §PM-6.B текст | §TR-7.C TR-ACTION-CTX |
| `запустить процесс P(args)` (`RunProcessExpr`) | существующий `checkRunProcess` (analyze.go:471-494): args-first, резолв против `i.processes`, арность | §TR-7.D TR-PROC-* (4 текста) |
| Голый `вернуть` вне функции (`ReturnStmt`) | существующий гард `inFunction=false` (analyze.go:323-332): `inStep=false` → базовый текст без суффикса | §TR-7.D `'вернуть' допустимо только внутри функции` |
| `прервать`/`продолжить` вне цикла | существующий гард `loopDepth==0` (analyze.go:337-342) | (существующие) |

Тело триггера семантически тождественно top-level по набору разрешённых конструкций (императивное ядро минус действия-шага); единственное расширение поверх top-level — `значение`/`событие` через флаги вида.

## Таблица 6 проверок (§TR-4)

| № | проверка | механизм / прецедент | id диагностики |
|---|---|---|---|
| 1 | метрика в условии объявлена | реестр `i.metrics`, образец `analyze.go:145-152, 487` | `TR-MET-UNDECL`, `TR-MET-NOTMETRIC` |
| 2 | формат `"ЧЧ:ММ"` в `в` | **ОТЛОЖЕНО в 007b** — содержимое строки в семпроходе не анализируется (§TR-11 п.3) | — (нет в 007a) |
| 3 | контекст-гард значение/событие | новые case `ValueExpr`/`EventExpr`, флаги `inMetricTrigger`/`inEventTrigger` | `TR-VAL-CTX`, `TR-EVT-CTX` |
| 4 | тело запрещает действия-шага | существующий гард `inStep=false`, `analyze.go:365-373` | (существующая §PM-6.B) |
| 5 | арность запускаемого процесса | существующий `checkRunProcess`, `analyze.go:471-494` | (существующие §PM-6.C) |
| 6 | «одно сравнение» (лимит v1) | **структурное** ограничение грамматики (parser-seams.md, шов A) — НЕ семантическая проверка | — |

- **№2** — формат `ЧЧ:ММ` ОТЛОЖЕН 007b: язык не валидирует содержимое строковых литералов на семпроходе; значение расписания в `run` не используется (no-op). В реестре 007a диагностики формата времени НЕТ (diagnostics.md §TR-7.E).
- **№6** — «одно сравнение» **структурно, без диагностики**: грамматика парсит условие плоско (`метрика Ident CompOp Expression`), составное `X<Y и Z>W` физически не собирается → попытка дописать `и …` упрётся в `expect(COLON)` → синтаксическая ошибка про ожидаемое `:` (FR-021), а не семантическая.

## Контракт scope тела (§TR-5)

| Категория имён | Видимость | Мутируемость |
|---|---|---|
| `значение` (метрика-тело) | видно | **read-only** (инжектируется движком на момент срабатывания, §TR-6) |
| `событие` (событие-тело) | видно | **read-only** (инжектируется при доставке события — 007b; в `run` 007a не инжектируется) |
| Глобальные `пусть` | видно | **read-only** из тела |
| Метрики (по имени) | видно | **read-only** (резолв `i.metrics`, ленивое вычисление) |
| Функции, процессы | видно | вызов / запуск |
| Локальные `пусть` тела | видно | **read/write локально** (эфемерны, исчезают по завершении прохода) |

**Порядок Define**: (1) глобалы уже связаны (проход триггеров идёт ПОСЛЕ `interp.Run`, §TR-6); (2) инжекция предопределённого имени в **локальный** env тела (`env.Define("значение", <число>)`); (3) локальные `пусть` по ходу исполнения. Предопределённое имя в локальном env не протекает наружу; коллизия с глобальным `пусть` того же имени невозможна — `значение`/`событие` это жёсткие KW, не Ident.

Событие-тело в 007a **валидируется полностью** (контекст-гард `событие`, запрет действий-шага, арность процессов), хотя в `run` не исполняется (FR-022) — фронтенд готов, исполнение отложено в 007b.

## Чего нет

- **Валидация строки расписания** (`ЧЧ:ММ`: ведущий ноль, диапазон 00–23:00–59) — отложено 007b (§TR-11 п.3); в 007a содержимое `AtSchedule.At` семпроходом не читается.
- **Тип-чек «число vs число»** в условии метрики — рантайм (FR-022), не семпроход.
- **Резолв имени события** против реестра — нет реестра событий в 007a (очередь `events` — 007b); событие-имя только синтаксически разбирается.
- **Declaredness плоского `Ident`** в позиции значения — как и в 003, рантайму (analyze.go:397 «Плоский Ident НЕ проверяется»).
- **Edge-детект / durable-состояние** — 007b; семпроход состояния триггера не касается.
