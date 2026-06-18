package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// clock_unify_test.go — тест-замки C4 (§C-4.3): на КАЖДЫЙ переписанный путь CLI
// (run / start / complete / tasks / metric) инъектируется ФИКСИРОВАННЫЙ момент T,
// и всё время-зависимое наблюдаемое поведение обязано следовать ИМЕННО T (детермин-
// но, байт-стабильно, и сменяется на T'). Зеркало serve_golden_test.go:216
// (TestServeMetricDateFollowsSchedulerClock); fixedClock (engine.Clock) определён там
// же (serve_golden_test.go:21-23) — переиспользуем его.
//
// ИНВЕРСИЯ (T015, мутпроба): каждый Test*ClockInjected сконструирован так, что если
// путь откатится на НЕЗАВИСИМЫЙ engine.SystemClock{} (реальные стенные часы), наблю-
// даемое значение разойдётся с T и тест ПОКРАСНЕЕТ. Принцип: «если путь падает на
// реал-тайм, этот тест обязан упасть». Проверяется временным возвратом
// engine.SystemClock{}.Now() в каждый путь (локально, затем откат) — соответствующий
// тест краснеет. Конкретные рычаги инверсии указаны в комментарии каждого теста.

// --- T010: run — дата метрик И «сейчас» сводки следуют T ---

// TestRunClockInjected гонит путь run (runFile) с оконной метрикой, чьё окно =
// КАЛЕНДАРНЫЙ МЕСЯЦ относительно даты вычисления (i.now() → инъектированные часы).
// Записи датированы маем 2026. При T=2026-05-31 окно=май → сумма>0 → метрика ИСТИНА →
// триггер запускает процесс → печать строки эффекта. При T'=январь 2026 окно=январь →
// сумма=0 → метрика ЛОЖЬ → строки нет. Так дата метрик доказуемо идёт от T.
//
// ИНВЕРСИЯ: верни на пути run независимый engine.SystemClock{} в интерпретатор
// (evalClockFromEngine{clock} → eval.SystemClock{}) — дата уедет в системный месяц
// (НЕ май), метрика станет ЛОЖЬ, «mayFired» исчезнет → assert на mayFired упадёт.
func TestRunClockInjected(t *testing.T) {
	src := windowedMetricSrc(t)
	prog := filepath.Join(t.TempDir(), "win.ladix")
	if err := os.WriteFile(prog, []byte(src), 0o644); err != nil {
		t.Fatalf("запись программы: %v", err)
	}

	const effectLine = "выручка за месяц положительна"

	runAt := func(c fixedClock) string {
		var out, errBuf bytes.Buffer
		code := runFile(prog, "", eval.DefaultMaxDepth, nil, c, &out, &errBuf)
		if code != 0 {
			t.Fatalf("runFile код=%d stderr=%q", code, errBuf.String())
		}
		return out.String()
	}

	may := fixedClock{time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}
	jan := fixedClock{time.Date(2026, 1, 15, 12, 0, 0, 0, time.Local)}

	mayOut := runAt(may)
	if !strings.Contains(mayOut, effectLine) {
		t.Fatalf("T=май: метрика не сработала — дата метрик НЕ от инъектированных часов; out=%q", mayOut)
	}
	// Байт-стабильность: тот же T → тот же вывод.
	if again := runAt(may); again != mayOut {
		t.Fatalf("недетерминизм при одном T: %q != %q", again, mayOut)
	}
	// Смена момента → вывод меняется (метрика гаснет).
	janOut := runAt(jan)
	if strings.Contains(janOut, effectLine) {
		t.Fatalf("T'=январь: метрика сработала, хотя май-записи вне января — часы не следуют T'; out=%q", janOut)
	}
}

// --- T014: metric — дата метрики интерпретатора следует T (engine-часы тоже T, латентно) ---

// TestMetricClockInjected гонит путь metric (runMetric, eval.Clock инъектируем) на той
// же оконной метрике. T=май → значение 100; T'=январь → 0. Так дата метрики идёт от T.
//
// ИНВЕРСИЯ: замени clock на eval.SystemClock{} в runMetric — дата уедет в системный
// месяц, значение станет 0 (вне мая) → assert «100» упадёт.
func TestMetricClockInjected(t *testing.T) {
	src := windowedMetricSrc(t)
	prog := filepath.Join(t.TempDir(), "win.ladix")
	if err := os.WriteFile(prog, []byte(src), 0o644); err != nil {
		t.Fatalf("запись программы: %v", err)
	}

	metricAt := func(d value.Дата) string {
		var out, errBuf bytes.Buffer
		code := runMetric(prog, "выручка_мая", eval.DefaultMaxDepth, eval.FixedClock{D: d}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("runMetric код=%d stderr=%q", code, errBuf.String())
		}
		return strings.TrimSpace(out.String())
	}

	may := metricAt(value.Дата{Year: 2026, Month: 5, Day: 31})
	if may != "100" {
		t.Fatalf("T=май: значение=%q, хотим 100 (дата метрики не от T)", may)
	}
	if again := metricAt(value.Дата{Year: 2026, Month: 5, Day: 31}); again != may {
		t.Fatalf("недетерминизм metric при одном T: %q != %q", again, may)
	}
	jan := metricAt(value.Дата{Year: 2026, Month: 1, Day: 15})
	if jan != "0" {
		t.Fatalf("T'=январь: значение=%q, хотим 0 (часы не следуют T')", jan)
	}
}

