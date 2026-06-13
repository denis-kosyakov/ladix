package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// Golden-тесты прохода fire-if-true в `run` (007a US2, §TR-6/§TR-8/§TR-9).
//
// Стратегия детерминизма (зеркало TestMetricDispatchNoPeriod): фикстуры пишутся в
// t.TempDir() с АБСОЛЮТНЫМ путём data-файла → без зависимости от cwd; метрика без
// периода/по_дате → без зависимости от сегодня(); шаг без `срок` → без масок времени.
// На свежем Store id детерминированы (p-000001/t-000001). Для повторного run --db id
// растут монотонно (счётчик персистентен) — там id нормализуются idMaskRE.

// triggerDataJSON — оплаченные 1_200_000 + 800_000 = 2_000_000, отменённый 500_000
// отфильтрован `где статус == "оплачен"`.
const triggerDataJSON = `[
  {"клиент": "Альфа",  "сумма_заказа": 1200000, "статус": "оплачен"},
  {"клиент": "Бета",   "сумма_заказа":  800000, "статус": "оплачен"},
  {"клиент": "Гамма",  "сумма_заказа":  500000, "статус": "отменён"}
]`

// triggerProgram строит .ladix-демо с заданным порогом сравнения метрика-триггера.
// dataPath — абсолютный путь к JSON (резолвится из любого cwd).
func triggerProgram(dataPath, threshold string) string {
	return "" +
		"источник продажи_демо:\n" +
		"    файл: \"" + dataPath + "\"\n\n" +
		"метрика выручка_демо:\n" +
		"    источник: продажи_демо\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n\n" +
		"процесс разбор_падения(текущая_выручка):\n" +
		"    шаг подготовка_отчёта:\n" +
		"        исполнитель: \"аналитик\"\n" +
		"        присвоить выручка = текущая_выручка\n\n" +
		"когда метрика выручка_демо < " + threshold + ":\n" +
		"    печать(\"выручка ниже порога:\", значение)\n" +
		"    запустить процесс разбор_падения(значение)\n"
}

