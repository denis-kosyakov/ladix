package engine_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/eval"
)

// TestProcessRuntimeStays8 — Инвариант 1 (T006, FR-011): протяжка payload идёт
// ВНУТРЕННИМ API *Engine (Complete/catchUp/advanceAfterComplete/advance), НЕ через шов
// ProcessRuntime. Интерфейс остаётся РОВНО 8 методов.
// Мутпроба: добавить метод в ProcessRuntime (рост шва) → красный.
func TestProcessRuntimeStays8(t *testing.T) {
	rt := reflect.TypeOf((*eval.ProcessRuntime)(nil)).Elem()
	if got := rt.NumMethod(); got != 8 {
		t.Fatalf("ProcessRuntime имеет %d методов, хотим 8 (шов не растёт; payload — внутренний API движка)", got)
	}
}

// TestEvalImportGraphClean — Инвариант 1/4 (T006/T021, FR-011/FR-013): internal/eval НЕ
// импортирует engine/store/jsonval. «данные» входит в eval уже как value.Запись (через
// stepEnv.Define движком), декод jsonval — на CLI. Мутпроба: импорт любого из трёх в
// eval (напр., дубль-декода payload в eval) → красный.
func TestEvalImportGraphClean(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/denis-kosyakov/ladix/internal/eval").Output()
	if err != nil {
		t.Fatalf("go list -deps eval: %v", err)
	}
	forbidden := []string{
		"github.com/denis-kosyakov/ladix/internal/engine",
		"github.com/denis-kosyakov/ladix/internal/store",
		"github.com/denis-kosyakov/ladix/internal/jsonval",
	}
	deps := out
	for _, f := range forbidden {
		if strings.Contains(string(deps), f+"\n") {
			t.Fatalf("internal/eval импортирует запрещённый пакет %q (инвариант хартии §5: eval тянет только ast+value)", f)
		}
	}
}

// TestCompletePayloadDecoderSingle — Инвариант 4 (T021, FR-013): payload декодируется
// РОВНО jsonval.PayloadToRecord; cmd/ladix импортирует jsonval (потребитель). Проба
// зависимости-потребления: cmd/ladix в своём дереве зависимостей имеет jsonval, а eval
// — нет (декод не дублируется внутри eval). Дубль json.Decoder/Unmarshal для payload
// вне jsonval ловится этим + предыдущим тестом (eval чист) + ревью диффа.
func TestCompletePayloadConsumesJsonval(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/denis-kosyakov/ladix/cmd/ladix").Output()
	if err != nil {
		t.Fatalf("go list -deps cmd/ladix: %v", err)
	}
	if !strings.Contains(string(out), "github.com/denis-kosyakov/ladix/internal/jsonval\n") {
		t.Fatalf("cmd/ladix НЕ зависит от internal/jsonval — payload-декод не потребляет канонический кодек")
	}
}
