package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// control_plan_golden_test.go — T-GOLD-METRIC (фича 023-v2-finalization, W2/§F-1).
// Детерминированный golden золотого сквозного примера examples/контроль_плана.ladix
// под FixedClock{2026,6,15}, прогон из КОРНЯ репо (withRepoRoot — иначе data/orders.csv
// не резолвится). Пинит 3 фасета §2-цепочки ДВУМЯ путями:
//   (i)  скаляр METRIC-путём: выручка_30д = 300000.0 (окно «последние 30дн»);
//   (ii) explain-строка §C-5.3 RUN-путём (метрика-триггер сработал, fire-if-true);
//   (iii) метрика-driven старт процесса p-000001/t-000001 RUN-путём.
// Заменяет дата-наивный TestCLIGoldenDeadlineEscalation (удалён, W2/T006): тот
// ломался от добавления источника (нерезолвимый путь + дата-зависимый снимок).

// fixedClock20260615Run — те же сутки 2026-06-15, что и метрический FixedClock, но как
// engine.Clock для RUN-пути (планировщик дедлайнов). Полдень — детерминированный момент.
var fixedClock20260615Run = fixedClock{time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local)}

// TestCLIControlPlanScalarFixedClock — фасет (i): METRIC-путь. Окно (2026-05-16,
// 2026-06-15] над data/orders.csv оставляет лишь оплаченный заказ 2026-05-27 (300000) →
// DoD-скаляр 300000.0 (та же половина данных, что и выручка_30д.ladix).
//
// 🔁 ИНВЕРСИЯ: снимок (300000.0) / граница окна / Clock сменились → красный.
func TestCLIControlPlanScalarFixedClock(t *testing.T) {
	withRepoRoot(t, func() {
		example := filepath.Join("examples", "контроль_плана.ladix")
		var out, errBuf bytes.Buffer
		code := runMetric(example, "выручка_30д", eval.DefaultMaxDepth,
			eval.FixedClock{D: value.Дата{Year: 2026, Month: 6, Day: 15}}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		if errBuf.Len() != 0 {
			t.Errorf("непустой stderr: %q", errBuf.String())
		}
		if out.String() != "300000.0\n" {
			t.Errorf("DoD-скаляр байт-не-точен (FixedClock 2026-06-15): получено %q, хотим %q",
				out.String(), "300000.0\n")
		}
	})
}

// TestCLIControlPlanRunFixedClock — фасеты (ii)+(iii): RUN-путь под FixedClock{2026,6,15}.
// Метрика-триггер «выручка_30д < 3_000_000» СРАБАТЫВАЕТ (300000.0 < 3000000) → fire-if-true:
//
//	(ii)  explain-строка §C-5.3 (одностройчная run-форма, БЕЗ ребра, числа без подчёркиваний);
//	(iii) запуск процесса эскалация_плана(значение) → инстанс p-000001, задача t-000001.
//
// Дедлайн маскируется (maskDeadlines → «срок до <DT>») → дата-независимость по моменту.
//
// 🔁 ИНВЕРСИЯ: снимок/порог/оператор в explain разошлись / триггер не сработал /
// старт не дал p-000001 → байт-расхождение → красный.
func TestCLIControlPlanRunFixedClock(t *testing.T) {
	withRepoRoot(t, func() {
		example := filepath.Join("examples", "контроль_плана.ladix")
		var out, errBuf bytes.Buffer
		code := runFile(example, "", eval.DefaultMaxDepth, nil, fixedClock20260615Run, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		if errBuf.Len() != 0 {
			t.Errorf("непустой stderr: %q", errBuf.String())
		}
		want := "" +
			"триггер 'выручка_30д < 3000000' сработал: выручка_30д = 300000.0 (снимок) < 3000000 (порог) → истина\n" +
			"[задача] t-000001 → менеджер, шаг 'связаться_с_клиентом', срок до <DT>\n" +
			"задача триггер 'эскалация_плана.связаться_с_клиентом' требует serve (фича 007b)\n" +
			"открытых задач: 1\n" +
			"t-000001  p-000001  'связаться_с_клиентом'  менеджер  срок до <DT>\n"
		if got := maskDeadlines(out.String()); got != want {
			t.Errorf("stdout (с маской <DT>) байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
		}
		// Фасет (ii) явный анти-over-маск: explain-строка присутствует целиком.
		if !strings.Contains(out.String(), "триггер 'выручка_30д < 3000000' сработал: выручка_30д = 300000.0 (снимок) < 3000000 (порог) → истина") {
			t.Errorf("explain-строка §C-5.3 отсутствует/искажена: %q", out.String())
		}
	})
}
