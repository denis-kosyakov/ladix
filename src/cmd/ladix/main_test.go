package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
)

func examplePath(name string) string {
	return filepath.Join("..", "..", "..", "examples", name)
}

// deadlineMaskRE — маска времени дедлайна (§EN-9): CLI-тесты не фиксируют абсолютный
// момент (SystemClock), маскируют только «срок до <время>». Формат времени —
// "2006-01-02 15:04" (engine.deadlineLayout).
var deadlineMaskRE = regexp.MustCompile(`срок до \d{4}-\d{2}-\d{2} \d{2}:\d{2}`)

// maskDeadlines заменяет «срок до <время>» на «срок до <DT>» — id детерминированы,
// маскируется только время (§EN-9).
func maskDeadlines(s string) string {
	return deadlineMaskRE.ReplaceAllString(s, "срок до <DT>")
}

// T044/T045: успешный прогон → stdout, код 0.
func TestRunSuccess(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("hello.ladix")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.String() != "Привет, Уклад!\n" {
		t.Errorf("stdout = %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// T047/SC-002: ошибка.ladix → ровно двухстрочный stderr, код 1, без stack trace.
func TestRunErrorExample(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("ошибка.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	want := "Ошибка в строке 5, колонка 14:\nделение на ноль\n"
	if errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q", errBuf.String(), want)
	}
	if strings.Contains(errBuf.String(), ".go:") || strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился Go stack trace: %q", errBuf.String())
	}
}

// Коды использования CLI (код 2): нет подкоманды/файла/неверный флаг/нет файла.
func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{},                     // нет подкоманды
		{"run"},                // нет файла
		{"run", "--max-depth"}, // флаг без значения
		{"run", "--max-depth", "0", examplePath("hello.ladix")}, // неверное значение
		{"run", "--нечто", examplePath("hello.ladix")},          // неизвестный флаг
		{"run", "нет-такого-файла.ladix"},                       // файл не читается
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain(args, &out, &errBuf)
			if code != 2 {
				t.Errorf("args=%v: код = %d, хотим 2", args, code)
			}
		})
	}
}

// --max-depth пробрасывается в лимит и в текст ошибки RT-DEPTH.
func TestMaxDepthFlag(t *testing.T) {
	// файл с убегающей рекурсией готовим во временном каталоге
	dir := t.TempDir()
	file := filepath.Join(dir, "rec.ladix")
	src := "функция f():\n    вернуть f()\nпечать(f())\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", "--max-depth", "7", file}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	if !strings.Contains(errBuf.String(), "превышена максимальная глубина вызовов (7).") {
		t.Errorf("stderr не содержит лимит 7: %q", errBuf.String())
	}
}

