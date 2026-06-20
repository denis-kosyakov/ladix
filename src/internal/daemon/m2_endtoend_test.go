package daemon

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// m2_endtoend_test.go — ГЕЙТ-GOLDEN вехи M2 «Автоматизация» (§AU-12.C end-to-end DoD).
// Сшивает В ОДНУ цепочку все шесть подфич B1–B6 на детерминированном стенде (FixedClock
// движка+демона, in-process httptest.Server, реальный SQLiteStore в t.TempDir(), прямой
// прогон d.tick() — надёжнее живого serve по таймингам, прецедент TestDeadlineDurableRestart):
//
//	1. реальный CSV → окно-метрика (М1): источник заказы.csv питает метрику сумма(...);
//	2. §AU-12.A метрика НЕ молчит: CSV стартует ВЫШЕ плана (cur=ложь, прайминг), затем
//	   данные ПАДАЮТ НИЖЕ плана → фронт ложь→истина → edge fire РОВНО один раз;
//	3. метрика-триггер `когда метрика … < план: запустить процесс эскалация_плана(значение)`
//	   → инстанс p-000001, переменная факт=значение (снимок метрики);
//	4. процесс с человеческим шагом связаться_с_клиентом (исполнитель «менеджер», срок 3дн)
//	   → задача t-000001 со сроком created+3дн;
//	5. Clock += 3дн (за срок) → 4-я фаза tick checkDeadlines→fireDeadlineBody исполняет
//	   тело `уведомить руководитель(факт)`;
//	6. РЕАЛЬНЫЙ эффект через httptest: уведомить под webhookCaller(httptest.URL) шлёт
//	   реальный POST {"цель":"руководитель","данные":[<факт>]} на тест-сервер (тело захвачено);
//	7. §AU-12.B durable×рестарт: эскалация РОВНО раз; рестарт store/демона → НЕТ повтора.
//
// CLI-форма (start/complete --data/inspect + §AU-12.B CLI start durable) — companion
// cmd/ladix/m2_golden_test.go. Прод-логика НЕ тронута: тест только наблюдает.

// advanceClock — мутабельные управляемые часы (engine.Clock) для пошагового продвижения
// в end-to-end golden. (fixedClock пакета уже даёт mu+advance — переиспользуем его.)

// webhookSink — in-process httptest.Server, фиксирующий полученные POST-тела/методы
// (потокобезопасно). Доказывает РЕАЛЬНЫЙ сетевой эффект уведомить под webhookCaller (B2).
type webhookSink struct {
	mu      sync.Mutex
	bodies  []string
	methods []string
}

func (s *webhookSink) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(b))
		s.methods = append(s.methods, req.Method)
		s.mu.Unlock()
		io.WriteString(w, "") // пустое тело ответа → Пусто (best-effort уведомить)
	}
}

func (s *webhookSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func (s *webhookSink) snapshot() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...), append([]string(nil), s.methods...)
}

// buildE2EDaemon собирает daemon-стек поверх данного Store с фиксированными часами И
// инъектированным ExternalCaller (httptest-вебхук): уведомить/вызвать тел триггеров
// уходят реальным POST. Зеркало buildDeadlineDaemon, но с WithExternalCaller (B2) и
// возвратом движка/Store для прямого eng.Start не требуется — top-level НЕ исполняется
// (interp.Run связывает глобали; инстансы родятся из метрика-триггера в demаемоне).
func buildE2EDaemon(t *testing.T, src string, st store.Store, caller engine.ExternalCaller, out io.Writer) (*Daemon, *fixedClock) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	clk := &fixedClock{t: time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)}
	interp := eval.NewInterpreter(out, 0, eval.SystemClock{})
	eng := engine.NewEngine(st, interp, out, engine.WithClock(clk), engine.WithExternalCaller(caller))
	interp.SetProcessRuntime(eng)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	if err := interp.Run(prog); err != nil {
		t.Fatalf("interp.Run: %s", err.Error())
	}
	d := New(st, eng, interp, clk, time.Minute, out)
	return d, clk
}

