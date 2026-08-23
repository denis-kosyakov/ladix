package engine_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// TestCompleteNoPayloadRegress — Замок e (T003): существующий путь Complete с пустой
// Записью даёт прежний §EN-7 вывод/статусы — поведение без --data байт-идентично
// доB3. Anchor против любого изменения вывода/статусов на no-payload пути.
// Мутпроба: изменить печать/статус на complete-пути → красный.
func TestCompleteNoPayloadRegress(t *testing.T) {
	_, st, eng, out := buildStack(t, twoHumanSrc, goldenMoment())
	if _, err := eng.Start("p", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	out.Reset()
	res, err := eng.Complete("t-000001", emptyRec())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.CaughtUp {
		t.Errorf("CaughtUp=true на штатном пути")
	}
	want := "" +
		"задача t-000001 завершена\n" +
		"[задача] t-000002 → Петров, шаг 'второй'\n" +
		"инстанс p-000001: ожидает, шаг 'второй'\n"
	if got := out.String(); got != want {
		t.Errorf("вывод без payload изменился (регресс):\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
	inst, _ := st.LoadInstance("p-000001")
	if inst.Status != store.StatusWaiting || inst.CurrentStep != "второй" {
		t.Errorf("статус=%q шаг=%q, хотим ожидает/второй", inst.Status, inst.CurrentStep)
	}
}

// payloadFirstSrc — человеческий шаг, затем АВТО-шаг догона, читающий данные.итог и
// сохраняющий его в переменную процесса «факт». После Complete первой задачи авто-шаг
// исполняется как ПЕРВЫЙ шаг догона и видит payload (§AU-5.3, US1).
const payloadFirstSrc = `процесс заявка(x):
    шаг подача:
        исполнитель: "клиент"
    шаг решение после подача:
        присвоить факт = данные.итог

пусть id = запустить процесс заявка(1)
`

// TestCompleteFirstStepSeesPayload — Замок a (T007): первый авто-шаг догона видит
// данные.итог; присвоить факт = данные.итог → факт == "готово".
// Мутпроба: убрать stepEnv.Define(payloadName, cur) → факт пусто → красный.
func TestCompleteFirstStepSeesPayload(t *testing.T) {
	_, st, eng, _ := buildStack(t, payloadFirstSrc, goldenMoment())
	if _, err := eng.Start("заявка", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := eng.Complete("t-000001", recOf([2]string{"итог", "готово"})); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	inst, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if got := value.String(inst.Variables["факт"]); got != "готово" {
		t.Fatalf("Variables[факт]=%q, хотим \"готово\" (payload не дошёл до первого шага)", got)
	}
}

// payloadTypesSrc — авто-шаг читает вложенный объект/число из payload и сохраняет в
// переменные процесса (семантика jsonval: число→Целое, вложенный объект→Запись).
const payloadTypesSrc = `процесс типы(x):
    шаг подача:
        исполнитель: "клиент"
    шаг разбор после подача:
        присвоить сумма = данные.сумма
        присвоить имя = данные.клиент.имя

пусть id = запустить процесс типы(1)
`

// TestPayloadTypesInStep — T008: payload {"сумма":2500000,"клиент":{"имя":"А"}} →
// шаг видит Целое и вложенную Запись (семантика jsonval, не дубль декода).
func TestPayloadTypesInStep(t *testing.T) {
	_, st, eng, _ := buildStack(t, payloadTypesSrc, goldenMoment())
	if _, err := eng.Start("типы", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	payload := value.NewRecord(
		[]string{"сумма", "клиент"},
		map[string]value.Value{
			"сумма": value.Целое{V: 2500000},
			"клиент": value.NewRecord([]string{"имя"},
				map[string]value.Value{"имя": value.Строка{V: "А"}}),
		},
	)
	if _, err := eng.Complete("t-000001", payload); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	inst, _ := st.LoadInstance("p-000001")
	sum, ok := inst.Variables["сумма"].(value.Целое)
	if !ok || sum.V != 2500000 {
		t.Fatalf("Variables[сумма]=%v, хотим Целое 2500000", inst.Variables["сумма"])
	}
	if got := value.String(inst.Variables["имя"]); got != "А" {
		t.Fatalf("Variables[имя]=%q, хотим \"А\" (вложенный объект не дошёл)", got)
	}
}

// catchUpPayloadSrc — после Complete задача УЖЕ помечена завершённой, но инстанс ещё
// ожидает на том же шаге (хвост сбойного окна D-4): второй Complete идёт веткой
// catchUp(inst, data, t). Авто-шаг догона читает данные.метка.
const catchUpPayloadSrc = `процесс догон(x):
    шаг подача:
        исполнитель: "клиент"
    шаг авто после подача:
        присвоить факт = данные.метка

пусть id = запустить процесс догон(1)
`

// TestPayloadThroughCatchUp — T009: ветка catchUp (caughtUp=true) тоже доносит данные
// до первого шага догона. Мутпроба: не прокинуть data через catchUp → факт пусто.
func TestPayloadThroughCatchUp(t *testing.T) {
	_, st, eng, out := buildStack(t, catchUpPayloadSrc, goldenMoment())
	if _, err := eng.Start("догон", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Симулируем сбойное окно D-4: задача помечена завершённой ВРУЧНУЮ, инстанс ещё
	// ожидает на 'подача' → Complete пойдёт веткой catchUp (t.Status == завершена).
	if err := st.MarkTaskCompleted("t-000001", goldenMoment()); err != nil {
		t.Fatalf("MarkTaskCompleted: %v", err)
	}
	res, err := eng.Complete("t-000001", recOf([2]string{"метка", "X"}))
	if err != nil {
		t.Fatalf("Complete (catchUp): %v", err)
	}
	if !res.CaughtUp {
		t.Fatalf("ожидали ветку catchUp (CaughtUp=true), out=%q", out.String())
	}
	inst, _ := st.LoadInstance("p-000001")
	if got := value.String(inst.Variables["факт"]); got != "X" {
		t.Fatalf("Variables[факт]=%q, хотим \"X\" (payload не дошёл через catchUp)", got)
	}
}

// twoAutoSrc — человеческий шаг, затем ДВА авто-шага догона, оба читают данные.метка.
// Первый видит payload, второй — пустую Запись (эфемерность §AU-5.3, US2).
const twoAutoSrc = `процесс эфем(x):
    шаг подача:
        исполнитель: "клиент"
    шаг первый_авто после подача:
        присвоить ш1 = данные.метка
    шаг второй_авто после первый_авто:
        присвоить ш2 = данные.метка

пусть id = запустить процесс эфем(1)
`

// TestSecondStepSeesEmpty — Замок b-1 (T011): из двух авто-шагов догона первый видит
// payload ("X"), второй — пустую Запись (данные.метка → Пусто).
// Мутпроба: убрать cur = value.NewRecord(nil,nil) → второй тоже видит "X" → красный.
func TestSecondStepSeesEmpty(t *testing.T) {
	_, st, eng, _ := buildStack(t, twoAutoSrc, goldenMoment())
	if _, err := eng.Start("эфем", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := eng.Complete("t-000001", recOf([2]string{"метка", "X"})); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	inst, _ := st.LoadInstance("p-000001")
	if got := value.String(inst.Variables["ш1"]); got != "X" {
		t.Fatalf("Variables[ш1]=%q, хотим \"X\" (первый шаг должен видеть payload)", got)
	}
	// Второй авто-шаг видит пустую Запись → данные.метка == Пусто (открытая запись).
	v2, ok := inst.Variables["ш2"]
	if !ok {
		t.Fatalf("ш2 не присвоена (второй шаг не исполнился)")
	}
	if _, isNone := v2.(value.Пусто); !isNone {
		t.Fatalf("Variables[ш2]=%v (тип %s), хотим Пусто — второй шаг видит payload (эфемерность нарушена)", v2, v2.TypeName())
	}
}

// TestPayloadNotPersisted — Замок b-2 (T012): после Complete с payload перечтение
// инстанса НЕ содержит поля «данные»; маппинг payload сохраняется только в ЯВНЫЕ
// переменные (факт), не как «данные». Мутпроба: processEnv.Define вместо stepEnv ИЛИ
// персист payload → в Variables появляется «данные» → красный.
func TestPayloadNotPersisted(t *testing.T) {
	_, st, eng, _ := buildStack(t, payloadFirstSrc, goldenMoment())
	if _, err := eng.Start("заявка", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := eng.Complete("t-000001", recOf([2]string{"итог", "да"})); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	inst, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if _, leaked := inst.Variables["данные"]; leaked {
		t.Fatalf("payload утёк в Variables как «данные»: %v (эфемерность/инжект в processEnv)", inst.Variables["данные"])
	}
	// Ни одно поле инстанса не несёт payload-структуру: «факт» содержит лишь
	// извлечённое значение (да), а самого payload-объекта нет.
	for k, v := range inst.Variables {
		if _, isRec := v.(value.Запись); isRec && k != "x" {
			t.Fatalf("в Variables просочилась Запись payload под ключом %q: %v", k, v)
		}
	}
	// White-box (D-AU-3): схема Store не несёт payload-поля. reflect.NumField фиксирует
	// форму ProcessInstance(7)/Task(9); добавление payload-колонки → красный. Task=9 после
	// 016 B4b: +поле Escalated (bool, durable-флаг эскалации, D-AU-5) — НЕ payload (страж
	// против payload-Записи — цикл по Variables выше). B3 payload по-прежнему не несёт поля.
	if n := reflect.TypeOf(store.ProcessInstance{}).NumField(); n != 7 {
		t.Fatalf("store.ProcessInstance имеет %d полей, хотим 7 (B3 НЕ добавляет payload-поле)", n)
	}
	if n := reflect.TypeOf(store.Task{}).NumField(); n != 9 {
		t.Fatalf("store.Task имеет %d полей, хотим 9 (B3 НЕ добавляет payload-поле; 9-е = Escalated 016 B4b)", n)
	}
}

// TestPayloadStructHasNoField — white-box: структуры store.Task / store.ProcessInstance
// НЕ имеют payload-поля (эфемерность на уровне схемы, D-AU-3). Сторож против добавления
// поля: тест компилируется только пока поля «данные»/«payload» отсутствуют (читаем
// существующие поля; добавление нового мы здесь не ловим компиляцией, но фиксируем
// инвариант перечтения — payload не воскрешается из Store).
func TestPayloadNotResurrectedOnReload(t *testing.T) {
	_, st, eng, _ := buildStack(t, payloadFirstSrc, goldenMoment())
	if _, err := eng.Start("заявка", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := eng.Complete("t-000001", recOf([2]string{"итог", "да"})); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	// Двойное перечтение (имитация рестарта/повторной загрузки) — payload не возникает.
	for i := 0; i < 2; i++ {
		inst, err := st.LoadInstance("p-000001")
		if err != nil {
			t.Fatalf("LoadInstance #%d: %v", i, err)
		}
		if _, leaked := inst.Variables["данные"]; leaked {
			t.Fatalf("перечтение #%d воскресило payload «данные»", i)
		}
	}
	_ = store.StatusDone
}

// noFlagSrc — авто-шаг читает данные.итог в факт; при Complete без payload (пустая
// Запись) данные.итог → Пусто, шаг исполняется штатно (НЕ ошибка).
const noFlagSrc = payloadFirstSrc

// TestNoFlagEmptyRecord — Замок c (T013, engine-уровень): Complete с пустой Записью →
// данные.итог == Пусто, факт == Пусто, инстанс штатно завершается (не ошибка/паника).
// Мутпроба: без payload → ошибка/nil-паника → красный.
func TestNoFlagEmptyRecord(t *testing.T) {
	_, st, eng, _ := buildStack(t, noFlagSrc, goldenMoment())
	if _, err := eng.Start("заявка", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := eng.Complete("t-000001", emptyRec()); err != nil {
		t.Fatalf("Complete без payload вернул ошибку: %v (хотим штатный путь)", err)
	}
	inst, _ := st.LoadInstance("p-000001")
	v, ok := inst.Variables["факт"]
	if !ok {
		t.Fatalf("факт не присвоена (авто-шаг не исполнился без payload)")
	}
	if _, isNone := v.(value.Пусто); !isNone {
		t.Fatalf("Variables[факт]=%v, хотим Пусто (данные.итог из пустой Записи)", v)
	}
}

// roSrc — тело авто-шага пытается переприсвоить сам payload: присвоить данные = ….
const roSrc = `процесс ро(x):
    шаг подача:
        исполнитель: "клиент"
    шаг попытка после подача:
        присвоить данные = "взлом"

пусть id = запустить процесс ро(1)
`

// TestPayloadReadOnly — T014: присвоить данные = … запрещён (read-only payload, §AU-5.3).
// Движок отвергает запись через AssignProcessVar(payloadName) → ОшибкаВыполнения шага,
// инстанс провален (D-14), payload НЕ персистирован. Мутпроба: снять guard → присвоение
// проходит и «данные» персистируется → красный.
func TestPayloadReadOnly(t *testing.T) {
	_, st, eng, _ := buildStack(t, roSrc, goldenMoment())
	if _, err := eng.Start("ро", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err := eng.Complete("t-000001", recOf([2]string{"итог", "ok"}))
	if err == nil {
		t.Fatalf("Complete: ожидали ошибку read-only (присвоить данные = …), получили nil")
	}
	if !strings.Contains(err.Error(), "только для чтения") {
		t.Fatalf("ошибка=%q, хотим про read-only payload", err.Error())
	}
	inst, _ := st.LoadInstance("p-000001")
	if inst.Status != store.StatusFailed {
		t.Fatalf("статус=%q, хотим провален (D-14) после нарушения read-only", inst.Status)
	}
	if _, leaked := inst.Variables["данные"]; leaked {
		t.Fatalf("payload утёк в Variables несмотря на отказ записи")
	}
}