// 006/US1 (§EN-9 Сценарий А): онбординг.ladix исполняется через MemoryStore (run
// без --db) — 'запустить процесс' активирован движком. Exit 0, байт-точный golden
// 5 строк. id детерминированы (свежий Store → p-000001/t-000001); маскируется только
// <время> дедлайна (deadlineMaskRE). Сменился вердикт фичи 006 относительно 005,
// где это была рантайм-граница (код 1, §DP-4).
func TestRunOnboardingProcessDeferred(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("онбординг.ladix")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	want := "" +
		"[уведомление] ИТ: создать учётку для Петров\n" +
		"[задача] t-000001 → руководитель, шаг 'провести_встречу', срок до <DT>\n" +
		"запущен онбординг, id: p-000001\n" +
		"открытых задач: 1\n" +
		"t-000001  p-000001  'провести_встречу'  руководитель  срок до <DT>\n"
	if got := maskDeadlines(out.String()); got != want {
		t.Errorf("stdout (с маской <DT>) байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
}

// T018 (016 B4a, §AU-6): контроль_плана.ladix — витрина эскалация-триггера.
// Процесс с человеческим шагом и `срок:` + `когда задача просрочена в P.S:`.
// Под `run` (MemoryStore): процесс стартует штатно (задача t-000001), затем
// эскалация-триггер печатает строку-заглушку «требует serve (фича 007b)» (тело
// НЕ исполняется — нет живого таймера, исполняет демон serve, B4b). Exit 0, id
// детерминированы (свежий Store → p-000001/t-000001), маскируется только <время>
// дедлайна (deadlineMaskRE, §EN-9). «задача»/«просрочена» остаются IDENT (D-AU-4).
// M3-C2b: пример эволюционировал (+2 авто-шага догона зафиксировать_итог/уведомить_crm),
// но stdout под `run` НЕ меняется — авто-шаги стоят ПОСЛЕ человеческого шага и
// исполняются только в `serve` после `complete --данные` (под `run` процесс паркуется
// на ожидающей задаче связаться_с_клиентом). Durable-гейт C2b — TestStepEffectExactlyOnceRestart.
// 🔁 ИНВЕРСИЯ: пример сломан/тело начало исполняться/текст заглушки разошёлся → красный.
func TestCLIGoldenDeadlineEscalation(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("контроль_плана.ladix")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	want := "" +
		"[задача] t-000001 → менеджер, шаг 'связаться_с_клиентом', срок до <DT>\n" +
		"запущен контроль плана, id: p-000001\n" +
		"задача триггер 'эскалация_плана.связаться_с_клиентом' требует serve (фича 007b)\n" +
		"открытых задач: 1\n" +
		"t-000001  p-000001  'связаться_с_клиентом'  менеджер  срок до <DT>\n"
	if got := maskDeadlines(out.String()); got != want {
		t.Errorf("stdout (с маской <DT>) байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
}

// T019/US1 (§TR-9/§TR-10.5 п.4/SC-001): выручка.ladix теперь компилируется ЧИСТО —
// 'когда метрика …' парсится как объявление триггера (фронтенд триггеров 007a),
// семпроход резолвит метрику выручка_месяца и процесс разбор_падения. Exit 0, ноль
// диагностик. Инверсия прежнего негатива (TestRunRevenueUnexpectedWhen, 005/CP-5.3 R1,
// ждавшего «неожиданный элемент 'когда'»).
//
// Устойчивость к US2 (T022, врезка fire-if-true): после неё метрика-триггер
// СРАБАТЫВАЕТ — выручка_месяца вычисляется (метрика с периодом, окно зависит от
// сегодня()) и дописывает задачу в сводку. Поэтому тест НЕ пинит полный stdout;
// утверждает только (а) exit 0 и (б) отсутствие маркеров диагностик. Прогон — из
// корня репо (withRepoRoot, как metric-тесты): относительный путь источника
// «data/sales.json» в выручка.ladix резолвится только оттуда, иначе сработавший
// триггер упёрся бы в «файл не найден» (exit 1). Так замок остаётся зелёным и после US2.
func TestRunRevenueParsesClean(t *testing.T) {
	withRepoRoot(t, func() {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"run", filepath.Join("examples", "выручка.ladix")}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		// Ноль диагностик: маркеры канона §13 не должны встречаться ни в stdout, ни в stderr.
		combined := out.String() + errBuf.String()
		for _, marker := range []string{"Ошибка в строке", "неожиданный элемент"} {
			if strings.Contains(combined, marker) {
				t.Errorf("в выводе просочилась диагностика %q: %q", marker, combined)
			}
		}
	})
}

// 005/FR-023 (CP-3): программа, ТОЛЬКО объявляющая процесс (без top-level
// 'запустить процесс') → код 0: рантайм отрабатывает, ProcessDecl — Decl,
// не Statement — пропускается циклом Run(); тело шага в 005 не исполняется
// (печать(1) внутри шага не попадает в stdout, §PM-5).
func TestRunProcessDeclOnly(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "процесс.ladix")
	src := "процесс P:\n    шаг A:\n        печать(1)\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", file}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout не пуст — тело шага не должно исполняться: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// T023/§EN-9 Сценарий Б — мост SQLite: цепочка из 6 команд на свежей БД. Состояние
// между командами живёт ТОЛЬКО в файле test.db (каждая команда открывает Store
// заново). id детерминированы (свежий Store → p-000001/t-000001…), маска — только
// <время> дедлайнов. Повтор run (шаг 6) даёт p-000002/t-000003 (счётчик персистентен).
func TestScenarioBSQLiteChain(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	file := examplePath("онбординг.ladix")

	type step struct {
		name string
		args []string
		want string
	}
	steps := []step{
		{
			name: "1: run --db",
			args: []string{"run", file, "--db", db},
			want: "" +
				"[уведомление] ИТ: создать учётку для Петров\n" +
				"[задача] t-000001 → руководитель, шаг 'провести_встречу', срок до <DT>\n" +
				"запущен онбординг, id: p-000001\n" +
				"открытых задач: 1\n" +
				"t-000001  p-000001  'провести_встречу'  руководитель  срок до <DT>\n",
		},
		{
			name: "2: tasks --db",
			args: []string{"tasks", "--db", db},
			want: "t-000001  p-000001  'провести_встречу'  руководитель  срок до <DT>\n",
		},
		{
			name: "3: tasks Петров --db",
			args: []string{"tasks", "Петров", "--db", db},
			want: "открытых задач нет\n",
		},
		{
			name: "4: complete t-000001",
			args: []string{"complete", file, "t-000001", "--db", db},
			want: "" +
				"задача t-000001 завершена\n" +
				"[задача] t-000002 → HR, шаг 'закрыть_адаптацию', срок до <DT>\n" +
				"инстанс p-000001: ожидает, шаг 'закрыть_адаптацию'\n",
		},
		{
			name: "5: complete t-000002",
			args: []string{"complete", file, "t-000002", "--db", db},
			want: "" +
				"задача t-000002 завершена\n" +
				"инстанс p-000001: выполнен\n",
		},
		{
			name: "6: run --db (повтор → p-000002/t-000003)",
			args: []string{"run", file, "--db", db},
			want: "" +
				"[уведомление] ИТ: создать учётку для Петров\n" +
				"[задача] t-000003 → руководитель, шаг 'провести_встречу', срок до <DT>\n" +
				"запущен онбординг, id: p-000002\n" +
				"открытых задач: 1\n" +
				"t-000003  p-000002  'провести_встречу'  руководитель  срок до <DT>\n",
		},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain(s.args, &out, &errBuf)
			if code != 0 {
				t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
			}
			if errBuf.Len() != 0 {
				t.Errorf("непустой stderr: %q", errBuf.String())
			}
			if got := maskDeadlines(out.String()); got != s.want {
				t.Errorf("stdout (с маской <DT>) байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, s.want)
			}
		})
	}
}

// testdataPath — путь к фикстуре impl-чата (src/cmd/ladix/testdata).
func testdataPath(name string) string {
	return filepath.Join("testdata", name)
}

// drainScenarioB прогоняет сценарий Б (§EN-9) до шага 5 на свежей БД в t.TempDir():
// run → complete t-000001 → complete t-000002. На выходе: инстанс p-000001 'выполнен',
// задача t-000001 'завершена', задача t-000002 'завершена'. Возвращает путь к БД.
func drainScenarioB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	file := examplePath("онбординг.ladix")
	steps := [][]string{
		{"run", file, "--db", db},
		{"complete", file, "t-000001", "--db", db},
		{"complete", file, "t-000002", "--db", db},
	}
	for _, args := range steps {
		var out, errBuf bytes.Buffer
		if code := realMain(args, &out, &errBuf); code != 0 {
			t.Fatalf("подготовка БД (%v): код=%d, stderr=%q", args, code, errBuf.String())
		}
	}
	return db
}