// m2GoldenSrc собирает программу гейт-среза §AU-12.C: CSV-источник заказы (путь
// абсолютный, подставляется тестом) → метрика план_контроль = сумма(сумма_заказа)
// (БЕЗ окна/периода — дата-независима, edge правит CSV) → метрика-триггер «< план»
// запускает эскалация_плана(значение) → процесс с человеческим шагом+срок 3дн →
// эскалация-триггер уведомляет руководителя переменной факт.
func m2GoldenSrc(csvPath string, план int) string {
	return "" +
		"источник заказы:\n" +
		"    файл: \"" + csvPath + "\"\n" +
		"    тип: csv\n" +
		"    поля:\n" +
		"        сумма_заказа: Дробное\n" +
		"\n" +
		"метрика план_контроль:\n" +
		"    источник: заказы\n" +
		"    агрегат:  сумма(сумма_заказа)\n" +
		"\n" +
		"когда метрика план_контроль < " + itoa(план) + ":\n" +
		"    запустить процесс эскалация_плана(значение)\n" +
		"\n" +
		"процесс эскалация_плана(факт):\n" +
		"    шаг связаться_с_клиентом:\n" +
		"        исполнитель: \"менеджер\"\n" +
		"        срок:        3дн\n" +
		"\n" +
		"когда задача просрочена в эскалация_плана.связаться_с_клиентом:\n" +
		"    уведомить руководитель(факт)\n"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// writeCSV пишет CSV-источник во временный файл (фикстура метрики; первая строка —
// заголовок «сумма_заказа», далее по одной сумме на строку).
func writeCSV(t *testing.T, path string, amounts ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("сумма_заказа\n")
	for _, a := range amounts {
		b.WriteString(a + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("запись CSV %s: %v", path, err)
	}
}

// TestM2GoldenEndToEnd — ТЕРМИНАЛЬНЫЙ гейт-критерий M2 (§AU-12.C). Прогоняет ВСЮ цепочку
// CSV→метрика-edge→start→человеческая задача со сроком→эскалация→РЕАЛЬНЫЙ webhook POST,
// затем рестарт без повтора (§AU-12.B). Каждая стадия — отдельный замок.
func TestM2GoldenEndToEnd(t *testing.T) {
	const план = 2_000_000
	const wantBody = `{"цель":"руководитель","данные":[1500000]}` // факт = снимок метрики (Дробное 1500000)

	dir := t.TempDir()
	csv := filepath.Join(dir, "заказы.csv")
	db := filepath.Join(dir, "demo.db")
	src := m2GoldenSrc(csv, план)

	sink := &webhookSink{}
	srv := httptest.NewServer(sink.handler())
	defer srv.Close()
	caller := engine.NewWebhookCaller(srv.URL, &http.Client{Timeout: 2 * time.Second})

	// --- Прогон 1: единый Store/демон на demo.db ---
	st1, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStore #1: %v", err)
	}
	var log1 strings.Builder
	d1, clk1 := buildE2EDaemon(t, src, st1, caller, &log1)
	created := clk1.Now()

	// Стадия 1+2 (§AU-12.A прайминг): CSV ВЫШЕ плана (сумма=3_000_000 > 2_000_000 →
	// условие «< план» = ложь). Первый тик праймит базу false, НЕ фаерит, НЕ молчит позже.
	writeCSV(t, csv, "3000000")
	d1.tick()
	if n := len(instancesOf(t, st1)); n != 0 {
		t.Fatalf("стадия 2 прайминг: инстансов = %d, хотим 0 (до пересечения — тишина)", n)
	}

	// Стадия 2 (§AU-12.A пересечение): CSV ПАДАЕТ НИЖЕ плана (сумма=1_500_000 < 2_000_000
	// → условие «< план» = истина). Фронт ложь→истина → edge fire РОВНО один раз →
	// запустить процесс эскалация_плана(значение=1500000).
	writeCSV(t, csv, "1500000")
	d1.tick()

	// Стадия 3: инстанс p-000001 создан, факт=значение метрики.
	insts := instancesOf(t, st1)
	if len(insts) != 1 {
		t.Fatalf("стадия 3: инстансов после edge = %d, хотим 1; log=%q", len(insts), log1.String())
	}
	p1 := insts[0]
	if p1.ID != "p-000001" || p1.ProcessName != "эскалация_плана" {
		t.Fatalf("стадия 3: инстанс=%s процесс=%s, хотим p-000001/эскалация_плана", p1.ID, p1.ProcessName)
	}

	// Стадия 4: человеческая задача t-000001 со сроком created+3дн, исполнитель «менеджер».
	tasks := pendingOf(t, st1)
	if len(tasks) != 1 {
		t.Fatalf("стадия 4: открытых задач = %d, хотим 1", len(tasks))
	}
	task := tasks[0]
	if task.Assignee != "менеджер" {
		t.Fatalf("стадия 4: исполнитель=%q, хотим «менеджер»", task.Assignee)
	}
	if task.Deadline == nil {
		t.Fatalf("стадия 4: у задачи нет срока")
	}
	wantDL := created.Add(3 * 24 * time.Hour)
	if !task.Deadline.Equal(wantDL) {
		t.Fatalf("стадия 4: срок=%v, хотим created+3дн=%v", task.Deadline, wantDL)
	}

	// §AU-12.A: edge не молчит ПОВТОРНО — ре-арм истина→истина не дублирует (всё ещё <план).
	d1.tick()
	if n := len(instancesOf(t, st1)); n != 1 {
		t.Fatalf("§AU-12.A ре-арм: инстансов = %d, хотим 1 (edge не повторяет)", n)
	}

	// До срока — тишина по эскалации (webhook не получал POST).
	if sink.count() != 0 {
		t.Fatalf("до срока: webhook получил %d POST, хотим 0 (тишина)", sink.count())
	}

	// Стадия 5+6: Clock += 3дн (за срок) → tick → 4-я фаза checkDeadlines→fireDeadlineBody
	// → уведомить руководитель(факт) → РЕАЛЬНЫЙ POST на httptest.
	setClock(clk1, created.Add(3*24*time.Hour+time.Minute))
	d1.tick()
	if sink.count() != 1 {
		t.Fatalf("стадия 5/6: webhook получил %d POST, хотим РОВНО 1; log=%q", sink.count(), log1.String())
	}
	bodies, methods := sink.snapshot()
	if methods[0] != http.MethodPost {
		t.Fatalf("стадия 6: метод=%q, хотим POST", methods[0])
	}
	if bodies[0] != wantBody {
		t.Fatalf("стадия 6: тело POST=%q, хотим %q", bodies[0], wantBody)
	}

	// Задача помечена Escalated и персистнута (durable).
	task2 := pendingOf(t, st1)[0]
	if !task2.Escalated {
		t.Fatalf("стадия 7: задача НЕ помечена Escalated после fire")
	}

	// Повторный тик БЕЗ рестарта → durable-фильтр Escalated → continue → нет повтора.
	d1.tick()
	if sink.count() != 1 {
		t.Fatalf("повторный тик без рестарта: webhook POST = %d, хотим 1 (одноразовость)", sink.count())
	}
	if err := st1.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}

	// --- Прогон 2 (§AU-12.B РЕСТАРТ): новый SQLiteStore на той же --db ---
	st2, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStore #2 (рестарт): %v", err)
	}
	defer st2.Close()

	// (б) персист: Escalated читается true из той же БД.
	rt, err := st2.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask после рестарта: %v", err)
	}
	if !rt.Escalated {
		t.Fatalf("§AU-12.B: Escalated НЕ персистнут между прогонами (durable нарушен)")
	}

	var log2 strings.Builder
	d2, clk2 := buildE2EDaemon(t, src, st2, caller, &log2)
	// Часы рестарт-демона уже за сроком; CSV всё ещё < план.
	setClock(clk2, created.Add(3*24*time.Hour+time.Minute))
	d2.RunRestartScan()
	d2.tick()

	// §AU-12.B: рестарт НЕ повторяет эскалацию — webhook суммарно получил РОВНО 1 POST.
	if sink.count() != 1 {
		t.Fatalf("§AU-12.B ПОВТОР после рестарта: webhook POST = %d, хотим РОВНО 1 за оба прогона; log2=%q", sink.count(), log2.String())
	}
}

// instancesOf возвращает все инстансы из Store (по обоим статусам, детерминированно).
func instancesOf(t *testing.T, st store.Store) []*store.ProcessInstance {
	t.Helper()
	var all []*store.ProcessInstance
	for _, s := range []store.Status{store.StatusCreated, store.StatusRunning, store.StatusWaiting, store.StatusDone, store.StatusFailed} {
		lst, err := st.ListInstancesByStatus(string(s))
		if err != nil {
			t.Fatalf("ListInstancesByStatus(%s): %v", s, err)
		}
		all = append(all, lst...)
	}
	return all
}

// pendingOf возвращает все открытые задачи (без assignee-фильтра).
func pendingOf(t *testing.T, st store.Store) []*store.Task {
	t.Helper()
	tasks, err := st.ListPendingTasks("")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	return tasks
}
