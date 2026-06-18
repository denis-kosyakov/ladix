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

	if got := readUserVersion(t, path); got != 2 {
		t.Errorf("user_version = %d, want 2", got)
	}
	if got := countTables(t, path, "outbox"); got != 1 {
		t.Errorf("outbox table count = %d, want 1", got)
	}
}

// TestMigrateLegacyV0 (контракт A2 + G-A3 · FR-003/FR-008 · SC-002): база с
// базовыми таблицами, данными и user_version=0 (до-версионная) при повторном
// открытии поднимается до версии 2, outbox появляется, а ранее записанные данные
// (инстанс + задача) сохраняются без изменений (отзыв D-AU-9: НЕ сброс схемы).
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

	// Сбросить версию в 0 — имитируем до-версионную БД (M2-эра без user_version).
	if _, err := st.db.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatalf("reset user_version: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Повторное открытие — миграция 0→1→2.
	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()

	if got := readUserVersion(t, path); got != 2 {
		t.Errorf("user_version after reopen = %d, want 2", got)
	}
	if got := countTables(t, path, "outbox"); got != 1 {
		t.Errorf("outbox table count = %d, want 1", got)
	}

	// Данные базовых таблиц целы (G-A3 / FR-008).
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

	if got := readUserVersion(t, path); got != 2 {
		t.Errorf("user_version after second open = %d, want 2", got)
	}
	if got := countTables(t, path, "outbox"); got != 1 {
		t.Errorf("outbox table count = %d, want 1 (no duplicate)", got)
	}
}
