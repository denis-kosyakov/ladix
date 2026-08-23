package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// readUserVersion открывает отдельное соединение к файлу БД и читает
// PRAGMA user_version. Используется тестами миграций как независимая проверка
// (не через приватные поля SQLiteStore).
func readUserVersion(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q): %v", path, err)
	}
	defer db.Close()
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	return v
}

// countTables возвращает число объектов type='table' с данным именем в
// sqlite_master (0 — таблицы нет, 1 — есть, >1 — дубликат).
func countTables(t *testing.T, path, name string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q): %v", path, err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n); err != nil {
		t.Fatalf("count table %q: %v", name, err)
	}
	return n
}

// TestMigrateFreshDB (контракт A1 · FR-001/FR-004 · SC-001): открытие на свежем
// файле приводит схему к версии currentSchemaVersion=2 и создаёт таблицу outbox.
//
// ИНВЕРСИОННЫЙ ЗАМОК (SC-004): если убрать элемент schemaMigrations (или бамп
// версии), target станет 1 → outbox не создаётся И user_version остаётся 1 → этот
// тест КРАСНЕЕТ на обеих проверках. Мутпроба подтверждена вручную (T011).
func TestMigrateFreshDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore(%q): %v", path, err)
	}
	defer st.Close()

	if got := readUserVersion(t, path); got != 4 {
		t.Errorf("user_version = %d, want 4", got)
	}
	if got := countTables(t, path, "outbox"); got != 1 {
		t.Errorf("outbox table count = %d, want 1", got)
	}
}

// TestMigrateLegacyV0 (контракт A2 + G-A3 · FR-003/FR-008 · SC-002): настоящая
// до-версионная БД — базовые таблицы + ДАННЫЕ присутствуют, outbox ОТСУТСТВУЕТ,
// user_version=0. При повторном открытии: (a) данные целы (data-intact), (b) outbox
// создана из отсутствия и запрашиваема (created-from-absence), (c) user_version==2.
// Замок мутационно-плотный: краснеет, если миграцию 1→2 убрать ИЛИ она не создаст
// outbox — независимо от IF NOT EXISTS (отзыв D-AU-9: НЕ сброс схемы).
func TestMigrateLegacyV0(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Первое открытие создаёт базовые таблицы; пишем данные.
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	inst := &ProcessInstance{
		ID:          "p-000001",
		ProcessName: "контроль",
		Status:      StatusRunning,
		CurrentStep: "связаться",
		Variables:   map[string]value.Value{"факт": value.Целое{V: 42}},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	task := &Task{
		ID:         "t-000001",
		InstanceID: "p-000001",
		StepName:   "связаться",
		Assignee:   "менеджер",
		Status:     TaskPending,
		CreatedAt:  now,
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	// Имитируем настоящую до-версионную БД (M2-эра без user_version): базовые таблицы
	// и ДАННЫЕ есть, но outbox физически ОТСУТСТВУЕТ, а user_version=0. DROP TABLE
	// outbox делает шаг 1→2 «создающим из отсутствия» — мутационно-плотно: тест
	// КРАСНЕЕТ, если миграция 1→2 убрана или не создаёт outbox, НЕЗАВИСИМО от
	// IF NOT EXISTS (на чистом CREATE поведение то же).
	if _, err := st.db.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatalf("reset user_version: %v", err)
	}
	if _, err := st.db.Exec(`DROP TABLE IF EXISTS outbox`); err != nil {
		t.Fatalf("drop outbox: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Санити: outbox действительно отсутствует ДО повторного открытия.
	if got := countTables(t, path, "outbox"); got != 0 {
		t.Fatalf("precondition: outbox count = %d, want 0 (table must be absent)", got)
	}

	// Повторное открытие — миграция 0→1→2.
	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()

	// (c) версия поднялась до 4.
	if got := readUserVersion(t, path); got != 4 {
		t.Errorf("user_version after reopen = %d, want 4", got)
	}
	// (b) outbox создана из отсутствия и реально запрашиваема (SELECT, не только
	// присутствие в sqlite_master).
	if got := countTables(t, path, "outbox"); got != 1 {
		t.Errorf("outbox table count = %d, want 1 (created from absence)", got)
	}
	if err := st2.db.QueryRow(`SELECT count(*) FROM outbox`).Scan(new(int)); err != nil {
		t.Fatalf("outbox not queryable after migrate: %v", err)
	}

	// (a) данные базовых таблиц целы (G-A3 / FR-008).
	gotInst, err := st2.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance after migrate: %v", err)
	}
	if gotInst.ProcessName != "контроль" || gotInst.CurrentStep != "связаться" {
		t.Errorf("instance corrupted: %+v", gotInst)
	}
	if v, ok := gotInst.Variables["факт"]; !ok || v.(value.Целое).V != 42 {
		t.Errorf("instance variable lost/changed: %+v", gotInst.Variables)
	}
	gotTask, err := st2.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask after migrate: %v", err)
	}
	if gotTask.Assignee != "менеджер" || gotTask.StepName != "связаться" {
		t.Errorf("task corrupted: %+v", gotTask)
	}
}

// TestMigrateIdempotent (контракт A4 + B-I4 · FR-007 · SC-003): повторное открытие
// уже актуальной (версия 2) БД — чистый no-op: версия остаётся 2, outbox не
// дублируется, ошибок нет.
func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idem.db")

	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("second open (idempotent): %v", err)
	}
	defer st2.Close()

	if got := readUserVersion(t, path); got != 4 {
		t.Errorf("user_version after second open = %d, want 4", got)
	}
	if got := countTables(t, path, "outbox"); got != 1 {
		t.Errorf("outbox table count = %d, want 1 (no duplicate)", got)
	}
}

