package daemon

import (
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// restartProcSrc — процесс с автоматическим первым шагом (печать-маркер RESTART, без
// исполнителя) и человеческим вторым. Top-level НЕ запускает процесс: рестарт-скан
// поднимает только инстансы, фабрикованные тестом напрямую в Store (имитация залипших
// после прерванного прогона).
const restartProcSrc = `процесс заявка(клиент):
    шаг подготовить:
        печать("RESTART " + клиент)
    шаг согласовать после подготовить:
        исполнитель: "менеджер"
`

// stuckInst фабрикует инстанс в данном статусе на данном шаге (запись, оставшаяся в БД
// от прерванного прогона до подъёма демона).
func stuckInst(id, step string, status store.Status) *store.ProcessInstance {
	return &store.ProcessInstance{
		ID:          id,
		ProcessName: "заявка",
		Status:      status,
		CurrentStep: step,
		Variables:   map[string]value.Value{"клиент": value.Строка{V: "ООО"}},
	}
}

// eachStore прогоняет тело на ОБОИХ бэкендах Store (паритет Memory+SQLite, как 006).
func eachStore(t *testing.T, fn func(t *testing.T, st store.Store)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) { fn(t, store.NewMemoryStore()) })
	t.Run("sqlite", func(t *testing.T) {
		sq, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "restart.db"))
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}
		defer sq.Close()
		fn(t, sq)
	})
}

// TestRunRestartScanValidStep — инстанс «выполняется» с валидным CurrentStep →
// реактивирован (advance догоняет до ожидания, тело исполнено). FR-019, SC-008.
func TestRunRestartScanValidStep(t *testing.T) {
	eachStore(t, func(t *testing.T, st store.Store) {
		out := &countWriter{marker: "RESTART"}
		if err := st.SaveInstance(stuckInst("p-000001", "подготовить", store.StatusRunning)); err != nil {
			t.Fatalf("SaveInstance: %v", err)
		}
		d, _ := buildDaemon(t, restartProcSrc, st, out)

		d.RunRestartScan()

		if out.count() != 1 {
			t.Fatalf("ожидалось ровно одно исполнение тела шага, получено %d; out=%q", out.count(), out.String())
		}
		got, err := st.LoadInstance("p-000001")
		if err != nil {
			t.Fatalf("LoadInstance: %v", err)
		}
		if got.Status != store.StatusWaiting || got.CurrentStep != "согласовать" {
			t.Fatalf("инстанс не догнан: статус=%q шаг=%q", got.Status, got.CurrentStep)
		}
	})
}

// TestRunRestartScanDrift — CurrentStep отсутствует в перезагруженном определении →
// лог расхождения, инстанс залип (не тронут), скан не паникует и не прерывает старт.
// FR-020, SC-008.
func TestRunRestartScanDrift(t *testing.T) {
	eachStore(t, func(t *testing.T, st store.Store) {
		out := &countWriter{marker: "RESTART"}
		if err := st.SaveInstance(stuckInst("p-000001", "удалённый_шаг", store.StatusRunning)); err != nil {
			t.Fatalf("SaveInstance: %v", err)
		}
		d, _ := buildDaemon(t, restartProcSrc, st, out)

		d.RunRestartScan() // не должен паниковать

		if out.count() != 0 {
			t.Fatalf("дрейф-инстанс не должен исполнять тело, out=%q", out.String())
		}
		if !out.contains("дрейф исходника") {
			t.Fatalf("ожидался лог расхождения о дрейфе, out=%q", out.String())
		}
		got, _ := st.LoadInstance("p-000001")
		if got.Status != store.StatusRunning || got.CurrentStep != "удалённый_шаг" {
			t.Fatalf("дрейф-инстанс изменён: статус=%q шаг=%q", got.Status, got.CurrentStep)
		}
	})
}

// TestRunRestartScanIgnoresWaiting — инстанс «ожидает» НЕ сканируется (корректен,
// проснётся по complete): тело не исполняется, инстанс не тронут. FR-019.
func TestRunRestartScanIgnoresWaiting(t *testing.T) {
	eachStore(t, func(t *testing.T, st store.Store) {
		out := &countWriter{marker: "RESTART"}
		if err := st.SaveInstance(stuckInst("p-000001", "согласовать", store.StatusWaiting)); err != nil {
			t.Fatalf("SaveInstance: %v", err)
		}
		d, _ := buildDaemon(t, restartProcSrc, st, out)

		d.RunRestartScan()

		if out.count() != 0 {
			t.Fatalf("инстанс 'ожидает' не должен реактивироваться, out=%q", out.String())
		}
		got, _ := st.LoadInstance("p-000001")
		if got.Status != store.StatusWaiting {
			t.Fatalf("инстанс 'ожидает' тронут: статус=%q", got.Status)
		}
	})
}

// TestRunRestartScanDeterministicOrder — несколько залипших инстансов обрабатываются в
// детерминированном порядке (по возрастанию ID), смесь валидных и дрейф не роняет скан.
func TestRunRestartScanDeterministicOrder(t *testing.T) {
	eachStore(t, func(t *testing.T, st store.Store) {
		out := &countWriter{marker: "RESTART"}
		// Два валидных + один дрейф в статусе «выполняется».
		for _, inst := range []*store.ProcessInstance{
			stuckInst("p-000001", "подготовить", store.StatusRunning),
			stuckInst("p-000002", "удалённый_шаг", store.StatusRunning),
			stuckInst("p-000003", "подготовить", store.StatusRunning),
		} {
			if err := st.SaveInstance(inst); err != nil {
				t.Fatalf("SaveInstance: %v", err)
			}
		}
		d, _ := buildDaemon(t, restartProcSrc, st, out)

		d.RunRestartScan()

		// Два валидных реактивированы (2 исполнения тела), дрейф залип с логом.
		if out.count() != 2 {
			t.Fatalf("ожидалось 2 исполнения тела (валидные), получено %d; out=%q", out.count(), out.String())
		}
		if !out.contains("дрейф исходника") {
			t.Fatalf("ожидался лог дрейфа для p-000002, out=%q", out.String())
		}
		drift, _ := st.LoadInstance("p-000002")
		if drift.Status != store.StatusRunning {
			t.Fatalf("дрейф-инстанс p-000002 изменён: статус=%q", drift.Status)
		}
	})
}
