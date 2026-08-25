package metrics_test

// Исполняемый замок README-сниппета «Вычисление метрики над своими данными».
//
// Example_readme — дословное зеркало кода сниппета (без main-обвязки):
// тот же исходник, те же записи, те же Options. Если рефактор Options/Result
// или семантики Evaluate изменит вывод — Example покраснеет по Output.
// TestReadmeSnippetAligned дополнительно грепает README.md на ключевые
// строки зеркала: молчаливый дрейф README относительно этого теста краснеет.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix"
	"github.com/denis-kosyakov/ladix/metrics"
)

const readmeSource = `источник продажи:
    файл: "продажи.json"
    поля:
        дата_заказа: Дата
метрика выручка_за_месяц:
    источник: продажи
    период: ежемесячно
    по_дате: дата_заказа
    агрегат: сумма(сумма_заказа)
`

func Example_readme() {
	prog, diags, err := ladix.Compile(readmeSource)
	if err != nil || prog == nil {
		panic(fmt.Sprint(err, diags))
	}

	records := []map[string]any{
		{"сумма_заказа": 1500, "дата_заказа": "2026-05-31"},
		{"сумма_заказа": 2300, "дата_заказа": "2026-05-20"},
	}
	opts := metrics.Options{
		Today:  metrics.Date{Year: 2026, Month: 5, Day: 31},
		Fields: map[string]string{"дата_заказа": "Дата"},
	}

	res, mdiags, err := metrics.Evaluate(prog.Metrics[0], records, opts)
	if err != nil {
		panic(fmt.Sprint(err, mdiags))
	}
	fmt.Println(res.Text)
	// Output: 3800
}

// TestReadmeSnippetAligned — README.md обязан содержать дословные ключевые
// строки зеркала выше; иначе Example проверяет уже не то, что читает потребитель.
func TestReadmeSnippetAligned(t *testing.T) {
	raw, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("README.md: %v", err)
	}
	readme := string(raw)
	for _, fragment := range []string{
		"агрегат: сумма(сумма_заказа)",
		`{"сумма_заказа": 1500, "дата_заказа": "2026-05-31"}`,
		"Today:  metrics.Date{Year: 2026, Month: 5, Day: 31}",
		"res, mdiags, err := metrics.Evaluate(prog.Metrics[0], records, opts)",
		"fmt.Println(res.Text)",
	} {
		if !strings.Contains(readme, fragment) {
			t.Errorf("README.md разошёлся со сниппетом-замком: не найдено %q", fragment)
		}
	}
}
