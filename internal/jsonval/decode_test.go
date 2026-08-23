package jsonval

import (
	"bytes"
	"math"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// TestPayloadToRecordValueTypes — декод JSON-payload в value-типы (§9.3): различение
// Целое/Дробное по форме токена, деградация int64-overflow → Дробное (numberToValue),
// построение Списка из JSON-массива (decodeArray), доступ к полю/None у открытой Записи.
// ПЕРЕНЕСЁН из daemon/events_test.go (B2 лифт декодера в jsonval); упражняет лифтнутый
// PayloadToRecord/DecodeValue напрямую.
func TestPayloadToRecordValueTypes(t *testing.T) {
	rec, err := PayloadToRecord(`{"n":3,"f":1.5,"xs":[1,2],"big":99999999999999999999}`)
	if err != nil {
		t.Fatalf("PayloadToRecord: %v", err)
	}

	// Целое: число без '.'/'e' в пределах int64 → value.Целое.
	if got, ok := rec.Get("n").(value.Целое); !ok || got.V != 3 {
		t.Errorf("поле n = %#v, хотим value.Целое{V:3}", rec.Get("n"))
	}
	// Дробное: число с '.' → value.Дробное (различение по форме токена, не по величине).
	if got, ok := rec.Get("f").(value.Дробное); !ok || got.V != 1.5 {
		t.Errorf("поле f = %#v, хотим value.Дробное{V:1.5}", rec.Get("f"))
	}
	// int64-overflow деградирует в Дробное (payload толерантен: приближение > сбой доставки).
	if _, ok := rec.Get("big").(value.Дробное); !ok {
		t.Errorf("поле big (вне int64) = %#v, хотим value.Дробное (деградация)", rec.Get("big"))
	}
	// Список: JSON-массив → value.Список с поэлементным декодом.
	xs, ok := rec.Get("xs").(value.Список)
	if !ok {
		t.Fatalf("поле xs = %#v, хотим value.Список", rec.Get("xs"))
	}
	elems := *xs.Elems
	if len(elems) != 2 {
		t.Fatalf("список xs: длина = %d, хотим 2", len(elems))
	}
	if e0, ok := elems[0].(value.Целое); !ok || e0.V != 1 {
		t.Errorf("xs[0] = %#v, хотим value.Целое{V:1}", elems[0])
	}
	if e1, ok := elems[1].(value.Целое); !ok || e1.V != 2 {
		t.Errorf("xs[1] = %#v, хотим value.Целое{V:2}", elems[1])
	}
	// Открытая Запись: отсутствующее поле → None (value.Пусто), без ошибки.
	if _, ok := rec.Get("нет_такого").(value.Пусто); !ok {
		t.Errorf("отсутствующее поле = %#v, хотим value.Пусто (None)", rec.Get("нет_такого"))
	}
}

// TestPayloadToRecordKeyOrder — порядок ключей Записи сохранён (детерминизм декода).
func TestPayloadToRecordKeyOrder(t *testing.T) {
	rec, err := PayloadToRecord(`{"я":1,"а":2,"м":3}`)
	if err != nil {
		t.Fatalf("PayloadToRecord: %v", err)
	}
	got := rec.Keys()
	want := []string{"я", "а", "м"}
	if len(got) != len(want) {
		t.Fatalf("ключи = %v, хотим %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ключ[%d] = %q, хотим %q (порядок появления)", i, got[i], want[i])
		}
	}
}

// TestPayloadToRecordNonObject — верхний уровень не объект → ошибка (§AU-5.2).
func TestPayloadToRecordNonObject(t *testing.T) {
	if _, err := PayloadToRecord(`[1,2,3]`); err == nil {
		t.Errorf("массив на верхнем уровне: хотим ошибку, получили nil")
	}
}

// TestPayloadToRecordEmpty — пустой payload → пустая Запись без ошибки.
func TestPayloadToRecordEmpty(t *testing.T) {
	rec, err := PayloadToRecord("   ")
	if err != nil {
		t.Fatalf("пустой payload: %v", err)
	}
	if len(rec.Keys()) != 0 {
		t.Errorf("пустой payload → ключи %v, хотим []", rec.Keys())
	}
}

// TestDecodeValueScalarAndObject — DecodeValue над разными верхними уровнями (для
// engine.webhookCaller: ответ вебхука может быть скаляром/массивом/объектом).
func TestDecodeValueScalarAndObject(t *testing.T) {
	// Скаляр (объект НЕ требуется, в отличие от PayloadToRecord).
	v, err := DecodeValue(NewDecoder(bytes.NewReader([]byte(`"ок"`))))
	if err != nil {
		t.Fatalf("DecodeValue(скаляр): %v", err)
	}
	if s, ok := v.(value.Строка); !ok || s.V != "ок" {
		t.Errorf("скаляр = %#v, хотим Строка{ок}", v)
	}
	// Объект.
	ov, err := DecodeValue(NewDecoder(bytes.NewReader([]byte(`{"статус":"ок"}`))))
	if err != nil {
		t.Fatalf("DecodeValue(объект): %v", err)
	}
	rec, ok := ov.(value.Запись)
	if !ok {
		t.Fatalf("объект = %#v, хотим Запись", ov)
	}
	if got, ok := rec.Get("статус").(value.Строка); !ok || got.V != "ок" {
		t.Errorf("статус = %#v, хотим Строка{ок}", rec.Get("статус"))
	}
}

