package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Контракт: specs/009-v1-finalization/contracts/docs-alignment.md (A1–A4).
// Замок US5 (T036): доки выровнены с реальностью, а поведение кода НЕ изменилось.
// ИНВЕРСИЯ: возврат любого снятого утверждения (Go 1.22 в командах сборки README,
// сводка «Найдено K ошибок», «deferred до 006» в онбординге, достижимость тип(x))
// → красный.

// readRepoDoc читает файл доки из корня репозитория (как repoRoot() в metric_test.go).
func readRepoDoc(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join(repoRoot(), rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("не прочитать %s: %v", rel, err)
	}
	return string(b)
}

// A1: версия Go выровнена — в командах сборки README больше нет «1.22»;
// фактический минимум go.mod (go 1.25) указан как порог.
func TestDocsAlignmentA1GoVersion(t *testing.T) {
	readme := readRepoDoc(t, "README.md")

	// Команды сборки README не должны обещать Go 1.22 (порог занижен относительно go.mod).
	if strings.Contains(readme, "1.22") {
		t.Errorf("A1: README всё ещё упоминает «1.22» — версия Go рассинхронизирована с src/go.mod (go 1.25)")
	}
	// Фактический минимум присутствует.
	if !strings.Contains(readme, "1.25") {
		t.Errorf("A1: README не называет фактический минимум Go «1.25» из src/go.mod")
	}

	// Пользовательский quickstart тоже не должен занижать порог до 1.22.
	quickstart := readRepoDoc(t, "docs/quickstart.md")
	if strings.Contains(quickstart, "1.22") {
		t.Errorf("A1: docs/quickstart.md всё ещё упоминает «1.22» — версия Go рассинхронизирована с src/go.mod (go 1.25)")
	}

	// Источник истины — go.mod: порог не понижается ниже зафиксированного там.
	gomod := readRepoDoc(t, "src/go.mod")
	if !strings.Contains(gomod, "go 1.25") {
		t.Errorf("A1: src/go.mod не содержит «go 1.25» — обнови привязку версии в README/тесте")
	}
}

// A2: обещание итоговой строки-сводки снято — ни README, ни SPEC §13 не утверждают,
// что агрегатор печатает «Найдено K ошибок». (Сам код её не печатает: см.
// internal/errors/aggregate_test.go.)
func TestDocsAlignmentA2NoFoundSummary(t *testing.T) {
	for _, f := range []string{"README.md", "SPEC.md"} {
		doc := readRepoDoc(t, f)
		// Обещанная сводка вида «Найдено K ошибок» (с любым символом между «Найдено» и «ошибок»).
		if strings.Contains(doc, "Найдено K ошибок") {
			t.Errorf("A2: %s всё ещё обещает сводку «Найдено K ошибок» — код её не печатает", f)
		}
	}
}

// A3: стейл-коммент онбординга переписан — нет «deferred до 006» (006 смержена,
// процесс исполняется движком).
func TestDocsAlignmentA3OnboardingComment(t *testing.T) {
	onb := readRepoDoc(t, "examples/онбординг.ladix")
	if strings.Contains(onb, "deferred до 006") {
		t.Errorf("A3: examples/онбординг.ladix всё ещё содержит стейл-коммент «deferred до 006»")
	}
}

// A4 (010-A1, §SC-10 #7, ИНВЕРСИЯ 009-замка): тип(x) ПО-ПРЕЖНЕМУ недостижим из
// синтаксиса — но механизм изменён. С 010-A1 «тип» более НЕ зарезервированное
// слово, а ключевое слово-атрибут источника (KW_TYPE, §SC-D-RESERVE). В выражении
// KW_TYPE не начинает primary → печать(тип(5)) даёт ПАРС-ошибку «неожиданный токен
// 'тип'» (код 1), а НЕ «Целое». Ядро инварианта — (a) код 1 + (c) stdout без
// «Целое» (builtin `тип` dormant) — сохранено; assert (b) переведён с reserved-word
// на парс-диагностику (эмпирически: «неожиданный токен 'тип'»). Замок ОБЯЗАН
// кусаться: если тип(x) станет достижим (KW_TYPE попадёт в parsePrimary) →
// stdout=«Целое» → красный.
func TestDocsAlignmentA4TipReserved(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tip.ladix")
	if err := os.WriteFile(file, []byte("печать(тип(5))\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", file}, &out, &errBuf)

	if code != 1 {
		t.Fatalf("A4: печать(тип(5)) дал код %d, хотим 1 (парс-ошибка KW_TYPE); stdout=%q stderr=%q",
			code, out.String(), errBuf.String())
	}
	// (b) Должна быть именно парс-диагностика про «тип» (KW_TYPE не начинает выражение),
	// а не reserved-word и не успешный вывод «Целое».
	if !strings.Contains(errBuf.String(), "неожиданный токен 'тип'") {
		t.Errorf("A4: stderr не содержит «неожиданный токен 'тип'»: %q", errBuf.String())
	}
	if strings.Contains(out.String(), "Целое") {
		t.Errorf("A4: тип(x) стал достижим — stdout вернул «Целое»: %q (функция активирована вопреки v1-резерву)", out.String())
	}
}

// A5: serve/emit РЕАЛИЗОВАНЫ и смержены (main.go диспатчит serve→serveMain,
// emit→emitMain; internal/daemon + serve_golden_test/emit_golden_test). README
// больше не должен помечать их «ещё не реализован(а)»/«целевой контракт». Эта фраза
// фигурировала РОВНО в двух снятых блок-цитатах серверных команд — её возврат был бы
// ложным заявлением, что serve/emit будущие.
// ИНВЕРСИЯ: вернётся ложное «ещё не реализована» (или «контракт ниже — целевой») —
// красный.
func TestDocsAlignmentA5ServeEmitImplemented(t *testing.T) {
	readme := readRepoDoc(t, "README.md")

	// Снятая подстрока серверных блок-цитат: «ещё не реализован» помечала serve/emit
	// как будущие. Код их реализует — фраза не должна возвращаться.
	if strings.Contains(readme, "ещё не реализован") {
		t.Errorf("A5: README снова заявляет «ещё не реализован(а)» — serve/emit РЕАЛИЗОВАНЫ " +
			"(main.go → serveMain/emitMain, internal/daemon, *_golden_test.go)")
	}
	// «Контракт ниже — целевой» — второй маркер будущности из тех же блок-цитат.
	if strings.Contains(readme, "Контракт ниже — целевой") {
		t.Errorf("A5: README снова помечает контракт serve/emit «целевым» (будущим) — команды доступны в v1")
	}
	// serve/emit должны быть описаны как присутствующие команды (защита от удаления секций).
	for _, cmd := range []string{"ladix serve", "ladix emit"} {
		if !strings.Contains(readme, cmd) {
			t.Errorf("A5: README не упоминает доступную команду %q", cmd)
		}
	}
}