// TestSchemaVersionInvariant (INV-R1): const currentSchemaVersion обязана РОВНО
// равняться производной от реестра версии baselineVersion+len(schemaMigrations).
// Этот замок краснеет при молчаливом дрейфе (шаг добавлен/убран без бампа const),
// дублируя в тесте старт-чек init() в sqlite.go (который иначе паникует при запуске).
func TestSchemaVersionInvariant(t *testing.T) {
	if want := baselineVersion + len(schemaMigrations); currentSchemaVersion != want {
		t.Errorf("INV-R1 нарушен: currentSchemaVersion=%d, baselineVersion+len(schemaMigrations)=%d",
			currentSchemaVersion, want)
	}
}

// TestMigrateOutboxErrorTextV3toV4 (029 Уровень 2 · миграция 3→4): аддитивная nullable-
// колонка outbox.error_text. Старая БД (v3, колонки нет) с ДОСТАВЛЕННОЙ строкой: при
// реоткрытии колонка добавляется, СТАРАЯ строка читается с ErrorText="" и Delivered=true
// (прежнее поведение — успех; старые строки трактуются как delivered). ИНВЕРСИОННЫЙ
// ЗАМОК: убрать ступень 3→4 → колонки нет → LoadOutbox SELECT error_text падает на
// реоткрытии И user_version остаётся 3 → тест краснеет.
func TestMigrateOutboxErrorTextV3toV4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "errtext.db")

	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	rec := &OutboxRecord{
		DedupKey:    "p-000001|шаг|0",
		InstanceID:  "p-000001",
		StepName:    "шаг",
		EffectIndex: 0,
		Kind:        "уведомить",
		Target:      "crm",
		Result:      value.None,
		Delivered:   true,
		CreatedAt:   now,
		DeliveredAt: &now,
	}
	if err := st.SaveOutbox(rec); err != nil {
		t.Fatalf("SaveOutbox: %v", err)
	}

	// Имитируем настоящую v3-БД: снимаем error_text и откатываем версию на 3 (первое
	// открытие уже довело схему до v4 с колонкой; v3 её не имела).
	if _, err := st.db.Exec(`ALTER TABLE outbox DROP COLUMN error_text`); err != nil {
		t.Fatalf("drop error_text (имитация v3): %v", err)
	}
	if _, err := st.db.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatalf("reset user_version=3: %v", err)
	}
	// Санити: колонки нет — выборка падает.
	if _, err := st.db.Exec(`SELECT error_text FROM outbox LIMIT 1`); err == nil {
		t.Fatalf("precondition: error_text всё ещё присутствует (DROP не сработал)")
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Реоткрытие — миграция 3→4 добавляет колонку поверх существующих данных.
	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen (migrate 3→4): %v", err)
	}
	defer st2.Close()

	if got := readUserVersion(t, path); got != 4 {
		t.Errorf("user_version after reopen = %d, want 4", got)
	}
	// Старая строка читается: ErrorText="" (NULL), Delivered=true — поведение прежнее.
	got, err := st2.LoadOutbox("p-000001|шаг|0")
	if err != nil {
		t.Fatalf("LoadOutbox после миграции 3→4: %v", err)
	}
	if !got.Delivered {
		t.Errorf("старая строка Delivered=%v, хотим true (трактуется как успех)", got.Delivered)
	}
	if got.ErrorText != "" {
		t.Errorf("старая строка ErrorText=%q, хотим \"\" (NULL)", got.ErrorText)
	}
}