// --- T011: start — lifecycle-штампы (CreatedAt/Deadline задачи) следуют T ---

// TestStartClockInjected гонит путь start (startMain с инъектированными engine-часами)
// процесса с человеческим шагом и сроком 3дн. Движок штампует Task.CreatedAt от
// инъектированных часов и считает Deadline = CreatedAt + 3дн. При T задача обязана
// иметь CreatedAt==T и Deadline==T+3дн — детерминированно.
//
// ИНВЕРСИЯ: верни в startMain независимый engine-дефолт (без engine.WithClock(clock))
// — CreatedAt уедет в реальное «сейчас» (≈сегодня, НЕ T) → assert CreatedAt==T упадёт.
func TestStartClockInjected(t *testing.T) {
	prog := examplePath("контроль_плана.ladix")
	T := time.Date(2026, 3, 10, 9, 30, 0, 0, time.Local)

	startAt := func(clk fixedClock) *store.Task {
		db := filepath.Join(t.TempDir(), "start.db")
		var out, errBuf bytes.Buffer
		code := startMain([]string{prog, "эскалация_плана", "2500000", "--db", db}, clk, &out, &errBuf)
		if code != 0 {
			t.Fatalf("startMain код=%d stderr=%q", code, errBuf.String())
		}
		sq, err := store.NewSQLiteStore(db)
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer sq.Close()
		task, err := sq.LoadTask("t-000001")
		if err != nil {
			t.Fatalf("LoadTask: %v", err)
		}
		return task
	}

	task := startAt(fixedClock{T})
	if !task.CreatedAt.Equal(T) {
		t.Fatalf("Task.CreatedAt=%v, хотим T=%v (штамп не от инъектированных часов)", task.CreatedAt, T)
	}
	if task.Deadline == nil || !task.Deadline.Equal(T.Add(3*24*time.Hour)) {
		t.Fatalf("Task.Deadline=%v, хотим T+3дн=%v", task.Deadline, T.Add(3*24*time.Hour))
	}
	// Детерминизм: другой запуск с тем же T → тот же штамп.
	again := startAt(fixedClock{T})
	if !again.CreatedAt.Equal(T) {
		t.Fatalf("недетерминизм start: CreatedAt=%v != T=%v", again.CreatedAt, T)
	}
}

// --- T012: complete — MarkTaskCompleted / UpdatedAt следуют T ---

// TestCompleteClockInjected: start доводит инстанс до открытой задачи (под часами Ts),
// затем complete завершает её под инъектированными часами Tc. Движок штампует
// CompletedAt и UpdatedAt задачи от Tc — обязаны равняться Tc детерминированно.
//
// ИНВЕРСИЯ: убери engine.WithClock(clock) из completeTask (движок на дефолт-реалтайме)
// — CompletedAt уедет в «сейчас» (НЕ Tc) → assert CompletedAt==Tc упадёт.
func TestCompleteClockInjected(t *testing.T) {
	prog := examplePath("контроль_плана.ladix")
	db := filepath.Join(t.TempDir(), "complete.db")
	Ts := fixedClock{time.Date(2026, 2, 1, 8, 0, 0, 0, time.Local)}
	Tc := time.Date(2026, 2, 4, 14, 15, 0, 0, time.Local)

	var so, se bytes.Buffer
	if code := startMain([]string{prog, "эскалация_плана", "2500000", "--db", db}, Ts, &so, &se); code != 0 {
		t.Fatalf("start код=%d stderr=%q", code, se.String())
	}

	var co, ce bytes.Buffer
	if code := completeTask(prog, "t-000001", db, eval.DefaultMaxDepth, `{"итог":"перезвонит"}`, nil, fixedClock{Tc}, &co, &ce); code != 0 {
		t.Fatalf("complete код=%d stderr=%q", code, ce.String())
	}

	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer sq.Close()
	task, err := sq.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if task.CompletedAt == nil || !task.CompletedAt.Equal(Tc) {
		t.Fatalf("CompletedAt=%v, хотим Tc=%v (штамп завершения не от инъектированных часов)", task.CompletedAt, Tc)
	}
	// UpdatedAt живёт на инстансе (ProcessInstance.UpdatedAt): движок штампует его
	// от инъектированных часов перед каждым SaveInstance при продвижении из complete.
	inst, err := sq.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if !inst.UpdatedAt.Equal(Tc) {
		t.Fatalf("Instance.UpdatedAt=%v, хотим Tc=%v", inst.UpdatedAt, Tc)
	}
}

