# Контракт: семпроход процесса (`internal/eval/analyze.go`)

**Фаза**: 1 (design) | **Якорь**: `docs/process-model.md §PM-4/§PM-5/§PM-6` | **Решения**: D-3, D-4,
D-5, D-9, D-10, D-11, D-12

> Канон правил/текстов — §PM-4 (семпроход) + §PM-6 (реестр) + §PM-5 (граница deferred); этот контракт
> переписывает их в файлово-конкретные правки с проверенными координатами (сверены с `eval/analyze.go`
> 10.06.2026). При расхождении побеждает якорь. Все тексты — байт-точно (конституция VIII). Новых
> типов ошибок нет: `СемантическаяОшибка` (`semErr`, D-3).

## Реестр процессов

- `Interpreter` (`interpreter.go:17-35`): добавить поле `processes map[string]*ast.ProcessDecl`
  (рядом с `sources`/`metrics`).
- `NewInterpreter` (`interpreter.go:53-73`): `processes: make(map[string]*ast.ProcessDecl)`.

## Порядок `Analyze` (итог, §PM-4)

Шаг1 регистрация (функции/источники/метрики/**процессы**) → Шаг1b `checkMetricDecl` → **Шаг1c
`checkProcessDecl`** → Шаг2 глобальная область → Шаг2b тела функций. Реестр процессов готов с Шага1 →
`checkRunProcess` работает в любой области.

## Шаг 1 — регистрация (`analyze.go:25-62`, D-5)

Добавить `case *ast.ProcessDecl` в **оба** type-switch'а:
- Первый (`analyze.go:29-38`): `name, pos = d.Name.Name, d.Name.Pos()` (НЕ `isFunc`; общий
  глобальный namespace — повтор даёт общий `'<имя>' уже объявлено в строке N`).
- Второй (`analyze.go:49-62`): `i.processes[name] = d`; `i.checkReservedDeclName(name, d.Pos())`
  (`analyze.go:141-151` — переиспользуется: запрет столкновения со встроенной/периодом).

Тексты (§PM-6.B): `'<имя>' уже объявлено в строке N`; `имя '<имя>' зарезервировано встроенной
функцией`; `имя '<имя>' зарезервировано предопределённым периодом`.

## Шаг 1c — `checkProcessDecl(pd)` (новый, после `analyze.go:76`)

Цикл по `prog.Items`: `pd, ok := item.(*ast.ProcessDecl)` → `i.checkProcessDecl(pd)`. Внутри
(fail-fast, порядок):

1. **Уникальность шагов** (D-5): `имя → строка` первого; повтор →
   `semErr(step.Name.Pos(), fmt.Sprintf("шаг '%s' уже объявлен в строке %d", name, line))`.
2. **Резолв `после`** (D-4, валидатор): для шага `S` на индексе `i`, каждый `X ∈ S.After`:
   - `X` не среди шагов → `semErr(Xident.Pos(), fmt.Sprintf("шаг '%s' после '%s', но шаг '%s' не
     объявлен", S, X, X))`.
   - `X` на индексе `j >= i` → `semErr(Xident.Pos(), fmt.Sprintf("шаг '%s' после '%s', но '%s'
     объявлен позже", S, X, X))`.
   - Ацикличность — автоматически (ссылка строго назад `j < i`).
3. **`срок`-без-`исполнитель`** (§11.4): `Attrs.DeadlinePos.Line != 0 && Attrs.AssigneePos.Line == 0`
   → `semErr(step.Attrs.DeadlinePos, fmt.Sprintf("шаг '%s': срок без исполнитель не имеет эффекта",
   name))`.
4. **`analyzeStep(step, pd.Params)`** (анализ тела).

## `analyzeStep(step, params)` (новый, D-12, образец `analyzeArea` `analyze.go:156-167`)

```go
vars := map[string]bool{}
letLine := map[string]int{}
for _, p := range params {
	vars[p.Name] = true       // засев параметрами (чтение/вызов → рантайм, не «не объявлено»)
	// letLine параметрами НЕ засевается — пусть x с именем параметра в шаге разрешён (теняет, §6.4)
}
if err := collectVars(step.Body, letLine, vars); err != nil { return err } // дубль шаг-локальных пусть/для
return i.checkStmts(step.Body, vars, false /*inFunction*/, true /*inStep*/, 0 /*loopDepth*/)
```

## Прокинуть `inStep bool`

Добавить параметр в `checkStmts` (`analyze.go:222`), `checkStmt` (`analyze.go:231`), `checkElse`
(`analyze.go:281`); все рекурсивные вызовы передают полученный `inStep` без изменения. `analyzeArea`
(`analyze.go:166`) → `inStep=false`; `analyzeStep` → `inStep=true`. **`checkExpr` НЕ трогать.**

## Контекст-гард действий (заменить `analyze.go:275-276`, D-11)

```go
case *ast.AssignAction, *ast.CallAction, *ast.NotifyAction:
	if !inStep {
		return semErr(st.Pos(), fmt.Sprintf("действие '%s' допустимо только в шаге процесса", constructName(st)))
	}
	return nil // в шаге валидно; payload (Args/Value) НЕ обходится; рантайм-deferred (stmt.go:64, недостижимо)