// TestCompleteNegatives — 6 негативов §EN-9 (после шага 5 сценария Б): дословный
// stderr (для (1)-(5) маска не нужна — id детерминированы, текстов времени нет; для
// (6) — канон §13). Коды 2 (CLI-ошибки §EN-8.B) / 1 (компиляция §13).
func TestCompleteNegatives(t *testing.T) {
	file := examplePath("онбординг.ladix")

	// (1) несуществующая задача → §EN-8.B B1, exit 2.
	t.Run("1: задача не найдена", func(t *testing.T) {
		db := drainScenarioB(t)
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", file, "t-999999", "--db", db}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
		}
		if errBuf.String() != "ladix: задача 't-999999' не найдена\n" {
			t.Errorf("stderr=%q", errBuf.String())
		}
	})

	// (2) повтор при инстансе 'выполнен' (догон неприменим) → §EN-8.B B2, exit 2.
	t.Run("2: задача уже завершена", func(t *testing.T) {
		db := drainScenarioB(t)
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", file, "t-000001", "--db", db}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
		}
		if errBuf.String() != "ladix: задача 't-000001' уже завершена\n" {
			t.Errorf("stderr=%q", errBuf.String())
		}
	})

	// (3) файл без процесса 'онбординг' (компилируется чисто) → §EN-8.B B6, exit 2.
	t.Run("3: процесс не найден в определении", func(t *testing.T) {
		db := drainScenarioB(t)
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", examplePath("hello.ladix"), "t-000001", "--db", db}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
		}
		if errBuf.String() != "ladix: процесс 'онбординг' не найден в определении\n" {
			t.Errorf("stderr=%q", errBuf.String())
		}
	})

	// (4) дрейф шага (онбординг есть, закрыть_адаптацию переименован) → §EN-8.B B7,
	// exit 2. После шага 5 inst.CurrentStep == 'закрыть_адаптацию'; дрейф-гарды Q3
	// идут ДО гарда «уже завершена» (§EN-3) → ловится шаг, а не «уже завершена».
	t.Run("4: шаг не найден в определении", func(t *testing.T) {
		db := drainScenarioB(t)
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", testdataPath("онбординг-дрейф.ladix"), "t-000001", "--db", db}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
		}
		if errBuf.String() != "ladix: шаг 'закрыть_адаптацию' не найден в определении процесса 'онбординг'\n" {
			t.Errorf("stderr=%q", errBuf.String())
		}
	})

	// (5) неоткрываемый путь БД → §EN-8.B B8, exit 2. Текст причины маскируем
	// (зависит от драйвера SQLite) — проверяем префикс дословно.
	t.Run("5: не удалось открыть хранилище", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", file, "t-000001", "--db", "нет/такого/каталога.db"}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
		}
		if !strings.HasPrefix(errBuf.String(), "ladix: не удалось открыть хранилище 'нет/такого/каталога.db': ") {
			t.Errorf("stderr=%q, хотим префикс «ladix: не удалось открыть хранилище 'нет/такого/каталога.db': <причина>»", errBuf.String())
		}
	})

	// (6) парс-ошибка фикстуры (complete компилирует файл → канон §13) → exit 1.
	t.Run("6: парс-ошибка → канон §13", func(t *testing.T) {
		db := drainScenarioB(t)
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", testdataPath("сломанный.ladix"), "t-000001", "--db", db}, &out, &errBuf)
		if code != 1 {
			t.Fatalf("код=%d, хотим 1; stderr=%q", code, errBuf.String())
		}
		// Канон §13: двухстрочный «Ошибка в строке N, колонка M:\n<описание>».
		if !strings.HasPrefix(errBuf.String(), "Ошибка в строке ") {
			t.Errorf("stderr=%q, хотим канон §13", errBuf.String())
		}
		if strings.Contains(errBuf.String(), "ladix:") {
			t.Errorf("канон §13 не должен нести префикс ladix:: %q", errBuf.String())
		}
		if strings.Contains(errBuf.String(), ".go:") || strings.Contains(errBuf.String(), "goroutine") {
			t.Errorf("в stderr просочился Go stack trace: %q", errBuf.String())
		}
	})
}

