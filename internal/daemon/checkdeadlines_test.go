package daemon

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// buildDeadlineDaemon — вариант buildDaemon, возвращающий ТАКЖЕ движок и Store: тестам
// эскалации нужен прямой eng.Start (создать инстанс+задачу со сроком) минуя top-level
// «запустить процесс» (детерминированный момент создания под управляемыми часами).
// Один engine.Clock инжектится и в движок, и в демон (§AU-6.4 — без «двойных часов»):
// CreatedAt задачи и d.clock.Now() из одного источника.
func buildDeadlineDaemon(t *testing.T, src string, st store.Store, out *countWriter) (*Daemon, *fixedClock, *engine.Engine, store.Store) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	clk := &fixedClock{t: time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}
	interp := eval.NewInterpreter(out, 0, eval.SystemClock{})
	eng := engine.NewEngine(st, interp, out, engine.WithClock(clk))
	interp.SetProcessRuntime(eng)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	if err := interp.Run(prog); err != nil {
		t.Fatalf("interp.Run: %s", err.Error())
	}
	d := New(st, eng, interp, clk, time.Minute, out)
	return d, clk, eng, st
}

// escalationSrc — процесс эскалация_плана(факт) с человеческим шагом + сроком 2дн и
// эскалация-триггер, чьё тело уведомляет руководителя значением переменной процесса
// «факт» (инжект всех InstanceVariables, D-AU-6). Маркер durable-golden — точная
// строка §AU-4.2 «[уведомление] руководитель: <факт>».
const escalationSrc = "процесс эскалация_плана(факт):\n" +
	"    шаг связаться_с_клиентом:\n" +
	"        исполнитель: \"менеджер\"\n" +
	"        срок: 2дн\n" +
	"когда задача просрочена в эскалация_плана.связаться_с_клиентом:\n" +
	"    уведомить руководитель(факт)\n"

// TestCheckDeadlinesFire — фаза checkDeadlines (§AU-6.2.2): задача со сроком; Clock до
// срока → тишина; Clock за срок → fireDeadlineBody печатает «[уведомление]
// руководитель: 2500000» (инжект «факт» из inst.Variables, D-AU-6), Escalated=true.
func TestCheckDeadlinesFire(t *testing.T) {
	out := &countWriter{marker: "[уведомление] руководитель: 2500000"}
	st := store.NewMemoryStore()
	d, clk, eng, _ := buildDeadlineDaemon(t, escalationSrc, st, out)

	created := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, created)
	if _, err := eng.Start("эскалация_плана", []value.Value{value.Целое{V: 2500000}}); err != nil {
		t.Fatalf("eng.Start: %v", err)
	}

	// До срока (created+1дн < created+2дн): тишина.
	setClock(clk, created.Add(24*time.Hour))
	d.tick()
	if out.count() != 0 {
		t.Fatalf("до срока: эскалаций = %d, хотим 0; out=%q", out.count(), out.String())
	}

	// За срок (created+3дн > created+2дн): РОВНО одна эскалация, инжект «факт».
	setClock(clk, created.Add(3*24*time.Hour))
	d.tick()
	if out.count() != 1 {
		t.Fatalf("за срок: эскалаций = %d, хотим 1; out=%q", out.count(), out.String())
	}

	// Задача помечена Escalated в Store.
	pend, err := st.ListPendingTasks("")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(pend) != 1 || !pend[0].Escalated {
		t.Fatalf("задача не помечена Escalated после fire: %+v", pend)
	}
}

// TestCheckDeadlinesNoTriggersEarlyReturn — нет эскалация-триггеров → ранний return БЕЗ
// листинга задач (§AU-6.2.2: «если deadlineTriggers пуст { return }»). spyStore считает
// вызовы ListPendingTasks: должен остаться 0.
func TestCheckDeadlinesNoTriggersEarlyReturn(t *testing.T) {
	out := &countWriter{marker: "x"}
	spy := &listSpyStore{Store: store.NewMemoryStore()}
	// Источник без эскалация-триггеров: только процесс (создаёт задачу), без «когда задача …».
	src := "процесс p(факт):\n    шаг s:\n        исполнитель: \"менеджер\"\n        срок: 2дн\n"
	d, clk, eng, _ := buildDeadlineDaemon(t, src, spy, out)

	created := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, created)
	if _, err := eng.Start("p", []value.Value{value.Целое{V: 1}}); err != nil {
		t.Fatalf("eng.Start: %v", err)
	}
	spy.resetPendingCount() // обнулить счётчик после старта (engine мог листить)

	setClock(clk, created.Add(3*24*time.Hour)) // даже за сроком
	d.tick()

	if spy.pendingCount() != 0 {
		t.Fatalf("без эскалация-триггеров checkDeadlines не должен листить задачи: ListPendingTasks вызван %d раз", spy.pendingCount())
	}
}

