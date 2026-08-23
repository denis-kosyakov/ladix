package engine_test

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
)

// outbox_reset_test.go — замок per-step сброса effectIndex (M3-C2b, §C-2b.4).
// Драйвит РЕАЛЬНЫЙ advance процесса с ДВУМЯ авто-шагами, каждый с одним эффектом в
// теле. Кадр advance пушится один раз на весь прогон; без сброса effectIndex в начале
// каждой итерации шага второй шаг получил бы idx 1 вместо 0 → ключ |шаг2|1, и на
// рестарте (реплей с более раннего шага) индексы бы разошлись.
//
// Замок: ключ outbox второго шага = "<inst>|оповестить|0" (idx 0, сброс сработал).
// Мутпроба: убрать frame.effectIndex=0 из advance → ключ станет |оповестить|1 →
// LoadOutbox("...|оповестить|0") вернёт ErrOutboxNotFound → тест краснит.

const twoStepEffectsSrc = `процесс рассылка(адресат):
    шаг приветствие:
        уведомить почта("привет, " + адресат)
    шаг оповестить после приветствие:
        уведомить смс("уведомление для " + адресат)

пусть id = запустить процесс рассылка("Иванов")
печать("id:", id)
`

func TestStepEffectIndexResetsPerStep(t *testing.T) {
	interp, st, _, _ := buildStack(t, twoStepEffectsSrc, goldenMoment())

	tokens, errList := lexer.New(twoStepEffectsSrc).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if err := interp.Run(prog); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Оба эффекта — единственные в своём теле шага → idx 0 в каждом (сброс per-step).
	if _, err := st.LoadOutbox("p-000001|приветствие|0"); err != nil {
		t.Errorf("ключ шага приветствие idx0 не найден: %v", err)
	}
	if _, err := st.LoadOutbox("p-000001|оповестить|0"); err != nil {
		t.Errorf("ключ шага оповестить idx0 не найден (effectIndex НЕ сброшен per-step?): %v", err)
	}
}