```

`constructName` (`interpreter.go:150-164`) уже даёт `присвоить`/`вызвать`/`уведомить`. Текст §PM-6.B:
`действие '<имя>' допустимо только в шаге процесса`.

## `вернуть` в шаге (обновить `analyze.go:239-242`, §7.3)

```go
case *ast.ReturnStmt:
	if !inFunction {
		msg := "'вернуть' допустимо только внутри функции"
		if inStep { msg += "; в шаге процесса используйте 'присвоить'" }
		return semErr(st.Pos(), msg)
	}
	if st.Value != nil { return i.checkExpr(st.Value, vars) }
	return nil
```

`прервать`/`продолжить` в шаге — по `loopDepth`, не трогать. Текст §PM-6.B: `'вернуть' допустимо
только внутри функции; в шаге процесса используйте 'присвоить'`.

## Арность `запустить процесс` (заменить `analyze.go:330-331`, D-10)

В `checkExpr`: `case *ast.RunProcessExpr: return i.checkRunProcess(ex, vars)`. Новый метод:

```go
func (i *Interpreter) checkRunProcess(r *ast.RunProcessExpr, vars map[string]bool) error {
	for _, a := range r.Args {                 // args-first, fail-fast (как checkCall)
		if err := i.checkExpr(a, vars); err != nil { return err }
	}
	name := r.Process.Name
	if pd, ok := i.processes[name]; ok {        // резолв ТОЛЬКО против процессов (D-10)
		if len(r.Args) != len(pd.Params) {
			return semErr(r.Pos(), fmt.Sprintf("'%s' принимает %d аргументов, передано %d", name, len(pd.Params), len(r.Args)))
		}
		return nil
	}
	if _, ok := i.funcs[name]; ok { return semErr(r.Pos(), fmt.Sprintf("'%s' — функция, не процесс", name)) }
	if _, ok := i.metrics[name]; ok { return semErr(r.Pos(), fmt.Sprintf("'%s' — не процесс", name)) }
	if _, ok := i.sources[name]; ok { return semErr(r.Pos(), fmt.Sprintf("'%s' — не процесс", name)) }
	return semErr(r.Pos(), fmt.Sprintf("процесс '%s' не объявлен", name))
}
```

- `i.builtins` НЕ проверяется (имя встроенной → общий `процесс '<P>' не объявлен`, §PM-4).
- `checkRunProcess` НЕ принимает `inStep` (работает в любой области). Тексты §PM-6.C.
- `DurationLit` (`analyze.go:332-333`) — НЕ трогать (D-11).

## Граница deferred — eval НЕ трогать (§PM-5, критично)

`stmt.go:63-64` (`AssignAction`/`CallAction`/`NotifyAction` → `deferredConstruct`) и `expr.go:48-51`
(`RunProcessExpr`/`DurationLit` → `deferredConstruct`) — **корректны для 005, НЕ трогать**.
Наблюдаемая рантайм-граница 005 = top-level `запустить процесс`; тело шага в рантайме недостижимо
(`ProcessDecl` — `Decl`, `Run()` пропускает). **Недостижимый тест «действие в шаге → deferred» НЕ
писать.**

## Тесты (образец `eval/analyze_decl_test.go`)

Exact-match §PM-6.B/C: уник шагов; `после` вперёд/неизвестный; `срок`-без-`исполнитель`; действие
вне шага (`присвоить x=1` на top-level); `вернуть 1` в шаге; арность (4 текста: принимает N/функция-
не-процесс/не-процесс/не объявлен). Позитив: действие в шаге → чисто; чтение параметра в шаге → без
«не объявлено»; `после A` назад → чисто; `пусть id = запустить процесс P("…")` арность 1==1 → чисто.