// TestCheckDeadlinesCompletedNoFire — задача завершена ДО просрочки → не в
// ListPendingTasks → эскалации нет (§AU-6.3 граничный случай).
func TestCheckDeadlinesCompletedNoFire(t *testing.T) {
	out := &countWriter{marker: "[уведомление]"}
	st := store.NewMemoryStore()
	d, clk, eng, _ := buildDeadlineDaemon(t, escalationSrc, st, out)

	created := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, created)
	if _, err := eng.Start("эскалация_плана", []value.Value{value.Целое{V: 2500000}}); err != nil {
		t.Fatalf("eng.Start: %v", err)
	}
	// Завершить задачу до просрочки.
	pend, _ := st.ListPendingTasks("")
	if len(pend) != 1 {
		t.Fatalf("ожидали 1 открытую задачу, получили %d", len(pend))
	}
	if err := st.MarkTaskCompleted(pend[0].ID, created.Add(time.Hour)); err != nil {
		t.Fatalf("MarkTaskCompleted: %v", err)
	}

	setClock(clk, created.Add(3*24*time.Hour))
	d.tick()
	if out.count() != 0 {
		t.Fatalf("завершённая задача не эскалируется: out=%q", out.String())
	}
}

// listSpyStore считает вызовы ListPendingTasks (проверка раннего return без листинга).
type listSpyStore struct {
	store.Store
	n int
}

func (s *listSpyStore) ListPendingTasks(assignee string) ([]*store.Task, error) {
	s.n++
	return s.Store.ListPendingTasks(assignee)
}
func (s *listSpyStore) pendingCount() int  { return s.n }
func (s *listSpyStore) resetPendingCount() { s.n = 0 }

// TestDeadlineDurableRestart — INV-2 (§AU-12.B / contracts/durable-restart.md):
// durable-golden эскалации × рестарт на уровне Go-API (ladix start — это B5, после B4).
// Уведомление руководителю печатается РОВНО ОДИН раз за все тики обоих прогонов:
// (1) создать инстанс на SQLiteStore(demo.db), факт=2500000, срок created+2дн;
// (2) FixedClock=created → tick → тишина (не просрочено);
// (3) Clock+3дн → tick → РОВНО одна эскалация, Escalated персистнут (UPSERT, точка 3);
// (4) tick снова без рестарта → Escalated==true → continue → нет повтора;
// (5) РЕСТАРТ (новый SQLiteStore на той же --db) + RunRestartScan + tick →
//
//	ListPendingTasks читает Escalated=true (точка 4) → continue → нет повтора;
//
// (6) assert: уведомление РОВНО один раз. Это главный durable-замок (мутпроба снятия
//
//	фильтра if t.Escalated краснит этот тест).
func TestDeadlineDurableRestart(t *testing.T) {
	const marker = "[уведомление] руководитель: 2500000"
	dbPath := filepath.Join(t.TempDir(), "demo.db")
	created := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)

	// --- Прогон 1: один Store/демон на demo.db ---
	out1 := &countWriter{marker: marker}
	st1, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore #1: %v", err)
	}
	d1, clk1, eng1, _ := buildDeadlineDaemon(t, escalationSrc, st1, out1)

	setClock(clk1, created)
	if _, err := eng1.Start("эскалация_плана", []value.Value{value.Целое{V: 2500000}}); err != nil {
		t.Fatalf("eng.Start: %v", err)
	}

	// tick до срока → тишина.
	d1.tick()
	if out1.count() != 0 {
		t.Fatalf("до срока: эскалаций = %d, хотим 0", out1.count())
	}

	// Clock+3дн → tick → РОВНО одна эскалация.
	setClock(clk1, created.Add(3*24*time.Hour))
	d1.tick()
	if out1.count() != 1 {
		t.Fatalf("за срок: эскалаций = %d, хотим 1; out=%q", out1.count(), out1.String())
	}

	// tick снова без рестарта → Escalated → continue → нет повтора.
	d1.tick()
	if out1.count() != 1 {
		t.Fatalf("повторный тик без рестарта: эскалаций = %d, хотим 1 (одноразовость)", out1.count())
	}

	// Escalated персистнут в SQLite (перечитать тем же Store).
	pend1, _ := st1.ListPendingTasks("")
	if len(pend1) != 1 || !pend1[0].Escalated {
		t.Fatalf("Escalated не персистнут в прогоне 1: %+v", pend1)
	}
	if err := st1.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}

	// --- Прогон 2 (РЕСТАРТ): новый SQLiteStore на той же --db ---
	out2 := &countWriter{marker: marker}
	st2, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore #2 (рестарт): %v", err)
	}
	d2, clk2, _, _ := buildDeadlineDaemon(t, escalationSrc, st2, out2)
	defer st2.Close()

	// Часы рестарт-демона уже за сроком.
	setClock(clk2, created.Add(3*24*time.Hour))

	// (б) персист: на рестарте Escalated читается true из той же --db (точка 4 кодека).
	rt, err := st2.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask после рестарта: %v", err)
	}
	if !rt.Escalated {
		t.Fatalf("Escalated НЕ персистнут между прогонами (durable нарушен)")
	}

	d2.RunRestartScan()
	d2.tick()
	if out2.count() != 0 {
		t.Fatalf("ПОВТОР эскалации после рестарта: out=%q (durable нарушен)", out2.String())
	}

	// (а) единичность: суммарно за оба прогона уведомление РОВНО ОДИН раз.
	total := strings.Count(out1.String(), marker) + strings.Count(out2.String(), marker)
	if total != 1 {
		t.Fatalf("уведомление руководителю напечатано %d раз за оба прогона, хотим РОВНО 1", total)
	}
}
