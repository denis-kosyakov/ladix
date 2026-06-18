package eval

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// ExplainFire собирает человеко-читаемую строку «почему сработал триггер» (§C-5.3,
// наблюдаемость «почему»). ЕДИНЫЙ форматтер для обоих путей (run и serve), чтобы
// формулировки не разошлись. Текст дословно из якоря reliability-model.md §C-5.3
// (Принцип VIII):
//
//	run   (withEdge=false): триггер '<имя> <оп> <порог>' сработал: <имя> = <снимок> (снимок) <оп> <порог> (порог) → истина
//	serve (withEdge=true):  триггер '<имя> <оп> <порог>' сработал (ребро ложь→истина): <имя> = <снимок> (снимок) <оп> <порог> (порог) → истина
//
// Семантика различий путей (НЕ опечатка, R-fmt-2): run = fire-if-true (без ребра);
// serve = фронт ложь→истина (durable edge). Числа рендерятся value.String — БЕЗ
// разделителей-подчёркиваний (3000000, не 3_000_000; R-fmt-1). Оператор — лексема
// BinOp.String (ast/op.go). Чистая функция: тот же снимок/порог/оператор → та же
// строка (R-fmt-5, детерминизм, пригодно для exact-match golden).
func ExplainFire(name string, op ast.CompOp, snapshot, threshold value.Value, withEdge bool) string {
	opStr := ast.BinOp(op).String()
	snapStr := value.String(snapshot)
	threshStr := value.String(threshold)
	label := fmt.Sprintf("%s %s %s", name, opStr, threshStr)
	marker := ""
	if withEdge {
		marker = " (ребро ложь→истина)"
	}
	return fmt.Sprintf(
		"триггер '%s' сработал%s: %s = %s (снимок) %s %s (порог) → истина",
		label, marker, name, snapStr, opStr, threshStr,
	)
}