// writeTriggerFixture кладёт data.json + prog.ladix во временный каталог и возвращает
// путь к .ladix. Метрика без периода → дата-независима; шаг без срока → без масок.
func writeTriggerFixture(t *testing.T, threshold string) string {
	t.Helper()
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "данные.json")
	if err := os.WriteFile(dataPath, []byte(triggerDataJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	progPath := filepath.Join(dir, "демо.ladix")
	if err := os.WriteFile(progPath, []byte(triggerProgram(dataPath, threshold)), 0o644); err != nil {
		t.Fatal(err)
	}
	return progPath
}

// idMaskRE маскирует изменчивые id процессов/задач (p-NNNNNN / t-NNNNNN): при
// повторном run --db счётчик персистентен → id сдвигаются (это ОК по §TR-8.3). Под
// маской остаётся инвариантная часть строки.
var idMaskRE = regexp.MustCompile(`[pt]-\d{6}`)

func maskIDs(s string) string { return idMaskRE.ReplaceAllString(s, "<ID>") }

// T023 — истина: метрика 2_000_000, порог < 3_000_000 → триггер срабатывает.
// Тело печатает «значение» (= 2_000_000), запускает процесс (задача-ожидание), и
// задача попадает в сводку. Свежий Store (без --db) → id детерминированы.
func TestRunTriggerFiresGolden(t *testing.T) {
	prog := writeTriggerFixture(t, "3_000_000")
	want := "" +
		"выручка ниже порога: 2000000\n" +
		"[задача] t-000001 → аналитик, шаг 'подготовка_отчёта'\n" +
		"открытых задач: 1\n" +
		"t-000001  p-000001  'подготовка_отчёта'  аналитик\n"

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	if out.String() != want {
		t.Errorf("stdout байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", out.String(), want)
	}
}

// T023 — контр-демо: тот же каркас, порог < 1_000_000 (ложь при 2_000_000) →
// молчание, ПУСТАЯ сводка, exit 0. Тело не исполнено, процесс не запущен.
func TestRunTriggerSilentGolden(t *testing.T) {
	prog := writeTriggerFixture(t, "1_000_000")

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout не пуст — ложное условие должно молчать: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// T023 — повторный run --db на той же БД идентичен по ПОВЕДЕНИЮ (§TR-8.3): база ЛОЖЬ
// эфемерна (trigger_state не читается/не пишется), поэтому оба прогона срабатывают.
// id процессов/задач сдвигаются между прогонами (счётчик персистентен) — это ОК,
// нормализуется maskIDs. После маски: прогон 1 создаёт 1 задачу; прогон 2 снова
// срабатывает и добавляет ВТОРУЮ задачу (старая durable-задача остаётся в сводке).
func TestRunTriggerDBRepeatEphemeral(t *testing.T) {
	prog := writeTriggerFixture(t, "3_000_000")
	db := filepath.Join(t.TempDir(), "trig.db")

	// Прогон 1: одна сработка, одна задача.
	want1 := "" +
		"выручка ниже порога: 2000000\n" +
		"[задача] <ID> → аналитик, шаг 'подготовка_отчёта'\n" +
		"открытых задач: 1\n" +
		"<ID>  <ID>  'подготовка_отчёта'  аналитик\n"
	var out1, err1 bytes.Buffer
	if code := realMain([]string{"run", prog, "--db", db}, &out1, &err1); code != 0 {
		t.Fatalf("прогон 1: код = %d, хотим 0; stderr=%q", code, err1.String())
	}
	if got := maskIDs(out1.String()); got != want1 {
		t.Errorf("прогон 1 (маска <ID>) не совпал:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want1)
	}

	// Прогон 2: триггер снова срабатывает (база ЛОЖЬ эфемерна) → ВТОРАЯ задача;
	// сводка durable показывает обе. id сдвинуты, но под маской идентичны инварианту.
	want2 := "" +
		"выручка ниже порога: 2000000\n" +
		"[задача] <ID> → аналитик, шаг 'подготовка_отчёта'\n" +
		"открытых задач: 2\n" +
		"<ID>  <ID>  'подготовка_отчёта'  аналитик\n" +
		"<ID>  <ID>  'подготовка_отчёта'  аналитик\n"
	var out2, err2 bytes.Buffer
	if code := realMain([]string{"run", prog, "--db", db}, &out2, &err2); code != 0 {
		t.Fatalf("прогон 2: код = %d, хотим 0; stderr=%q", code, err2.String())
	}
	if got := maskIDs(out2.String()); got != want2 {
		t.Errorf("прогон 2 (маска <ID>) не совпал:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want2)
	}

	// Канон §TR-6.2/§TR-8.3: durable-состояние триггера эфемерно под `run` —
	// `run`/fire-if-true НЕ читает и НЕ пишет trigger_state (потому оба прогона
	// снова срабатывают). Схема trigger_state/events существует с 007b (DDL
	// безусловный), но `run` оставляет её ПУСТОЙ. Проверяем поведенческий
	// инвариант: после двух прогонов в trigger_state нет строки триггера trg-0
	// (LoadTriggerState → ErrTriggerStateNotFound), а очередь events пуста.
	sq, oerr := store.NewSQLiteStore(db)
	if oerr != nil {
		t.Fatalf("открытие БД: %v", oerr)
	}
	defer sq.Close()
	if _, lerr := sq.LoadTriggerState("trg-0"); !errors.Is(lerr, store.ErrTriggerStateNotFound) {
		t.Errorf("trigger_state не пуст после run --db (err=%v) — нарушен канон «база ЛОЖЬ эфемерно»", lerr)
	}
	if evs, eerr := sq.ListUnprocessedEvents(); eerr != nil || len(evs) != 0 {
		t.Errorf("events не пуст после run --db (events=%v, err=%v) — run не трогает очередь событий", evs, eerr)
	}
}

// T023 — событие/расписание-триггеры в run = no-op + одна строка-заглушка на триггер
// в порядке объявления (§TR-6.4). Тело НЕ исполняется (нет «… тело» в stdout).
func TestRunTriggerEventScheduleStubGolden(t *testing.T) {
	dir := t.TempDir()
	prog := filepath.Join(dir, "загл.ladix")
	src := "" +
		"когда событие заявка_создана:\n" +
		"    печать(\"событие тело\")\n\n" +
		"когда расписание каждые 1дн:\n" +
		"    печать(\"расписание тело\")\n\n" +
		"когда расписание в \"09:00\":\n" +
		"    печать(\"в тело\")\n"
	if err := os.WriteFile(prog, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	want := "" +
		"событие триггер 'заявка_создана' требует serve (фича 007b)\n" +
		"расписание триггер '1дн' требует serve (фича 007b)\n" +
		"расписание триггер '09:00' требует serve (фича 007b)\n"

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	if out.String() != want {
		t.Errorf("stdout заглушек байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", out.String(), want)
	}
}

// writeProgFixture кладёт data.json (общая фикстура triggerDataJSON) во временный
// каталог и склеивает .ladix-программу из переданного тела src, подставляя
// абсолютный путь к JSON через плейсхолдер %DATA%. Возвращает путь к .ladix.
// Зеркало writeTriggerFixture, но с произвольным телом программы (нужны несколько
// метрик/триггеров вперемешку, чего одноформенный triggerProgram не покрывает).
func writeProgFixture(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "данные.json")
	if err := os.WriteFile(dataPath, []byte(triggerDataJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	progPath := filepath.Join(dir, "демо.ladix")
	prog := strings.ReplaceAll(src, "%DATA%", dataPath)
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	return progPath
}

// COV-1 / FR-012 / §TR-6.1 — порядок исполнения НЕСКОЛЬКИХ метрика-триггеров следует
// текстовому порядку ОБЪЯВЛЕНИЯ триггеров, а не порядку имён метрик в реестре. Две
// метрики `яблоко` и `арбуз` (обе истинны при 2_000_000 < 3_000_000) объявлены, а
// триггеры по ним заданы в порядке [яблоко, арбуз] — это ОБРАТНО алфавитному порядку
// имён метрик (арбуз < яблоко): случайная/именная сортировка реестра дала бы арбуз
// первым и сломала бы тест. АССЕРТ: и строки stdout, и id создаваемых процессов
// (p-000001 раньше p-000002) идут в порядке объявления триггеров.
func TestRunTriggerMultiMetricOrderGolden(t *testing.T) {
	src := "" +
		"источник продажи_демо:\n" +
		"    файл: \"%DATA%\"\n\n" +
		"метрика яблоко:\n" +
		"    источник: продажи_демо\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n\n" +
		"метрика арбуз:\n" +
		"    источник: продажи_демо\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n\n" +
		"процесс разбор_я(текущая_выручка):\n" +
		"    шаг отчёт_я:\n" +
		"        исполнитель: \"аналитик\"\n" +
		"        присвоить выручка = текущая_выручка\n\n" +
		"процесс разбор_а(текущая_выручка):\n" +
		"    шаг отчёт_а:\n" +
		"        исполнитель: \"аналитик\"\n" +
		"        присвоить выручка = текущая_выручка\n\n" +
		"когда метрика яблоко < 3_000_000:\n" +
		"    печать(\"маркер ЯБЛОКО\", значение)\n" +
		"    запустить процесс разбор_я(значение)\n\n" +
		"когда метрика арбуз < 3_000_000:\n" +
		"    печать(\"маркер АРБУЗ\", значение)\n" +
		"    запустить процесс разбор_а(значение)\n"
	prog := writeProgFixture(t, src)

	// Порядок объявления триггеров: яблоко → арбуз. id процессов/задач рождаются в
	// этом же порядке: ЯБЛОКО получает p-000001/t-000001, АРБУЗ — p-000002/t-000002.
	want := "" +
		"маркер ЯБЛОКО 2000000\n" +
		"[задача] t-000001 → аналитик, шаг 'отчёт_я'\n" +
		"маркер АРБУЗ 2000000\n" +
		"[задача] t-000002 → аналитик, шаг 'отчёт_а'\n" +
		"открытых задач: 2\n" +
		"t-000001  p-000001  'отчёт_я'  аналитик\n" +
		"t-000002  p-000002  'отчёт_а'  аналитик\n"

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	if out.String() != want {
		t.Errorf("stdout байт-не-точен (порядок объявления ≠ порядок имён метрик):\n--- получено ---\n%s\n--- ожидание ---\n%s", out.String(), want)
	}
}

// COV-2 / FR-018 / §TR-6.4 — метрика+событие+расписание ВПЕРЕМЕШКУ. Sibling к
// TestRunTriggerEventScheduleStubGolden: триггеры в порядке [расписание, метрика
// (истина), событие] — истинный метрика-триггер ЗАЖАТ между двумя заглушками.
// АССЕРТ: строка-заглушка расписания → вывод тела метрики (печать + запуск процесса)
// → строка-заглушка события — строго в порядке объявления; затем сводка задач
// («открытых задач: N» + строка задачи) ПОСЛЕДНЕЙ (§TR-8.1: RunTriggers до сводки).
func TestRunTriggerMixedKindsOrderGolden(t *testing.T) {
	src := "" +
		"источник продажи_демо:\n" +
		"    файл: \"%DATA%\"\n\n" +
		"метрика выручка_демо:\n" +
		"    источник: продажи_демо\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n\n" +
		"процесс разбор_падения(текущая_выручка):\n" +
		"    шаг подготовка_отчёта:\n" +
		"        исполнитель: \"аналитик\"\n" +
		"        присвоить выручка = текущая_выручка\n\n" +
		"когда расписание каждые 1дн:\n" +
		"    печать(\"расписание тело\")\n\n" +
		"когда метрика выручка_демо < 3_000_000:\n" +
		"    печать(\"метрика тело\", значение)\n" +
		"    запустить процесс разбор_падения(значение)\n\n" +
		"когда событие заявка_создана:\n" +
		"    печать(\"событие тело\")\n"
	prog := writeProgFixture(t, src)

	// Порядок объявления: расписание (заглушка) → метрика (тело: печать + задача) →
	// событие (заглушка); сводка durable-задач — последней.
	want := "" +
		"расписание триггер '1дн' требует serve (фича 007b)\n" +
		"метрика тело 2000000\n" +
		"[задача] t-000001 → аналитик, шаг 'подготовка_отчёта'\n" +
		"событие триггер 'заявка_создана' требует serve (фича 007b)\n" +
		"открытых задач: 1\n" +
		"t-000001  p-000001  'подготовка_отчёта'  аналитик\n"

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	if out.String() != want {
		t.Errorf("stdout вперемешку байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", out.String(), want)
	}
}

// COV-3 позитив / FR-025 / §TR-5 — ВИДИМОСТЬ (чтение) глобалов в теле триггера +
// эфемерность локального «пусть» тела. НЕ проверяет запрет ЗАПИСИ в глобал — это
// отдельный замок TestRunTriggerGlobalReadOnly (TR-BODY-RO). Фиксируем два свойства
// одним прогоном:
//
//	(а) тело ЧИТАЕТ глобальный «пусть g = 7» и «значение» (снимок метрики) — Lookup
//	    поднимается вверх сквозь boundary-env (барьер режет запись, не чтение);
//	(б) локальный «пусть g = 999» в теле A ТЕНИТ глобал лишь локально и НЕ протекает
//	    ни в глобал, ни в последующее тело B.
//
// Семантика подтверждена пробой: «пусть g» в теле триггера = env.Define в локальном
// boundary-env тела (NewEnvironment(global)), а не Assign в глобал; vars/letLine тела
// засеяны пусто (analyze.go checkTriggerBody), поэтому теневой «пусть g» при глобальном
// g НЕ даёт SEM-REDECL-VAR. Базовый вариант ПРИМЕНИМ напрямую (шейдинг разрешён):
// A печатает 999 (тень), B печатает 7 (глобал цел, тень A испарилась).
func TestRunTriggerBodyReadShadowGolden(t *testing.T) {
	src := "" +
		"пусть g = 7\n\n" +
		"источник продажи_демо:\n" +
		"    файл: \"%DATA%\"\n\n" +
		"метрика выручка_демо:\n" +
		"    источник: продажи_демо\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n\n" +
		"когда метрика выручка_демо < 3_000_000:\n" +
		"    пусть g = 999\n" +
		"    печать(\"A:\", g, значение)\n\n" +
		"когда метрика выручка_демо < 3_000_000:\n" +
		"    печать(\"B:\", g, значение)\n"
	prog := writeProgFixture(t, src)

	// A видит снимок «значение»=2000000 и свою локальную тень g=999; B видит тот же
	// снимок и НЕТРОНУТЫЙ глобал g=7 (тень A не протекла ни в глобал, ни в тело B).
	want := "" +
		"A: 999 2000000\n" +
		"B: 7 2000000\n"

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	if out.String() != want {
		t.Errorf("stdout областей видимости байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", out.String(), want)
	}
}

// COV-3 настоящий замок / FR-025 / §TR-5 / TR-BODY-RO — read-only ЗАПИСИ в глобал из
// тела триггера обеспечен env-барьером (environment.go boundary). Глобал «пусть g = 7»,
// метрика-триггер с ИСТИННЫМ условием, тело — ГОЛОЕ присваивание «g = 5» (без «пусть»:
// попытка мутации глобала, не объявление локали). Барьер обрывает Assign на boundary-env
// тела (g не локаль тела), а eval различает: Lookup нашёл g вверху (глобал) → рантайм-
// ошибка TR-BODY-RO, exit 1. Это и есть инфорсмент FR-025: БЕЗ барьера тест падает —
// Assign поднялся бы к глобалу, тихо записал 5 и вернул exit 0 (полый прежний замок).
func TestRunTriggerGlobalReadOnly(t *testing.T) {
	src := "" +
		"пусть g = 7\n\n" +
		"источник продажи_демо:\n" +
		"    файл: \"%DATA%\"\n\n" +
		"метрика выручка_демо:\n" +
		"    источник: продажи_демо\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n\n" +
		"когда метрика выручка_демо < 3_000_000:\n" +
		"    g = 5\n"
	prog := writeProgFixture(t, src)

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "глобальная переменная 'g' доступна в теле триггера только для чтения") {
		t.Errorf("stderr не содержит TR-BODY-RO для записи в глобал 'g': %q", errBuf.String())
	}
}

// COV-3 негатив-замок / §TR-5 — «значение» неприсваиваемо. Проба подтвердила: токен
// «значение» — KW_VALUE (первичное ValueExpr, не Ident), поэтому «значение = 5» в теле
// триггера упирается в синтаксический гард цели присваивания (lvalue обязан быть
// Ident): SE-ASSIGN-TARGET «неверная цель присваивания», код 1. Замок фиксирует, что
// read-only-привязка значение не может быть переприсвоена пользователем.
func TestRunTriggerValueNotAssignable(t *testing.T) {
	src := "" +
		"источник продажи_демо:\n" +
		"    файл: \"%DATA%\"\n\n" +
		"метрика выручка_демо:\n" +
		"    источник: продажи_демо\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n\n" +
		"когда метрика выручка_демо < 3_000_000:\n" +
		"    значение = 5\n"
	prog := writeProgFixture(t, src)

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "неверная цель присваивания") {
		t.Errorf("stderr не содержит ошибку цели присваивания «значение»: %q", errBuf.String())
	}
}

// N1-FN-LOCK / §TR-5 «Граница гарантии» — инвариант: функция, ВЫЗВАННАЯ из тела
// триггера, штатно ПЕРЕПРИВЯЗЫВАЕТ глобал. Кадр функции создаётся с parent=global
// (call.go callUser: NewEnvironment(i.global)), а НЕ от boundary-env тела — поэтому
// барьер read-only в его цепочку не попадает, и присваивание глобала из функции
// проходит штатно. Это функциональная семантика 003 (барьер режет перепривязку имени
// ИЗ ТЕЛА, не из вызванной функции), вне read-only-гарантии триггера. Глобал «пусть
// счёт = 0», функция обновить() пишет «счёт = 99», метрика-триггер (истина) её ВЫЗЫВАЕТ,
// второй метрика-триггер печатает обновлённый счёт.
//
// Назначение замка: барьер НЕ должен течь в кадры вызова функций. Если кадр
// отрефакторят на parent=callerEnv (boundary-env тела), барьер протечёт и «счёт = 99»
// внутри функции начнёт ЛОЖНО падать TR-BODY-RO — этот тест поймает регресс
// (мутпробой подтверждено: без него ни один тест этого не ловит). Ожидание: exit 0,
// stdout содержит «счёт: 99», stderr БЕЗ «только для чтения».
func TestRunTriggerFnCallWritesGlobal(t *testing.T) {
	src := "" +
		"пусть счёт = 0\n\n" +
		"функция обновить():\n" +
		"    счёт = 99\n\n" +
		"источник продажи_демо:\n" +
		"    файл: \"%DATA%\"\n\n" +
		"метрика выручка_демо:\n" +
		"    источник: продажи_демо\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n\n" +
		"когда метрика выручка_демо < 3_000_000:\n" +
		"    обновить()\n\n" +
		"когда метрика выручка_демо < 3_000_000:\n" +
		"    печать(\"счёт:\", счёт)\n"
	prog := writeProgFixture(t, src)

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", prog}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	if strings.Contains(errBuf.String(), "только для чтения") {
		t.Errorf("ложный TR-BODY-RO: барьер протёк в кадр функции, вызванной из тела: %q", errBuf.String())
	}
	if !strings.Contains(out.String(), "счёт: 99") {
		t.Errorf("stdout не содержит обновлённый глобал «счёт: 99» — функция из тела не записала глобал: %q", out.String())
	}
}

// TestRunBadTimeFormat / SE-TIME-FORMAT (FR-014/SC-010/FR-026) — РУН-СТОРОННИЙ замок на
// САНКЦИОНИРОВАННУЮ дельту 007b: расписание-триггер с невалидным «в "ЧЧ:ММ"» в `run`
// теперь даёт семош «неверный формат времени…» + exit 1. В 007a та же строка была
// stub-no-op с exit 0 (формат не анализировался, граница 007a). serve_golden_test уже
// фиксирует это на serve-пути (TestServeBadTimeFormat); этот тест пинит run-путь, чтобы
// будущее не откатило намеренную дельту молча (run и serve делят interp.Analyze).
func TestRunBadTimeFormat(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", filepath.Join("testdata", "bad_time.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "неверный формат времени") {
		t.Errorf("stderr не содержит диагностику SE-TIME-FORMAT: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "Ошибка в строке") {
		t.Errorf("ожидалась каноническая двухстрочная диагностика, stderr=%q", errBuf.String())
	}
	if strings.Contains(errBuf.String(), ".go:") || strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился Go stack trace: %q", errBuf.String())
	}
}
