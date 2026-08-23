package ir

import (
	"encoding/json"
	"reflect"
	"testing"
)

// goldenProgram — фикстура контракта v1 (contracts/ir-schema.md): одна метрика,
// один процесс с одним шагом, один метрик-триггер.
func goldenProgram() Program {
	return Program{
		SchemaVersion: SchemaVersion,
		Metrics: []Metric{{
			Name:      "выручка",
			Source:    "продажи",
			Where:     `(статус == "оплачено")`,
			Aggregate: "сумма(сумма_заказа)",
			Period:    "ежемесячно",
			ByDate:    "дата_заказа",
			Pos:       Position{Line: 3, Col: 1},
		}},
		Processes: []Process{{
			Name:   "обработка_заказа",
			Params: []string{"заказ"},
			Steps: []Step{{
				Name:     "подтвердить",
				After:    "",
				Assignee: "менеджер",
				Deadline: "длит(2|дн)",
				Actions:  []string{"уведомить менеджер(заказ)"},
				Pos:      Position{Line: 12, Col: 3},
			}},
			Pos: Position{Line: 11, Col: 1},
		}},
		Triggers: []Trigger{{
			Kind:      KindMetric,
			Metric:    "выручка",
			Op:        "<",
			Threshold: "1000000",
			Process:   "обработка_заказа",
			Step:      "подтвердить",
			Pos:       Position{Line: 20, Col: 1},
		}},
	}
}

// goldenJSON — БАЙТ-ТОЧНАЯ сериализация goldenProgram. Замок формы контракта v1
// (FR-021): любое изменение имён/порядка/наличия полей краснит тест и обязано
// сопровождаться решением «аддитивно или bump SchemaVersion».
//
// Оператор "<" виден здесь как \u003c: encoding/json по умолчанию экранирует
// <, > и & (HTML-безопасность). Это деталь КОДИРОВАНИЯ, а не формы контракта —
// при декодировании возвращается "<" (проверяется round-trip ниже).
const goldenJSON = `{"schema_version":1,"metrics":[{"name":"выручка","source":"продажи","where":"(статус == \"оплачено\")","aggregate":"сумма(сумма_заказа)","period":"ежемесячно","by_date":"дата_заказа","pos":{"line":3,"col":1}}],"processes":[{"name":"обработка_заказа","params":["заказ"],"steps":[{"name":"подтвердить","after":"","assignee":"менеджер","deadline":"длит(2|дн)","actions":["уведомить менеджер(заказ)"],"pos":{"line":12,"col":3}}],"pos":{"line":11,"col":1}}],"triggers":[{"kind":"metric","metric":"выручка","op":"\u003c","threshold":"1000000","event":"","schedule":"","process":"обработка_заказа","step":"подтвердить","pos":{"line":20,"col":1}}]}`

// TestJSONGoldenRoundTrip — контракт v1 (FR-021): Marshal даёт байт-точный golden,
// Unmarshal возвращает исходную структуру без потерь.
func TestJSONGoldenRoundTrip(t *testing.T) {
	src := goldenProgram()

	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != goldenJSON {
		t.Errorf("форма JSON разошлась с контрактом v1.\nполучено: %s\nожидалось: %s", b, goldenJSON)
	}

	var back Program
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(src, back) {
		t.Errorf("round-trip потерял данные:\nбыло:  %+v\nстало: %+v", src, back)
	}
}

// TestJSONForwardCompat — замок FR-020: декодер ОБЯЗАН пережить будущее
// расширение схемы — неизвестное поле, неизвестный severity, неизвестный stage.
// Это контракт, который данный пакет предъявляет и себе, и потребителю.
func TestJSONForwardCompat(t *testing.T) {
	future := `{
		"schema_version": 1,
		"metrics": [{"name":"выручка","source":"продажи","where":"","aggregate":"","period":"","by_date":"","pos":{"line":1,"col":1},"unit":"руб"}],
		"processes": [],
		"triggers": [],
		"annotations": {"origin":"future-version"}
	}`
	var p Program
	if err := json.Unmarshal([]byte(future), &p); err != nil {
		t.Fatalf("неизвестные поля обязаны игнорироваться, получена ошибка: %v", err)
	}
	if len(p.Metrics) != 1 || p.Metrics[0].Name != "выручка" {
		t.Errorf("известные поля потеряны при наличии неизвестных: %+v", p.Metrics)
	}

	futureDiag := `{"severity":"warning","stage":"typecheck","message":"будущая диагностика","pos":{"line":2,"col":4},"code":"W-001"}`
	var d Diagnostic
	if err := json.Unmarshal([]byte(futureDiag), &d); err != nil {
		t.Fatalf("неизвестные severity/stage/поле обязаны декодироваться, получена ошибка: %v", err)
	}
	if d.Severity != "warning" || d.Stage != "typecheck" {
		t.Errorf("неизвестные значения словарей обязаны сохраняться как есть: %+v", d)
	}
}
