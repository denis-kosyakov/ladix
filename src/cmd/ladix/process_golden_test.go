package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

// T021/T022 (US3 класс 4, §VF-…) — golden ПОЛНОГО жизненного цикла процесса на
// витринном examples/процесс.ladix: run --db (старт инстанса) → tasks --db (список) →
// complete <task-id> --db (завершение шага → ПРОБУЖДЕНИЕ следующего) → повторный tasks
// (следующий шаг уже открыт) → complete последнего → tasks (пусто).
//
// Детерминизм (зеркало trigger_golden_test): БД — свежий файл в t.TempDir() (на свежем
// Store id монотонны p-000001/t-000001/t-000002); шаги без `срок` → строка задачи без
// дедлайна → масок времени нет. Несмотря на стабильность id на свежей БД, прогоняем
// весь stdout через maskIDs (<ID>) — замок остаётся зелёным, даже если генерация id
// сменит схему; под маской фиксируется ИНВАРИАНТ переходов шагов.
//
// ИНВЕРСИЯ: если переход шага сломан (complete не пробуждает 'отгрузить'), вывод любой
// команды разойдётся с пиннингом, или код выхода ≠ 0 — замок краснеет.

// runProcCmd прогоняет одну подкоманду ladix против витринного процесс.ladix с общей
// БД db и возвращает маскированный stdout (id → <ID>). Требует exit 0 и пустой stderr.
func runProcCmd(t *testing.T, db string, args ...string) string {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := realMain(args, &out, &errBuf)
	if code != 0 {
		t.Fatalf("%v: код = %d, хотим 0; stderr=%q", args, code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("%v: непустой stderr: %q", args, errBuf.String())
	}
	return maskIDs(out.String())
}

func TestProcessLifecycleGolden(t *testing.T) {
	prog := examplePath("процесс.ladix")
	db := filepath.Join(t.TempDir(), "процесс.db")

	// Шаг 1: run --db — старт инстанса. Движок печатает строку задачи первого шага,
	// затем top-level печать(id), затем сводка открытых задач (§EN-7).
	got := runProcCmd(t, db, "run", prog, "--db", db)
	want := "" +
		"[задача] <ID> → комплектовщик, шаг 'собрать_заказ'\n" +
		"запущена выдача заказа, id: <ID>\n" +
		"открытых задач: 1\n" +
		"<ID>  <ID>  'собрать_заказ'  комплектовщик\n"
	if got != want {
		t.Errorf("run --db (маска <ID>) не совпал:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}

	// Шаг 2: tasks --db — список открытых задач (только первый шаг открыт).
	got = runProcCmd(t, db, "tasks", "--db", db)
	want = "<ID>  <ID>  'собрать_заказ'  комплектовщик\n"
	if got != want {
		t.Errorf("tasks --db #1 (маска <ID>) не совпал:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}

	// Шаг 3: complete первого шага → ПРОБУЖДЕНИЕ второго. Печатает «задача … завершена»,
	// строку новой задачи 'отгрузить' и статус инстанса «ожидает, шаг 'отгрузить'».
	got = runProcCmd(t, db, "complete", prog, "t-000001", "--db", db)
	want = "" +
		"задача <ID> завершена\n" +
		"[задача] <ID> → логист, шаг 'отгрузить'\n" +
		"инстанс <ID>: ожидает, шаг 'отгрузить'\n"
	if got != want {
		t.Errorf("complete #1 (маска <ID>) не совпал:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}

	// Шаг 4: повторный tasks --db — открыт уже ВТОРОЙ шаг (пробуждение сработало).
	got = runProcCmd(t, db, "tasks", "--db", db)
	want = "<ID>  <ID>  'отгрузить'  логист\n"
	if got != want {
		t.Errorf("tasks --db #2 (маска <ID>) не совпал:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}

	// Шаг 5: complete последнего шага → процесс ВЫПОЛНЕН (следующего шага нет).
	got = runProcCmd(t, db, "complete", prog, "t-000002", "--db", db)
	want = "" +
		"задача <ID> завершена\n" +
		"инстанс <ID>: выполнен\n"
	if got != want {
		t.Errorf("complete #2 (маска <ID>) не совпал:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}

	// Шаг 6: tasks --db — открытых задач больше нет (строка 11 §EN-7).
	got = runProcCmd(t, db, "tasks", "--db", db)
	want = "открытых задач нет\n"
	if got != want {
		t.Errorf("tasks --db #3 (маска <ID>) не совпал:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
}
