package store

import (
	stderrors "errors"
	"path/filepath"
	"testing"
)

// countTriggerStateRows возвращает число строк в trigger_state (независимая проверка
// сброса позиционного состояния миграцией 2→3).
func countTriggerStateRows(t *testing.T, st *SQLiteStore) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM trigger_state`).Scan(&n); err != nil {
		t.Fatalf("count trigger_state: %v", err)
	}
	return n
}

// TestMigrateTriggerRekeyV2toV3 (§FR-009): миграция 2→3 — ре-кей триггеров на
// контентные ключи. До-фичная БД (v2) держала позиционные строки trigger_state
// («trg-0», «trg-1»). Шаг 2→3 (`DELETE FROM trigger_state`) ОЧИЩАЕТ их: позиционные
// ключи больше не действительны, состояние перепраймится демоном под контентными
// ключами на первом тике (поведенчески-нейтрально, FR-010).
//
// ИНВЕРСИОННЫЙ ЗАМОК: убрать ступень 2→3 из schemaMigrations (или бамп версии) →
// target=2 → позиционные строки УЦЕЛЕЮТ И user_version останется 2 → этот тест
// краснеет на обеих проверках (rows!=0 и version!=3).
func TestMigrateTriggerRekeyV2toV3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rekey.db")

	// Первое открытие создаёт схему и доводит до currentSchemaVersion.
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	// Фабрикуем до-версионную БД v2: позиционные строки trigger_state + откат версии
	// на 2 (как если бы БД создана фичей-предшественником, до ре-кея). Имитирует
	// настоящий повтор-апгрейд: данные триггеров под старыми ключами «trg-0/1».
	for _, id := range []string{"trg-0", "trg-1"} {
		bt := true
		if err := st.SaveTriggerState(&TriggerState{TriggerID: id, Kind: "metric", LastBool: &bt}); err != nil {
			t.Fatalf("seed trigger_state %q: %v", id, err)
		}
	}
	if n := countTriggerStateRows(t, st); n != 2 {
		t.Fatalf("precondition: trigger_state rows = %d, want 2 (позиционные ключи засеяны)", n)
	}
	if _, err := st.db.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatalf("reset user_version=2: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Санити: до повторного открытия версия действительно 2, строки на месте.
	if got := readUserVersion(t, path); got != 2 {
		t.Fatalf("precondition: user_version = %d, want 2", got)
	}

	// Повторное открытие — миграция 2→3 (DELETE FROM trigger_state).
	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen (migrate 2→3): %v", err)
	}
	defer st2.Close()

	// (a) версия поднялась до 3.
	if got := readUserVersion(t, path); got != 3 {
		t.Errorf("user_version after reopen = %d, want 3", got)
	}
	// (b) trigger_state ОЧИЩЕН: позиционные ключи сброшены → LoadTriggerState не находит.
	if n := countTriggerStateRows(t, st2); n != 0 {
		t.Errorf("trigger_state rows after migrate = %d, want 0 (позиционные ключи сброшены)", n)
	}
	for _, id := range []string{"trg-0", "trg-1"} {
		if _, err := st2.LoadTriggerState(id); !stderrors.Is(err, ErrTriggerStateNotFound) {
			t.Errorf("LoadTriggerState(%q) после ре-кея: err=%v, want ErrTriggerStateNotFound", id, err)
		}
	}

	// (c) после ре-кея состояние под КОНТЕНТНЫМ ключом пишется и переживает рестарт.
	contentKey := "trg-deadbeefcafe0001"
	bt := true
	if err := st2.SaveTriggerState(&TriggerState{TriggerID: contentKey, Kind: "metric", LastBool: &bt}); err != nil {
		t.Fatalf("SaveTriggerState contentKey: %v", err)
	}
	if err := st2.Close(); err != nil {
		t.Fatalf("close st2: %v", err)
	}
	st3, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("third open: %v", err)
	}
	defer st3.Close()
	ts, err := st3.LoadTriggerState(contentKey)
	if err != nil {
		t.Fatalf("LoadTriggerState contentKey после рестарта: %v", err)
	}
	if ts.LastBool == nil || !*ts.LastBool {
		t.Fatalf("контентный ключ не пережил рестарт: LastBool=%v, хотим true", ts.LastBool)
	}
}
