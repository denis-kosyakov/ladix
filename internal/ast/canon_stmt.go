package ast

import (
	"fmt"
	"strings"
)

// CanonicalStatement — тотальный рекурсивный канонизатор ОПЕРАТОРОВ: текстонезависимое
// представление смысла оператора одной строкой. Парная к canonExpr функция; введена
// фичей 029 для понижения тела шага процесса в ir.Step.Actions (канонические строки
// действий, FR-008).
//
// Покрывает ВСЕ 13 конкретных типов Statement; неизвестный тип — ПАНИКА (а не молчаливый
// default), чтобы новый узел AST не «провалился» в неопределённое представление IR —
// та же дисциплина, что у canonExpr (конституция III: паника как инвариант «не должно
// случиться»). Замок T-CANON-STMT (canon_stmt_test.go) проверяет исчерпываемость:
// удаление любой ветки уводит её тип в default-панику и краснит тест.
//
// Канон стабилен: он часть наблюдаемой формы ir.Program (schema_version 1), поэтому
// изменение формата строк ниже — breaking-изменение IR (bump SchemaVersion).
func CanonicalStatement(s Statement) string {
	switch v := s.(type) {
	case *LetStmt:
		return "пусть " + v.Name.Name + " = " + canonExpr(v.Value)
	case *AssignStmt:
		return v.Name.Name + " = " + canonExpr(v.Value)
	case *ExpressionStmt:
		return canonExpr(v.Expr)
	case *IfStmt:
		return "если " + canonExpr(v.Cond) + " " + canonBlock(v.Then) + canonElse(v.Else)
	case *WhileStmt:
		return "пока " + canonExpr(v.Cond) + " " + canonBlock(v.Body)
	case *TryStmt:
		return "пытаться " + canonBlock(v.Try) + " словить " + canonBlock(v.Catch)
	case *ForStmt:
		return "для " + v.Var.Name + " в " + canonExpr(v.Iterable) + " " + canonBlock(v.Body)
	case *ReturnStmt:
		if v.Value == nil {
			return "вернуть"
		}
		return "вернуть " + canonExpr(v.Value)
	case *BreakStmt:
		return "прервать"
	case *ContinueStmt:
		return "продолжить"
	case *AssignAction:
		return "присвоить " + v.Name.Name + " = " + canonExpr(v.Value)
	case *CallAction:
		return "вызвать " + v.Name.Name + "(" + strings.Join(mapCanon(v.Args), ",") + ")"
	case *NotifyAction:
		return "уведомить " + v.Name.Name + "(" + strings.Join(mapCanon(v.Args), ",") + ")"
	default:
		panic(fmt.Sprintf("CanonicalStatement: незнакомый тип оператора %T", s))
	}
}

// canonBlock канонизирует блок операторов: «{оп; оп; …}». nil/пустой блок → «{}»
// (штатно: парсер не даёт пустых армов, но канонизатор обязан быть тотальным).
func canonBlock(b *Block) string {
	if b == nil || len(b.Stmts) == 0 {
		return "{}"
	}
	parts := make([]string, len(b.Stmts))
	for i, st := range b.Stmts {
		parts[i] = CanonicalStatement(st)
	}
	return "{" + strings.Join(parts, "; ") + "}"
}

// canonElse канонизирует хвост ветвления: « иначе {…}» для финального иначе либо
// « иначе если <усл> {…}<хвост>» для промежуточной ветви. nil → пустая строка.
func canonElse(e *ElseClause) string {
	if e == nil {
		return ""
	}
	if e.Body != nil {
		return " иначе " + canonBlock(e.Body)
	}
	return " иначе если " + canonExpr(e.Cond) + " " + canonBlock(e.Then) + canonElse(e.Else)
}
