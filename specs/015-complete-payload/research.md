# Research: B3 payload через `complete --данные`

Якорь: `docs/automation-model.md` §AU-5 (B3), §AU-1 D-AU-3, §AU-10.C. Эмпирика — на
`master` HEAD `aebac92` (B1+B2 влиты), дерево чисто.

## R1 — Путь завершения задачи (прецедент Complete/catchUp/advance)

Файл `src/internal/engine/engine.go` (на `aebac92`):

- `func (e *Engine) Complete(taskID string) (CompleteResult, error)` — **:108**.
  Грузит задачу/инстанс, гарды Q3/D-8/D-12. Две ветки догона зовут `catchUp` (:140,
  :164); прямой путь → `advanceAfterComplete(inst, false)` (:170).
- `func (e *Engine) catchUp(inst *store.ProcessInstance, t *store.Task) (CompleteResult, error)`
  — **:177**. Хвост сбойного окна D-4 (`caughtUp=true`); печатает строку 8, зовёт
  `advanceAfterComplete(inst, true)` (:183).
- `func (e *Engine) advanceAfterComplete(inst *store.ProcessInstance, caughtUp bool)
  (CompleteResult, error)` — **:189**. Терминал → `выполнен`; иначе `inst.CurrentStep
  = next` и `advance(inst)` (:202).
- `func (e *Engine) advance(inst *store.ProcessInstance) error` — **:242**. Строит
  `processEnv := eval.NewEnvironment(...)` (:243), кладёт `inst.Variables` (:244-246),
  крутит цикл шагов (:252-318). В цикле: `stepEnv := eval.NewEnvironment(processEnv)`
  (**:262**), фаза атрибутов, затем `e.interp.ExecStepBody(processEnv, stepEnv,
  step.Body)` (**:275**). Человеческий шаг → создаёт задачу, `return nil` (засыпание,
  :306); авто-шаг → `inst.CurrentStep = next`, следующая итерация.

**Вывод**: протяжка `data` точно по §AU-5.3 — все 4 функции, точка `Define` между
:262 (создание `stepEnv`) и :275 (`ExecStepBody`).

## R2 — Точка инжекта и эфемерность (§AU-5.3, закрыто)

- `данные` инжектится в **per-step** `stepEnv` (`stepEnv.Define("данные", cur)`), НЕ
  в `processEnv`. Причина: `processEnv` материализуется в `inst.Variables` через хук
  `присвоить`/персист — попадание `данные` туда нарушило бы эфемерность.
- Механизм «только первый шаг» (закрыт, не гадать): локальная `cur := data` перед
  циклом; инжект `cur`; после первой итерации `cur = value.NewRecord(nil, nil)`
  (пустая `Запись`). Второй+ шаг догона видит пустую `Запись`.
- Вне догона `данные` отсутствует → следующий `complete` без `--данные` видит пустую
  `Запись` (`данные.итог` → Пусто, открытая-запись семантика value.Запись).

## R3 — Read-only барьер `данные`

§AU-5.3: read-only «как тело триггера» — чтение `данные.поле` разрешено,
`присвоить данные = …` запрещён. Прецедент — env-`boundary` тела триггера
(007a/007b, `environment.go`, FR-025), уже существующий механизм. B3 ОПИРАЕТСЯ на
него (Define в stepEnv даёт чтение; переприсвоение блокируется тем же барьером
read-only env, что и тело триггера). Нового барьера НЕ вводим.

## R4 — jsonval уже существует (scope СУЖЕН)

`src/internal/jsonval/decode.go:31`:
`func PayloadToRecord(payload string) (value.Запись, error)` — экспортирован.
Поведение (проверено):
- пустой/пробельный payload → пустая `Запись`, **без ошибки** (:32-34);
- верхний уровень обязан быть `{` — иначе `fmt.Errorf("payload не является
  JSON-объектом")` (:42-46) → не-объект/массив отвергнут;
- `dec.UseNumber()`: число без `.eE` → `Целое`, int64-overflow → `Дробное`; `null`
  → Пусто; вложенный объект → `Запись`; массив → `Список` (через `decodeObject`/
  `decodeValue`/`decodeArray`).

Пакет создан в B2/014 (декодер лифтнут из daemon, энкодер написан). **B3 — чистый
ПОТРЕБИТЕЛЬ**: вызывает `jsonval.PayloadToRecord` из `cmd/ladix`; НЕ создаёт, НЕ
лифтит, НЕ дублирует. Импортёры jsonval уже включают `cmd/ladix` намерением (док
`decode.go:1-9` прямо называет «B3 (015) потребит PayloadToRecord»).

## R5 — CLI complete: прецедент флагов (зеркало --вебхук)

`src/cmd/ladix/main.go`:
- `completeMain(rest []string, …) int` — **:333**. Цикл парса флагов: `--max-depth`,
  `--db`, `--вебхук` (формы `X` и `X=`), `-`-префикс → «неизвестный флаг», иначе
  позиционные. Ровно 2 позиционных (файл, id) → иначе usage exit 2 (:385-388).
  `--вебхук` без значения → `ladix: флаг --вебхук требует значение` exit 2 (:370-372).
- `completeTask(path, taskID, dbPath, maxDepth, caller, …) int` — **:402**. Читает
  файл, лексер→парсер→Analyze под guard, собирает Engine, `eng.Complete(taskID)`
  (:432). guard/recover-барьер обёрнут (:421).

**Вывод**: `--данные` добавляется в `completeMain` зеркально `--вебхук` (формы `X`/
`X=`, «требует значение» exit 2); декод+валидация в `completeTask` ПЕРЕД
`eng.Complete`; невалидный → `ladix: неверный JSON в --данные: <деталь>` exit 2.

## R6 — ProcessRuntime = 8 методов (не шов)

`src/internal/eval/runtime.go`: интерфейс ровно 8 методов (`StartProcess`,
`AssignProcessVar`, `CallExternal`, `CallExternalResult`, `Notify`,
`InstanceStatus`, `InstanceVariables`, `UserTasks`). Протяжка `data` — во
ВНУТРЕННИХ методах `*Engine` (`Complete`/`catchUp`/`advanceAfterComplete`/`advance`),
НЕ в интерфейсе. Шов не растёт.

## R7 — Импорт-граф (инвариант хартии §5/VII)

`internal/eval` импортирует только `ast`+`value` (+ свои). `данные` приходит в eval
как готовая `value.Запись` через `stepEnv.Define` — eval НЕ узнаёт про jsonval/JSON.
Декод — на корне композиции `cmd/ladix` (импорт jsonval допустим). Цикла нет.

## Открытые вопросы

Нет. Решения залочены §AU-1 (D-AU-3) владельцем 2026-06-16; имена/строки —
эмпирически сверены на `aebac92`.