// TestCLIDiagnosticsEN8B — exact-match текстов §EN-8.B (одна строка «ladix: <текст>»,
// exit 2). Покрывает B4/B5 через CLI-цепочку невозможно (один активный шаг = одна
// задача) — поэтому фабрикуются прямой записью в SQLite через ту же подкоманду
// complete над специально подготовленным состоянием. Остальные тексты (B1/B2/B6/B7/
// B8/B10) покрыты TestCompleteNegatives и здесь не дублируются; B3/B9 — отдельно.
func TestCLIDiagnosticsEN8B(t *testing.T) {
	file := examplePath("онбординг.ladix")

	// B10: флаг --db без значения → §EN-8.B B10, exit 2 (вне guard, разбор флага).
	t.Run("B10: флаг --db требует значение", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", file, "t-000001", "--db"}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("код=%d, хотим 2", code)
		}
		if errBuf.String() != "ladix: флаг --db требует значение\n" {
			t.Errorf("stderr=%q", errBuf.String())
		}
	})

	// B3 (инстанс не найден) проверяется на уровне engine/CLI-маппинга в
	// TestCLIErrInstanceNotFound — SQLite FK (tasks.instance_id REFERENCES instances)
	// делает «осиротевшую задачу» недостижимой через Store API.

	// B4: инстанс не ожидает (статус 'выполняется') → exit 2 (гард D-8).
	t.Run("B4: инстанс не ожидает", func(t *testing.T) {
		db := newDBWithGuardState(t, "выполняется", "провести_встречу")
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", file, "t-000001", "--db", db}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
		}
		if errBuf.String() != "ladix: инстанс 'p-000001' не ожидает (статус 'выполняется')\n" {
			t.Errorf("stderr=%q", errBuf.String())
		}
	})

	// B5: задача не соответствует текущему шагу инстанса → exit 2 (гард D-8).
	t.Run("B5: задача не соответствует шагу", func(t *testing.T) {
		db := newDBWithGuardState(t, "ожидает", "закрыть_адаптацию")
		var out, errBuf bytes.Buffer
		code := realMain([]string{"complete", file, "t-000001", "--db", db}, &out, &errBuf)
		if code != 2 {
			t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
		}
		if errBuf.String() != "ladix: задача 't-000001' не соответствует текущему шагу инстанса 'p-000001'\n" {
			t.Errorf("stderr=%q", errBuf.String())
		}
	})
}

