package eval

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

func TestEnvironmentLookupUpChain(t *testing.T) {
	global := NewEnvironment(nil)
	global.Define("x", value.Целое{V: 1})
	frame := NewEnvironment(global)
	frame.Define("y", value.Целое{V: 2})

	// Lookup поднимается по цепочке
	if v, ok := frame.Lookup("x"); !ok || !value.Equal(v, value.Целое{V: 1}) {
		t.Errorf("Lookup(x) из кадра = %v, %v", v, ok)
	}
	if v, ok := frame.Lookup("y"); !ok || !value.Equal(v, value.Целое{V: 2}) {
		t.Errorf("Lookup(y) = %v, %v", v, ok)
	}
	// промах
	if _, ok := frame.Lookup("z"); ok {
		t.Errorf("Lookup(z) должен промахнуться")
	}
}

func TestEnvironmentAssignMissReportsUndeclared(t *testing.T) {
	env := NewEnvironment(nil)
	if env.Assign("нет", value.Целое{V: 1}) {
		t.Errorf("Assign по необъявленному имени вернул true")
	}
	env.Define("есть", value.Целое{V: 0})
	if !env.Assign("есть", value.Целое{V: 5}) {
		t.Fatalf("Assign по объявленному имени вернул false")
	}
	if v, _ := env.Lookup("есть"); !value.Equal(v, value.Целое{V: 5}) {
		t.Errorf("после Assign значение = %v, хотим 5", v)
	}
}

func TestEnvironmentAssignMutatesParent(t *testing.T) {
	global := NewEnvironment(nil)
	global.Define("счёт", value.Целое{V: 0})
	frame := NewEnvironment(global)
	// Assign из кадра находит привязку в родителе и мутирует её
	if !frame.Assign("счёт", value.Целое{V: 9}) {
		t.Fatalf("Assign не нашёл привязку в родителе")
	}
	if v, _ := global.Lookup("счёт"); !value.Equal(v, value.Целое{V: 9}) {
		t.Errorf("родительская привязка не мутирована: %v", v)
	}
}

func TestEnvironmentHasLocal(t *testing.T) {
	global := NewEnvironment(nil)
	global.Define("g", value.None)
	frame := NewEnvironment(global)
	if frame.hasLocal("g") {
		t.Errorf("hasLocal видит имя из родителя")
	}
	frame.Define("l", value.None)
	if !frame.hasLocal("l") {
		t.Errorf("hasLocal не видит локальное имя")
	}
}
