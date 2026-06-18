package daemon

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// checkdeadlines_fault_test.go — реальные fault-тесты 3 веток checkDeadlines (M3-C2b
// §C-2b.8, contracts/checkdeadlines-faults.md). Функция корректна; эти замки
// доказывают устойчивость каждой ветки к сбою Store: нет паники, точные лог-строки,
// прочие задачи обработаны. fault-Store — ручная обёртка над MemoryStore (0 новых
// зависимостей, без mock-библиотек); ошибка инжектируется в ОДИН метод по флагу.

// faultStore оборачивает store.Store и подменяет один метод управляемой ошибкой.
// Флаги взводятся ПОСЛЕ прайминга (eng.Start использует те же методы штатно).
type faultStore struct {
	store.Store
	failList      bool   // ListPendingTasks → ошибка (ветка 1)
	failLoadInst  string // LoadInstance(этот ID) → ошибка (ветка 2); "" = не падать
	failSaveTask  bool   // SaveTask → ошибка (ветка 3)
	saveTaskCalls int
	listErr       error
	loadInstErr   error
	saveTaskErr   error
}

func (s *faultStore) ListPendingTasks(assignee string) ([]*store.Task, error) {
	if s.failList {
		return nil, s.listErr
	}
	return s.Store.ListPendingTasks(assignee)
}

func (s *faultStore) LoadInstance(id string) (*store.ProcessInstance, error) {
	if s.failLoadInst != "" && id == s.failLoadInst {
		return nil, s.loadInstErr
	}
	return s.Store.LoadInstance(id)
}

func (s *faultStore) SaveTask(t *store.Task) error {
	s.saveTaskCalls++
	if s.failSaveTask {
		return s.saveTaskErr
	}
	return s.Store.SaveTask(t)
}

// TestCheckDeadlinesListError — ВЕТКА 1 (checkdeadlines.go:38-41): ListPendingTasks
// возвращает ошибку → лог «checkDeadlines: листинг задач: %s» + ранний return; фаза
// НЕ паникует, демон жив (следующий тик идёт штатно).
func TestCheckDeadlinesListError(t *testing.T) {
	out := &countWriter{marker: "checkDeadlines: листинг задач:"}
	fs := &faultStore{Store: store.NewMemoryStore(), listErr: errors.New("БД недоступна")}
	d, clk, eng, _ := buildDeadlineDaemon(t, escalationSrc, fs, out)

	created := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, created)
	if _, err := eng.Start("эскалация_плана", []value.Value{value.Целое{V: 2500000}}); err != nil {
		t.Fatalf("eng.Start: %v", err)
	}

	// Взвести fault ПОСЛЕ прайминга; часы за срок.
	fs.failList = true
	setClock(clk, created.Add(3*24*time.Hour))

	// НЕ должно паниковать (если паникует — тест упадёт через recover отсутствие).
	d.tick()

	if !strings.Contains(out.String(), "checkDeadlines: листинг задач: БД недоступна") {
		t.Fatalf("ветка 1: лог-строка листинга отсутствует: %q", out.String())
	}

	// Демон жив: снять fault → следующий тик эскалирует штатно (РОВНО раз).
	fs.failList = false
	d.tick()
	pend, _ := fs.Store.ListPendingTasks("")
	if len(pend) != 1 || !pend[0].Escalated {
		t.Fatalf("ветка 1: после снятия fault эскалация не прошла (демон не жив?): %+v", pend)
	}
}

// TestCheckDeadlinesLoadInstanceError — ВЕТКА 2 (checkdeadlines.go:50-53): LoadInstance
// падает для ОДНОЙ задачи → continue (задача пропущена), нет эскалации этой, нет
// паники, ОСТАЛЬНЫЕ задачи обработаны.
func TestCheckDeadlinesLoadInstanceError(t *testing.T) {
	out := &countWriter{marker: "[уведомление] руководитель:"}
	fs := &faultStore{Store: store.NewMemoryStore(), loadInstErr: errors.New("инстанс не читается")}
	d, clk, eng, _ := buildDeadlineDaemon(t, escalationSrc, fs, out)

	created := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, created)
	// Два инстанса → две задачи (t-000001/p-000001 и t-000002/p-000002).
	if _, err := eng.Start("эскалация_плана", []value.Value{value.Целое{V: 111}}); err != nil {
		t.Fatalf("eng.Start #1: %v", err)
	}
	if _, err := eng.Start("эскалация_плана", []value.Value{value.Целое{V: 222}}); err != nil {
		t.Fatalf("eng.Start #2: %v", err)
	}

	// LoadInstance падает только для p-000001 → его задача пропущена; p-000002 эскалируется.
	fs.failLoadInst = "p-000001"
	setClock(clk, created.Add(3*24*time.Hour))
	d.tick()

	// РОВНО одна эскалация (только p-000002, факт=222).
	if out.count() != 1 {
		t.Fatalf("ветка 2: эскалаций = %d, хотим 1 (одна задача пропущена по LoadInstance, другая обработана); out=%q", out.count(), out.String())
	}
	if !strings.Contains(out.String(), "222") {
		t.Fatalf("ветка 2: обработана не та задача (ждали факт=222): %q", out.String())
	}
	// Задача p-000001 НЕ эскалирована (пропущена).
	pend, _ := fs.Store.ListPendingTasks("")
	for _, p := range pend {
		if p.InstanceID == "p-000001" && p.Escalated {
			t.Fatalf("ветка 2: задача с несчитываемым инстансом всё же эскалирована: %+v", p)
		}
	}
}

// TestCheckDeadlinesSaveTaskError — ВЕТКА 3 (checkdeadlines.go:63-65): SaveTask
// (персист Escalated) падает ПОСЛЕ срабатывания тела (POST/уведомление УЖЕ отправлено)
// → лог «checkDeadlines: персист Escalated задачи %s: %s».
//
// ИЗВЕСТНОЕ ОКНО fire-then-persist (НЕ дефект, пара к §C-2b.5 dispatch-зазору / §C-9
// бэклог): тело сработало, но Escalated не персистнут → следующий тик/рестарт ПЕРЕШЛЁТ
// эскалацию = at-least-once. Закрывается лишь идемпотентностью приёмника (не владеем).
func TestCheckDeadlinesSaveTaskError(t *testing.T) {
	out := &countWriter{marker: "[уведомление] руководитель:"}
	fs := &faultStore{Store: store.NewMemoryStore(), saveTaskErr: errors.New("диск полон")}
	d, clk, eng, _ := buildDeadlineDaemon(t, escalationSrc, fs, out)

	created := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, created)
	if _, err := eng.Start("эскалация_плана", []value.Value{value.Целое{V: 2500000}}); err != nil {
		t.Fatalf("eng.Start: %v", err)
	}

	// Взвести fault SaveTask ПОСЛЕ прайминга (Start уже сохранил задачу штатно).
	fs.failSaveTask = true
	setClock(clk, created.Add(3*24*time.Hour))
	d.tick()

	// Тело УЖЕ сработало (fire-then-persist): уведомление отправлено.
	if out.count() != 1 {
		t.Fatalf("ветка 3: тело эскалации не сработало до сбоя SaveTask: out=%q", out.String())
	}
	// Лог-строка сбоя персиста присутствует (дословно §C-2b.8).
	if !strings.Contains(out.String(), "checkDeadlines: персист Escalated задачи t-000001: диск полон") {
		t.Fatalf("ветка 3: лог-строка сбоя персиста Escalated отсутствует: %q", out.String())
	}
}