// TestCLIErrInstanceNotFound — §EN-8.B B3 exact-match: задача загрузилась, но её
// инстанс не найден (битая/чужая БД). SQLite FK не даёт осиротить задачу, поэтому
// маппинг проверяется напрямую: engine возвращает типизированный GuardError с id
// инстанса, completeError формирует CLI-текст.
func TestCLIErrInstanceNotFound(t *testing.T) {
	var errBuf bytes.Buffer
	code := completeError(engine.GuardInstanceNotFound("p-bad"), "t-000001", &errBuf)
	if code != 2 {
		t.Fatalf("код=%d, хотим 2", code)
	}
	if errBuf.String() != "ladix: инстанс 'p-bad' не найден\n" {
		t.Errorf("stderr=%q", errBuf.String())
	}
}

// TestCLIStorageFailureB9 — §EN-8.B B9 exact-match: не-сентинельная ошибка Store на
// CLI-пути complete → «ladix: сбой хранилища: <причина>», exit 2 (FR-018). Проверяется
// через completeError напрямую (битую БД из теста надёжно не воспроизвести). На
// CLI-пути engine оборачивает сбой Store в *engine.StoreError.
func TestCLIStorageFailureB9(t *testing.T) {
	var errBuf bytes.Buffer
	code := completeError(engine.NewStoreError(errors.New("ошибка декода кодека")), "t-000001", &errBuf)
	if code != 2 {
		t.Fatalf("код=%d, хотим 2", code)
	}
	if errBuf.String() != "ladix: сбой хранилища: ошибка декода кодека\n" {
		t.Errorf("stderr=%q", errBuf.String())
	}
}

// newDBWithGuardState создаёт свежую SQLite-БД с инстансом p-000001 (status/step
// заданы) и открытой задачей t-000001 на шаге 'провести_встречу' — фабрикует
// состояния гардов D-8 (B4/B5), недостижимые штатной CLI-цепочкой (§EN-9).
func newDBWithGuardState(t *testing.T, status store.Status, currentStep string) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "test.db")
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer sq.Close()
	if err := sq.SaveInstance(&store.ProcessInstance{
		ID: "p-000001", ProcessName: "онбординг", Status: status, CurrentStep: currentStep,
	}); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	if err := sq.SaveTask(&store.Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "провести_встречу",
		Assignee: "руководитель", Status: store.TaskPending,
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	return db
}

// T047/SC-008: recover-барьер ловит непредвиденную Go-панику → дженерик, код 1.
func TestGuardRecoversPanic(t *testing.T) {
	var errBuf bytes.Buffer
	code := guard(&errBuf, func() int {
		panic("искусственная паника")
	})
	if code != 1 {
		t.Errorf("код = %d, хотим 1", code)
	}
	if errBuf.String() != "внутренняя ошибка интерпретатора\n" {
		t.Errorf("stderr = %q", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился stack trace")
	}
}

// TestMetricWiresEngineF1 — F1 (§EN-6): подкоманда metric СОБИРАЕТ движок процессов
// (общая сборка для run/complete/metric). Метрика, чья формула достигает process-builtin
// статус_процесса, идёт через РЕАЛЬНЫЙ движок: инстанс отсутствует → «процесс 'p-999999'
// не найден» (канон §13, exit 1), а НЕ «движок процессов не подключён» (nil-runtime,
// §EN-8.A:685 — недостижим для metric). Канал — слот период: формула вычисляется рано,
// до проверки типа результата, поэтому runtime-ошибка builtin опережает type-mismatch (§EN-6:579).
func TestMetricWiresEngineF1(t *testing.T) {
	dir := t.TempDir()
	dataFile := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataFile, []byte(`[{"когда": "2026-05-01"}]`), 0o644); err != nil {
		t.Fatalf("WriteFile data: %v", err)
	}
	srcFile := filepath.Join(dir, "m.ladix")
	src := "источник данные:\n    файл: \"" + dataFile + "\"\n\n" +
		"метрика m:\n" +
		"    источник: данные\n" +
		"    период: статус_процесса(\"p-999999\")\n" +
		"    по_дате: сегодня()\n" +
		"    агрегат: количество(запись)\n"
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"metric", srcFile, "m"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1\nstdout=%q\nstderr=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "процесс 'p-999999' не найден") {
		t.Errorf("stderr = %q, хотим содержащий «процесс 'p-999999' не найден» (движок подключён, §EN-6)", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "движок процессов не подключён") {
		t.Errorf("stderr = %q — nil-runtime недостижим для metric (§EN-8.A:685, F1)", errBuf.String())
	}
}