// --- T013: tasks — «сейчас» строки задачи (маркер ПРОСРОЧЕНА) следует T ---

// TestTasksClockInjected: создаём задачу с дедлайном D, затем гоним listTasks с двумя
// инъектированными часами. T_before < D → строки БЕЗ «ПРОСРОЧЕНА»; T_after > D → строка
// С «ПРОСРОЧЕНА». Overdue считается как now.After(Deadline) от часов clock → доказывает,
// что «сейчас» строки задачи идёт от инъектированных часов, а не от реал-тайма.
//
// ИНВЕРСИЯ: верни в listTasks сырой engine.SystemClock{}.Now() — «сейчас» станет
// сегодняшним (НЕ T) → маркер ПРОСРОЧЕНА перестанет следовать T_before/T_after →
// один из двух assert упадёт.
func TestTasksClockInjected(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tasks.db")
	deadline := time.Date(2026, 4, 1, 12, 0, 0, 0, time.Local)
	seedTaskWithDeadline(t, db, deadline)

	listAt := func(clk fixedClock) string {
		var out, errBuf bytes.Buffer
		code := listTasks("", db, clk, &out, &errBuf)
		if code != 0 {
			t.Fatalf("listTasks код=%d stderr=%q", code, errBuf.String())
		}
		return out.String()
	}

	before := listAt(fixedClock{deadline.Add(-24 * time.Hour)})
	if strings.Contains(before, "ПРОСРОЧЕНА") {
		t.Fatalf("T_before<D: задача помечена ПРОСРОЧЕНА — «сейчас» не от инъектированных часов; out=%q", before)
	}
	if again := listAt(fixedClock{deadline.Add(-24 * time.Hour)}); again != before {
		t.Fatalf("недетерминизм tasks при одном T: %q != %q", again, before)
	}
	after := listAt(fixedClock{deadline.Add(24 * time.Hour)})
	if !strings.Contains(after, "ПРОСРОЧЕНА") {
		t.Fatalf("T_after>D: задача НЕ помечена ПРОСРОЧЕНА — «сейчас» не следует T'; out=%q", after)
	}
}

// --- helpers ---

// windowedMetricSrc — программа с оконной метрикой выручка_мая (период ежемесячно +
// по_дате), записями за май 2026 и триггером, печатающим строку при метрике>0.
// Зеркалит testdata/metric_dated.ladix, но самодостаточна (инлайн-источник во tmp).
func windowedMetricSrc(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	srcJSON := filepath.Join(dir, "src.json")
	if err := os.WriteFile(srcJSON,
		[]byte(`[{"дата_заказа":"2026-05-10","сумма_заказа":60},{"дата_заказа":"2026-05-20","сумма_заказа":40}]`),
		0o644); err != nil {
		t.Fatalf("запись источника: %v", err)
	}
	return `источник заказы:
    файл: "` + srcJSON + `"

метрика выручка_мая:
    источник: заказы
    агрегат:  сумма(сумма_заказа)
    период:   ежемесячно
    по_дате:  дата(дата_заказа)

процесс эскалация:
    шаг оповестить:
        печать("выручка за месяц положительна")

когда метрика выручка_мая > 0:
    запустить процесс эскалация
`
}

// seedTaskWithDeadline кладёт в свежую SQLite-БД один инстанс + одну открытую задачу
// t-000001 с заданным дедлайном (прямой SaveInstance/SaveTask, образец inspect_golden).
func seedTaskWithDeadline(t *testing.T, db string, deadline time.Time) {
	t.Helper()
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("open db для сидирования: %v", err)
	}
	defer sq.Close()
	inst := &store.ProcessInstance{
		ID:          "p-000001",
		ProcessName: "эскалация_плана",
		CurrentStep: "связаться_с_клиентом",
		Status:      store.StatusWaiting,
		CreatedAt:   deadline.Add(-3 * 24 * time.Hour),
		UpdatedAt:   deadline.Add(-3 * 24 * time.Hour),
	}
	if err := sq.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	dl := deadline
	task := &store.Task{
		ID:         "t-000001",
		InstanceID: "p-000001",
		StepName:   "связаться_с_клиентом",
		Assignee:   "менеджер",
		Deadline:   &dl,
		Status:     store.TaskPending,
		CreatedAt:  deadline.Add(-3 * 24 * time.Hour),
	}
	if err := sq.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
}
