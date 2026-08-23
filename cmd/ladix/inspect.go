package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// deadlineLayout — формат печати дедлайна задачи в inspect (паритет
// engine.deadlineLayout «2006-01-02 15:04», §AU-10.D). Дублируется здесь, чтобы inspect
// не тащил движок (INV-1: без engine/eval).
const inspectDeadlineLayout = "2006-01-02 15:04"

// inspectMain разбирает аргументы подкоманды inspect и печатает снимок инстанса +
// лёгкую историю задач (018 B6, §AU-8). rest — аргументы ПОСЛЕ «inspect»: один
// позиционный <id> + опциональный --db (дефолт ladix.db). Файл НЕ принимается — всё из
// БД; движок/интерпретатор НЕ строятся (INV-1, FR-005). read-only (Store не пишется).
// Обёрнута guard/recover-барьером (конституция III).
func inspectMain(rest []string, stdout, stderr io.Writer) int {
	dbPath := defaultDBPath
	var positional []string
	for k := 0; k < len(rest); k++ {
		a := rest[k]
		switch {
		case a == "--db":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --db требует значение")
				return 2
			}
			dbPath = rest[k+1]
			k++
		case strings.HasPrefix(a, "--db="):
			dbPath = strings.TrimPrefix(a, "--db=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "ladix: неизвестный флаг %s\n", a)
			return 2
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	id := positional[0]

	// Store: дефолт inspect = SQLite ladix.db (D-AU-10); openStore (B5, §AU-9). Открытие
	// вне guard (ошибка открытия — окружение, exit 2).
	st, closeFn, oerr := openStore(dbPath)
	if oerr != nil {
		fmt.Fprintf(stderr, "ladix: не удалось открыть хранилище '%s': %s\n", dbPath, oerr.Error())
		return 2
	}
	defer closeFn()

	return guard(stderr, func() int {
		inst, err := st.LoadInstance(id)
		if err != nil {
			if errors.Is(err, store.ErrInstanceNotFound) {
				// §AU-10.C: английский сентинел НЕ печатается — русский текст дословно.
				fmt.Fprintf(stderr, "ladix: инстанс '%s' не найден\n", id)
				return 2
			}
			fmt.Fprintf(stderr, "ladix: сбой хранилища: %s\n", err.Error())
			return 2
		}
		tasks, terr := st.ListTasksByInstance(id)
		if terr != nil {
			fmt.Fprintf(stderr, "ladix: сбой хранилища: %s\n", terr.Error())
			return 2
		}
		printInspect(stdout, inst, tasks)
		return 0
	})
}

// printInspect печатает снимок инстанса + историю задач в канон §AU-10.D (exact-match).
// Свой формат строки задачи (НЕ engine.FormatTaskLine: иной разделитель «, открыта»/
// «, эскалирована», нет «ПРОСРОЧЕНА»). Переменные — в порядке итерации inst.Variables;
// задачи — в порядке ListTasksByInstance (ID ASC). Пустые блоки → только заголовок.
func printInspect(stdout io.Writer, inst *store.ProcessInstance, tasks []*store.Task) {
	fmt.Fprintf(stdout, "инстанс %s: процесс %s, статус %s, шаг '%s'\n",
		inst.ID, inst.ProcessName, inst.Status, inst.CurrentStep)

	fmt.Fprintln(stdout, "переменные:")
	for name, v := range inst.Variables {
		fmt.Fprintf(stdout, "  %s = %s\n", name, value.String(v))
	}

	fmt.Fprintln(stdout, "задачи:")
	for _, t := range tasks {
		fmt.Fprintln(stdout, inspectTaskLine(t))
	}
}

// inspectTaskLine строит строку одной задачи (§AU-10.D, отступ 2 пробела):
//
//	<t-id> шаг '<StepName>' → <Assignee>[, срок до <время>], <открыта|завершена>[, эскалирована]
//
// Сегменты слева направо: база (всегда) → «, срок до <время>» (только Deadline!=nil) →
// «, <статус>» (всегда) → «, эскалирована» (только Escalated).
func inspectTaskLine(t *store.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %s шаг '%s' → %s", t.ID, t.StepName, t.Assignee)
	if t.Deadline != nil {
		b.WriteString(", срок до " + t.Deadline.Format(inspectDeadlineLayout))
	}
	b.WriteString(", " + string(t.Status))
	if t.Escalated {
		b.WriteString(", эскалирована")
	}
	return b.String()
}
