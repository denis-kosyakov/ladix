package ir

import (
	"reflect"
	"strings"
	"testing"
)

// TestSchemaVersionIsOne — замок A11/FR-006: версия схемы в v1 равна 1.
// Краснеет при любом изменении константы — что и требуется: bump SchemaVersion
// обязан быть осознанным актом с обновлением этого замка и документации.
func TestSchemaVersionIsOne(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, ожидалась 1 (bump требует правки политики версионирования)", SchemaVersion)
	}
}

// TestVocabularies — словари Severity/Stage/Kind в v1 зафиксированы дословно.
func TestVocabularies(t *testing.T) {
	cases := map[string]string{
		"SeverityError": SeverityError,
		"StageLexical":  StageLexical,
		"StageSyntax":   StageSyntax,
		"StageSemantic": StageSemantic,
		"KindMetric":    KindMetric,
		"KindSchedule":  KindSchedule,
		"KindEvent":     KindEvent,
		"KindDeadline":  KindDeadline,
	}
	want := map[string]string{
		"SeverityError": "error",
		"StageLexical":  "lexical",
		"StageSyntax":   "syntax",
		"StageSemantic": "semantic",
		"KindMetric":    "metric",
		"KindSchedule":  "schedule",
		"KindEvent":     "event",
		"KindDeadline":  "deadline",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q, ожидалось %q", name, got, want[name])
		}
	}
}

// TestJSONTagsSnakeCaseAndNoOmitempty — замок формы контракта (contracts/ir-schema.md):
// КАЖДОЕ экспортированное поле КАЖДОГО типа IR несёт json-тег в snake_case и НЕ несёт
// omitempty (незаданные строки сериализуются как "", форма объекта стабильна).
func TestJSONTagsSnakeCaseAndNoOmitempty(t *testing.T) {
	types := []any{
		Program{}, Metric{}, Process{}, Step{}, Trigger{}, Diagnostic{}, Position{},
	}
	for _, v := range types {
		rt := reflect.TypeOf(v)
		t.Run(rt.Name(), func(t *testing.T) {
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				tag, ok := f.Tag.Lookup("json")
				if !ok || tag == "" {
					t.Errorf("%s.%s: json-тег отсутствует", rt.Name(), f.Name)
					continue
				}
				if strings.Contains(tag, "omitempty") {
					t.Errorf("%s.%s: omitempty запрещён (форма объекта должна быть стабильной)", rt.Name(), f.Name)
				}
				name := strings.Split(tag, ",")[0]
				if name != strings.ToLower(name) || strings.Contains(name, "-") {
					t.Errorf("%s.%s: json-тег %q не в snake_case", rt.Name(), f.Name, name)
				}
			}
		})
	}
}

// TestIRIsLeafPackage — замок листовости (Конституция VII): ir не тянет ни ast,
// ни errors, ни value — он несёт СОБСТВЕННУЮ Position. Проверяется структурно:
// поле Pos имеет тип именно ir.Position.
func TestIRIsLeafPackage(t *testing.T) {
	want := reflect.TypeOf(Position{})
	for _, v := range []any{Metric{}, Process{}, Step{}, Trigger{}, Diagnostic{}} {
		rt := reflect.TypeOf(v)
		f, ok := rt.FieldByName("Pos")
		if !ok {
			t.Errorf("%s: нет поля Pos", rt.Name())
			continue
		}
		if f.Type != want {
			t.Errorf("%s.Pos имеет тип %v, ожидался ir.Position (ir обязан оставаться листовым)", rt.Name(), f.Type)
		}
	}
}