// TestDecodeValueFloatOverflow — число вне диапазона float64 деградирует в
// Дробное{±Inf}, а НЕ в None (фикс D: толерантный контракт payload — число
// никогда не теряется при доставке). Float64 при ErrRange отдаёт ±Inf.
func TestDecodeValueFloatOverflow(t *testing.T) {
	// Положительный overflow → Дробное{+Inf}.
	v, err := DecodeValue(NewDecoder(bytes.NewReader([]byte(`1e400`))))
	if err != nil {
		t.Fatalf("DecodeValue(1e400): %v", err)
	}
	if d, ok := v.(value.Дробное); !ok || !math.IsInf(d.V, 1) {
		t.Errorf("1e400 = %#v, хотим value.Дробное{V:+Inf}", v)
	}
	// Отрицательный overflow → Дробное{-Inf}.
	nv, err := DecodeValue(NewDecoder(bytes.NewReader([]byte(`-1e400`))))
	if err != nil {
		t.Fatalf("DecodeValue(-1e400): %v", err)
	}
	if d, ok := nv.(value.Дробное); !ok || !math.IsInf(d.V, -1) {
		t.Errorf("-1e400 = %#v, хотим value.Дробное{V:-Inf}", nv)
	}
}

// === 028 A7 (FR-008, золотой замок): толерантный контракт numberToValue (payload)
// через КОНТРАКТНЫЙ путь PayloadToRecord — число НИКОГДА не деградирует в None.
// Замок до рейминга numberToValue→payloadNumberToValue. Дополняет
// TestDecodeValueFloatOverflow (тот через DecodeValue-скаляр), фиксируя поведение
// именно у PayloadToRecord+rec.Get. ±Inf/NaN — только math.IsInf/math.IsNaN, не ==. ===
//
// Эмпирически (research/contract A7): 1e400/-1e400 → Дробное{±Inf} (Float64 overflow,
// err проигнорирован); 99999999999999999999 (целое вне int64, без точки) → КОНЕЧНОЕ
// Дробное (~1e20, НЕ ±Inf — Float64 строки даёт конечное число). Во ВСЕХ случаях
// число остаётся Дробным (НИКОГДА не None). Мутация (вернуть None на overflow) краснит.
func TestPayloadNumberInfinity(t *testing.T) {
	t.Run("pinf", func(t *testing.T) {
		rec, err := PayloadToRecord(`{"x": 1e400}`)
		if err != nil {
			t.Fatalf("PayloadToRecord(1e400): %v", err)
		}
		d, ok := rec.Get("x").(value.Дробное)
		if !ok {
			t.Fatalf("x = %#v, хотим value.Дробное (НИКОГДА не None)", rec.Get("x"))
		}
		if !math.IsInf(d.V, +1) {
			t.Errorf("x = %v, хотим +Inf", d.V)
		}
	})
	t.Run("ninf", func(t *testing.T) {
		rec, err := PayloadToRecord(`{"x": -1e400}`)
		if err != nil {
			t.Fatalf("PayloadToRecord(-1e400): %v", err)
		}
		d, ok := rec.Get("x").(value.Дробное)
		if !ok {
			t.Fatalf("x = %#v, хотим value.Дробное (НИКОГДА не None)", rec.Get("x"))
		}
		if !math.IsInf(d.V, -1) {
			t.Errorf("x = %v, хотим -Inf", d.V)
		}
	})
	t.Run("big_int_no_dot_finite", func(t *testing.T) {
		rec, err := PayloadToRecord(`{"x": 99999999999999999999}`)
		if err != nil {
			t.Fatalf("PayloadToRecord(big): %v", err)
		}
		// Целое вне int64 без точки: НЕ None, стало Дробным — и КОНЕЧНЫМ (~1e20).
		d, ok := rec.Get("x").(value.Дробное)
		if !ok {
			t.Fatalf("x = %#v, хотим value.Дробное (число НИКОГДА не None)", rec.Get("x"))
		}
		if math.IsInf(d.V, 0) || math.IsNaN(d.V) {
			t.Errorf("x = %v, хотим КОНЕЧНОЕ Дробное (НЕ ±Inf/NaN)", d.V)
		}
	})
}
