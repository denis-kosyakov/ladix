package metrics

// Финальное ревью 030 — закрытие пробелов, найденных приёмкой:
//
//  1. spec.md Requirement «Форма данных потребителя», Scenario «Целое и Дробное
//     различаются по Go-типу» не был закрыт НИ ОДНИМ тестом: ни одна запись в
//     metrics/*_test.go не несла Go-значение типа float64 (golden-фикстуры идут
//     через json.Number, evaluate_test/parity_test — через int64/string).
//     Мутпроба: замена ветки `case float64: value.Дробное` на `value.Целое`
//     оставляла ВЕСЬ пакет зелёным. Ниже — замок на таблицу Д-8 целиком.
//  2. spec.md «Форма данных потребителя»: Result.Value в JSON-семантике
//     (Список → []any, Запись → map[string]any, Дата/Длительность/Период →
//     строкой) — ветки toResult не исполнялись ни одним тестом.
//  3. Циклическое / бездонно вложенное значение записи роняло процесс
//     ПОТРЕБИТЕЛЯ через `fatal error: stack overflow` (не ловится recover-
//     барьером Evaluate) — прямое нарушение Scenario «Паника не пересекает
//     границу API». Гард — recordValueDepthLimit (records.go).
//  4. Д-7 «Неизвестное имя типа в Fields → ErrInvalidOptions»: обе ветки
//     validatedFieldNames не исполнялись.

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
	"github.com/denis-kosyakov/ladix/ir"
)

// TestConversionGoTypeToLadixType — таблица Д-8 (spec.md «Форма данных
// потребителя») ПОЛНОСТЬЮ: какой Go-тип в какой тип Ladix отображается.
// Удаление/подмена любой ветки convertJSONValue краснит соответствующий кейс.
func TestConversionGoTypeToLadixType(t *testing.T) {
	cases := []struct {
		name     string
		in       any
		wantType string
		wantText string
	}{
		{"nil", nil, "Пусто", "пусто"},
		{"bool", true, "Булево", "истина"},
		{"string", "привет", "Строка", "привет"},
		{"int", int(7), "Целое", "7"},
		{"int8", int8(7), "Целое", "7"},
		{"int16", int16(7), "Целое", "7"},
		{"int32", int32(7), "Целое", "7"},
		{"int64", int64(154000), "Целое", "154000"},
		{"uint", uint(7), "Целое", "7"},
		{"uint8", uint8(7), "Целое", "7"},
		{"uint16", uint16(7), "Целое", "7"},
		{"uint32", uint32(7), "Целое", "7"},
		{"uint64", uint64(7), "Целое", "7"},
		{"float32", float32(1.5), "Дробное", "1.5"},
		{"float64", float64(154000), "Дробное", "154000.0"},
		// json.Number — СТРОГО по форме токена (§9.3 дословно), а не по значению.
		{"json.Number целая форма", json.Number("154000"), "Целое", "154000"},
		{"json.Number дробная форма", json.Number("154000.0"), "Дробное", "154000.0"},
		{"json.Number экспонента", json.Number("1e3"), "Дробное", "1000.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, kind := convertJSONValue(tc.in, 1)
			if kind != convOK {
				t.Fatalf("convertJSONValue вернул отказ %v для %T", kind, tc.in)
			}
			if got := v.TypeName(); got != tc.wantType {
				t.Errorf("TypeName = %q, хотим %q (Д-8: типизация по Go-типу)", got, tc.wantType)
			}
			if got := value.String(v); got != tc.wantText {
				t.Errorf("печать = %q, хотим %q", got, tc.wantText)
			}
		})
	}
}

// TestЦелоеИДробноеРазличаютсяПоGoТипу — spec.md Scenario «Целое и Дробное
// различаются по Go-типу» ДОСЛОВНО (int64(154000) и 154000.0), сквозь публичный
// Evaluate: агрегат макс(поле) отдаёт значение поля как есть, поэтому Result.Type
// показывает результат типизации входа.
func TestЦелоеИДробноеРазличаютсяПоGoТипу(t *testing.T) {
	cases := []struct {
		name     string
		v        any
		wantType string
		wantText string
		wantVal  any
	}{
		{"int64(154000) → Целое", int64(154000), "Целое", "154000", int64(154000)},
		{"154000.0 → Дробное", 154000.0, "Дробное", "154000.0", float64(154000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recs := []map[string]any{{"ид": int64(1), "сумма_заказа": tc.v}}
			got, diags, err := Evaluate(baseMetric("", "макс(сумма_заказа)", "", ""), recs,
				Options{Today: may31()})
			if err != nil || len(diags) != 0 {
				t.Fatalf("Evaluate: err=%v diags=%+v", err, diags)
			}
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, хотим %q", got.Type, tc.wantType)
			}
			if got.Text != tc.wantText {
				t.Errorf("Text = %q, хотим %q", got.Text, tc.wantText)
			}
			if !reflect.DeepEqual(got.Value, tc.wantVal) {
				t.Errorf("Value = %#v, хотим %#v", got.Value, tc.wantVal)
			}
		})
	}
}

