package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// emitMain разбирает аргументы подкоманды emit и кладёт событие в очередь (007b, FR-015,
// emit-command.md). rest — аргументы ПОСЛЕ «emit»: один обязательный позиционный
// (<событие>), один опциональный позиционный ([json] payload) и --db (дефолт ladix.db).
// emit НЕ запускает демон: пишет строку в events и выходит (доставка — фаза drainEvents
// демона serve). Очередь видна serve только через ОБЩИЙ --db (без --db — эфемерно).
// Нет имени события / лишний позиционный / неизвестный флаг → stderr usage, exit 2.
func emitMain(rest []string, stdout, stderr io.Writer) int {
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
	if len(positional) < 1 || len(positional) > 2 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	name := positional[0]
	payload := ""
	if len(positional) == 2 {
		payload = positional[1]
	}
	return emitEvent(name, payload, dbPath, engine.SystemClock{}, stdout, stderr)
}

// enqueueEvent — общий хелпер минта события в durable-очередь (D-IE-8, §IE-4.2).
// Единый путь минта для emit (CLI) и HTTP-приёмника (serve --listen): NextEventID →
// Event{CreatedAt: clock.Now()} → EnqueueEvent → (id, nil). Ack-печать НЕ входит —
// вызыватель печатает сам (emit «поставлено в очередь» / HTTP «принято», тексты различны
// НАМЕРЕННО). Принимает store.Store (интерфейс): работает над SQLite (emit) и над Store
// демона (HTTP-хендлер). CreatedAt — инъектируемые часы (детерминизм тестов, FR-IE-11).
func enqueueEvent(st store.Store, name, payload string, clock engine.Clock) (string, error) {
	id, err := st.NextEventID()
	if err != nil {
		return "", err
	}
	e := &store.Event{
		ID:          id,
		Name:        name,
		PayloadJSON: payload,
		CreatedAt:   clock.Now(),
		Processed:   false,
	}
	if err := st.EnqueueEvent(e); err != nil {
		return "", err
	}
	return id, nil
}

// emitEvent открывает Store (SQLite под dbPath), минтит ID и пишет одно событие в
// очередь (emit-command.md §жизненный цикл). Открытие Store вне guard (сбой —
// окружение, exit 2, зеркало §EN-8.B). Clock инжектируется (прод — SystemClock; тест —
// фиксированные часы для детерминированного CreatedAt). Пустой payload допустим (сырой
// текст пишется как есть; валидность JSON демон проверяет при маппинге). Подтверждение
// печатается по-русски. Обёрнута guard/recover-барьером (Принцип III).
func emitEvent(name, payload, dbPath string, clock engine.Clock, stdout, stderr io.Writer) int {
	sq, oerr := store.NewSQLiteStore(dbPath)
	if oerr != nil {
		fmt.Fprintf(stderr, "ladix: не удалось открыть хранилище '%s': %s\n", dbPath, oerr.Error())
		return 2
	}
	defer sq.Close()
	return guard(stderr, func() int {
		id, err := enqueueEvent(sq, name, payload, clock)
		if err != nil {
			fmt.Fprintf(stderr, "ladix: сбой хранилища: %s\n", err.Error())
			return 2
		}
		fmt.Fprintf(stdout, "событие %s '%s' поставлено в очередь\n", id, name)
		return 0
	})
}
