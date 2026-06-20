package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// payloadProgSrc — процесс: человеческий шаг 'подача', затем АВТО-шаг 'решение',
// читающий данные.итог и сохраняющий его в переменную процесса 'факт'. После
// complete --data авто-шаг исполняется как первый шаг догона и видит payload.
const payloadProgSrc = `процесс заявка(x):
    шаг подача:
        исполнитель: "клиент"
    шаг решение после подача:
        присвоить факт = данные.итог
        печать("решение:", данные.итог)

пусть id = запустить процесс заявка(1)
печать("запущена заявка")
`

// writePayloadProg кладёт payloadProgSrc во временный файл и стартует инстанс
// (run --db), возвращая путь к программе и БД (инстанс ожидает на задаче t-000001).
func writePayloadProg(t *testing.T) (prog, db string) {
	t.Helper()
	dir := t.TempDir()
	prog = filepath.Join(dir, "заявка.ladix")
	if err := os.WriteFile(prog, []byte(payloadProgSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	db = filepath.Join(dir, "заявка.db")
	var out, errBuf bytes.Buffer
	if code := realMain([]string{"run", prog, "--db", db}, &out, &errBuf); code != 0 {
		t.Fatalf("run: код=%d stderr=%q", code, errBuf.String())
	}
	return prog, db
}

// TestCompleteValidPayloadCLI — T018: complete --data '{"итог":"ок"}' (формы V и =V)
// → exit 0, авто-шаг получил payload (наблюдаемо через уведомить аудит: решение: ок).
func TestCompleteValidPayloadCLI(t *testing.T) {
	for _, form := range []string{"V", "=V"} {
		t.Run(form, func(t *testing.T) {
			prog, db := writePayloadProg(t)
			var out, errBuf bytes.Buffer
			var args []string
			if form == "V" {
				args = []string{"complete", prog, "t-000001", "--data", `{"итог":"ок"}`, "--db", db}
			} else {
				args = []string{"complete", prog, "t-000001", `--data={"итог":"ок"}`, "--db", db}
			}
			code := realMain(args, &out, &errBuf)
			if code != 0 {
				t.Fatalf("complete --data (%s): код=%d stderr=%q", form, code, errBuf.String())
			}
			if errBuf.Len() != 0 {
				t.Fatalf("непустой stderr: %q", errBuf.String())
			}
			// Авто-шаг увидел payload: уведомить аудит печатает «решение: ок».
			if !strings.Contains(out.String(), "решение: ок") {
				t.Fatalf("stdout не содержит «решение: ок» (payload не дошёл до шага):\n%s", out.String())
			}
		})
	}
}

// TestCompleteBadJSON — Замок d (T016): complete --data '{не json' → stderr ровно
// `ladix: неверный JSON в --data: <деталь>`, exit 2, Store НЕ изменён (инстанс ещё
// ожидает на t-000001). Также --data '[1,2]' (массив, не-объект) → та же ошибка.
func TestCompleteBadJSON(t *testing.T) {
	cases := []struct{ name, payload string }{
		{"невалидный", `{не json`},
		{"массив-не-объект", `[1,2]`},
		{"скаляр-не-объект", `42`},
	}
	const prefix = "ladix: неверный JSON в --data: "
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, db := writePayloadProg(t)
			var out, errBuf bytes.Buffer
			code := realMain([]string{"complete", prog, "t-000001", "--data", c.payload, "--db", db}, &out, &errBuf)
			if code != 2 {
				t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
			}
			got := errBuf.String()
			if !strings.HasPrefix(got, prefix) {
				t.Fatalf("stderr=%q, хотим префикс %q (дословно §AU-10.C)", got, prefix)
			}
			if !strings.HasSuffix(got, "\n") {
				t.Fatalf("stderr без перевода строки: %q", got)
			}
			// Store не тронут: задача всё ещё открыта (валидация ДО мутаций).
			var o2, e2 bytes.Buffer
			if code := realMain([]string{"tasks", "--db", db}, &o2, &e2); code != 0 {
				t.Fatalf("tasks: код=%d stderr=%q", code, e2.String())
			}
			if !strings.Contains(o2.String(), "t-000001") {
				t.Fatalf("задача t-000001 исчезла — Store мутирован при плохом JSON:\n%s", o2.String())
			}
		})
	}
}

// TestCompleteFlagNeedsValue — T017: complete --data (без значения, флаг последний)
// → stderr `ladix: флаг --data требует значение`, exit 2 (зеркало --webhook).
func TestCompleteFlagNeedsValue(t *testing.T) {
	prog, db := writePayloadProg(t)
	var out, errBuf bytes.Buffer
	// --data последним аргументом (значения нет).
	code := realMain([]string{"complete", prog, "t-000001", "--db", db, "--data"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код=%d, хотим 2; stderr=%q", code, errBuf.String())
	}
	if got := strings.TrimRight(errBuf.String(), "\n"); got != "ladix: флаг --data требует значение" {
		t.Fatalf("stderr=%q, хотим «ladix: флаг --data требует значение»", got)
	}
}

// TestCompleteNoFlagCLI — Замок c (T013, CLI-уровень): complete БЕЗ --data → exit 0,
// данные пусто (данные.итог → Пусто); шаг исполняется штатно, не ошибка.
func TestCompleteNoFlagCLI(t *testing.T) {
	prog, db := writePayloadProg(t)
	var out, errBuf bytes.Buffer
	code := realMain([]string{"complete", prog, "t-000001", "--db", db}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("complete без --data: код=%d stderr=%q", code, errBuf.String())
	}
	// Пустой payload → данные.итог == Пусто → «решение: пусто» (репр §7), не ошибка.
	if !strings.Contains(out.String(), "решение: пусто") {
		t.Fatalf("stdout не содержит «решение: пусто» (без --data payload должен быть пустым):\n%s", out.String())
	}
	if strings.Contains(out.String(), "решение: ок") {
		t.Fatalf("без --data payload не должен возникнуть, но «решение: ок» найдено:\n%s", out.String())
	}
}