// TestResultValueJSONСемантика — spec.md «Форма данных потребителя», абзац
// «Результат SHALL нести … Go-значение в JSON-семантике»: Список → []any,
// Запись → map[string]any (рекурсивно), Дата/Длительность/Период — строкой,
// равной Text. Проверяется toResult напрямую: агрегатом эти типы из записей не
// вытаскиваются, а контракт Result нормативен и обязан быть замкнут.
func TestResultValueJSONСемантика(t *testing.T) {
	list := value.NewList([]value.Value{value.Целое{V: 1}, value.Строка{V: "а"}, value.None})
	rec := value.NewRecord([]string{"а", "б"},
		map[string]value.Value{"а": value.Целое{V: 1}, "б": value.Булево{V: true}})

	cases := []struct {
		name     string
		in       value.Value
		wantType string
		wantVal  any
	}{
		{"Целое", value.Целое{V: 5}, "Целое", int64(5)},
		{"Дробное", value.Дробное{V: 2.5}, "Дробное", float64(2.5)},
		{"Строка", value.Строка{V: "х"}, "Строка", "х"},
		{"Булево", value.Булево{V: false}, "Булево", false},
		{"Пусто", value.None, "Пусто", nil},
		{"Список", list, "Список", []any{int64(1), "а", nil}},
		{"Запись", rec, "Запись", map[string]any{"а": int64(1), "б": true}},
		// Дата не имеет представления в JSON → строка, равная Text.
		{"Дата", value.Дата{Year: 2026, Month: 5, Day: 31}, "Дата", "2026-05-31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toResult(tc.in)
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, хотим %q", got.Type, tc.wantType)
			}
			if !reflect.DeepEqual(got.Value, tc.wantVal) {
				t.Errorf("Value = %#v (%T), хотим %#v", got.Value, got.Value, tc.wantVal)
			}
			if tc.wantType == "Дата" && got.Value != got.Text {
				t.Errorf("Value = %#v, для отложенных типов обязано равняться Text %q", got.Value, got.Text)
			}
		})
	}
}

// TestЦиклическоеЗначениеНеРоняетПроцесс — spec.md Scenario «Паника не пересекает
// границу API» в его САМОЙ ОПАСНОЙ форме: `fatal error: stack overflow` в Go НЕ
// перехватывается recover-барьером Evaluate, поэтому бесконечная рекурсия
// convertJSONValue означала бы аварийное завершение процесса ПОТРЕБИТЕЛЯ, а не
// диагностику. Замок краснеет (падением всего бинаря теста) при снятии
// recordValueDepthLimit.
func TestЦиклическоеЗначениеНеРоняетПроцесс(t *testing.T) {
	cyclicMap := map[string]any{"ид": int64(1)}
	cyclicMap["сам"] = cyclicMap

	cyclicList := make([]any, 1)
	cyclicList[0] = cyclicList

	// Бездонное, но АЦИКЛИЧЕСКОЕ дерево: ровно то, что даёт json.Unmarshal на
	// входе вида "[[[[…]]]]" размером в пару килобайт.
	var deep any = int64(1)
	for i := 0; i < recordValueDepthLimit+50; i++ {
		deep = []any{deep}
	}

	cases := []struct {
		name string
		v    any
	}{
		{"циклическая map", cyclicMap},
		{"циклический слайс", cyclicList},
		{"бездонное дерево", deep},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recs := []map[string]any{{"ид": int64(1), "вложено": tc.v}}
			_, diags, err := Evaluate(baseMetric("", "количество(ид)", "", ""), recs,
				Options{Today: may31()})
			checkOneDiag(t, diags, err,
				"источник 'продажи': запись 1: поле 'вложено': неподдерживаемый тип значения",
				ir.Position{Line: 1, Col: 1})
		})
	}
}

// TestГлубинаЗначенияНаПределеПроходит — граница гарда: вложенность ровно
// recordValueDepthLimit допустима (замок на «гард стал строже спеки»).
func TestГлубинаЗначенияНаПределеПроходит(t *testing.T) {
	var deep any = int64(1)
	for i := 0; i < recordValueDepthLimit-1; i++ {
		deep = []any{deep}
	}
	recs := []map[string]any{{"ид": int64(1), "вложено": deep}}
	got, diags, err := Evaluate(baseMetric("", "количество(ид)", "", ""), recs,
		Options{Today: may31()})
	if err != nil || len(diags) != 0 {
		t.Fatalf("вложенность %d отклонена, предел %d: err=%v diags=%+v",
			recordValueDepthLimit, recordValueDepthLimit, err, diags)
	}
	if got.Text != "1" {
		t.Errorf("Text = %q, хотим \"1\"", got.Text)
	}
}

// TestOptionsFieldsНекорректны — Д-7: неизвестное имя типа и недопустимое имя
// поля в Options.Fields → ErrInvalidOptions (а не паника и не молчаливое
// игнорирование). Второй кейс — ещё и замок на инъекцию через имя поля: оно
// попадает в ТЕКСТ синтетической программы (template.go), поэтому «имя» с
// переводом строки обязано быть отвергнуто ДО сборки текста.
func TestOptionsFieldsНекорректны(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]string
	}{
		{"неизвестное имя типа", map[string]string{"дата_заказа": "Дата2"}},
		{"пустое имя типа", map[string]string{"дата_заказа": ""}},
		{"инъекция через имя поля", map[string]string{"x: Целое\n        y": "Дата"}},
		{"имя поля — ключевое слово", map[string]string{"и": "Целое"}},
		{"имя поля с пробелом", map[string]string{"дата заказа": "Дата"}},
		{"пустое имя поля", map[string]string{"": "Дата"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags, err := Evaluate(baseMetric("", "количество(ид)", "", ""),
				salesRecords(), Options{Today: may31(), Fields: tc.fields})
			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("err = %v, хотим ErrInvalidOptions", err)
			}
			if len(diags) != 0 {
				t.Errorf("диагностик = %d, при ErrInvalidOptions ожидалось 0: %+v", len(diags), diags)
			}
			if !strings.HasPrefix(err.Error(), "metrics: некорректные Options") {
				t.Errorf("текст = %q, ожидался префикс сентинела", err.Error())
			}
		})
	}
}
