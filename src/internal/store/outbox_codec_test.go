package store

import (
	"fmt"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// outbox_codec_test.go — round-trip кодека OutboxRecord (M3-C2b, §C-2b.6,
// contracts/outbox-codec.md). Переиспользуем существующий value-кодек:
//   Args   → encodeList(value.NewList(args)) / decodeList → .Elems
//   Result → encodeValue / decodeValue (None → tagged-Пусто blob, НЕ SQL NULL)
// Новых форматов нет; цель — доказать lossless round-trip и tagged-None.

// encodeArgs/decodeArgs — внутренние хелперы кодека outbox (зеркалят то, что
// делают SQLiteStore.SaveOutbox/LoadOutbox). Здесь — белый ящик над codec.go.
func encodeArgs(args []value.Value) (string, error) {
	b, err := encodeValue(value.NewList(args))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeArgs(s string) ([]value.Value, error) {
	v, err := decodeValue([]byte(s))
	if err != nil {
		return nil, err
	}
	lst, ok := v.(value.Список)
	if !ok {
		return nil, fmt.Errorf("args_json не Список: %T", v)
	}
	return *lst.Elems, nil
}

func TestOutboxCodecArgsRoundTrip(t *testing.T) {
	args := []value.Value{
		value.Целое{V: 42},
		value.Строка{V: "итог звонка"},
		value.None,
	}
	enc, err := encodeArgs(args)
	if err != nil {
		t.Fatalf("encodeArgs: %v", err)
	}
	got, err := decodeArgs(enc)
	if err != nil {
		t.Fatalf("decodeArgs: %v", err)
	}
	if len(got) != len(args) {
		t.Fatalf("длина: got %d, want %d", len(got), len(args))
	}
	for i := range args {
		if value.String(got[i]) != value.String(args[i]) {
			t.Errorf("элемент %d: got %q, want %q", i, value.String(got[i]), value.String(args[i]))
		}
		if got[i].TypeName() != args[i].TypeName() {
			t.Errorf("элемент %d тип: got %s, want %s", i, got[i].TypeName(), args[i].TypeName())
		}
	}
}

func TestOutboxCodecResultRoundTrip(t *testing.T) {
	res := value.Целое{V: 2500000}
	b, err := encodeValue(res)
	if err != nil {
		t.Fatalf("encodeValue: %v", err)
	}
	got, err := decodeValue(b)
	if err != nil {
		t.Fatalf("decodeValue: %v", err)
	}
	gi, ok := got.(value.Целое)
	if !ok || gi.V != res.V {
		t.Fatalf("result round-trip: got %#v, want %#v", got, res)
	}
}

// TestOutboxCodecResultNoneIsTaggedBlob — None кодируется как непустой
// tagged-Пусто blob (НЕ SQL NULL); decode возвращает value.None.
// Мутпроба (см. T030/контракт): хранить None как SQL NULL → decode не-None → краснит.
func TestOutboxCodecResultNoneIsTaggedBlob(t *testing.T) {
	b, err := encodeValue(value.None)
	if err != nil {
		t.Fatalf("encodeValue(None): %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("None закодировано пустым blob — должно быть tagged-Пусто, не NULL")
	}
	got, err := decodeValue(b)
	if err != nil {
		t.Fatalf("decodeValue(None blob): %v", err)
	}
	if _, ok := got.(value.Пусто); !ok {
		t.Fatalf("decode tagged-None дал %T, ожидался value.Пусто", got)
	}
}

func TestOutboxCodecEmptyArgs(t *testing.T) {
	enc, err := encodeArgs([]value.Value{})
	if err != nil {
		t.Fatalf("encodeArgs(пусто): %v", err)
	}
	got, err := decodeArgs(enc)
	if err != nil {
		t.Fatalf("decodeArgs(пусто): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("пустые Args round-trip: got len %d, want 0", len(got))
	}
}
