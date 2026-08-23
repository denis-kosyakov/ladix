package parser

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
)

// 029 try/catch фронтенд-тесты. Грамматика:
//   TryStmt ::= "пытаться" ":" Block "словить" ":" Block

// TestParseTryValid — валидный пытаться/словить парсится в *ast.TryStmt с
// заполненными армами Try/Catch (по ≥1 оператору). Pos() = токен пытаться.
func TestParseTryValid(t *testing.T) {
	src := "пытаться:\n    присвоить x = 1\n    вызвать crm(x)\nсловить:\n    присвоить x = 2\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("неожиданные диагностики: %s", el.Error())
	}
	ts, ok := prog.Items[0].(*ast.TryStmt)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *ast.TryStmt", prog.Items[0])
	}
	if ts.Try == nil || len(ts.Try.Stmts) != 2 {
		t.Errorf("Try арм = %+v, хотим 2 оператора", ts.Try)
	}
	if ts.Catch == nil || len(ts.Catch.Stmts) != 1 {
		t.Errorf("Catch арм = %+v, хотим 1 оператор", ts.Catch)
	}
	if ts.Pos().Line != 1 {
		t.Errorf("Pos().Line = %d, хотим 1 (токен пытаться)", ts.Pos().Line)
	}
}

// TestParseDanglingCatch — словить без предшествующего пытаться → SE-CATCH-NO-TRY
// (первая диагностика — на токене словить). Мутпроба: убрать кейс KW_CATCH из
// parseStatement → словить пойдёт в parseExprStatement, текст сменится.
func TestParseDanglingCatch(t *testing.T) {
	_, el := parseProgramSrc(t, "словить:\n    присвоить x = 1\n")
	if el.Empty() {
		t.Fatalf("ожидалась диагностика SE-CATCH-NO-TRY, получено 0")
	}
	if got := el.Errors()[0].Error(); !strings.Contains(got, msgCatchNoTry) {
		t.Errorf("первая диагностика = %q, хотим SE-CATCH-NO-TRY %q", got, msgCatchNoTry)
	}
}

// TestParseTryEmptyArm — пустой арм пытаться → существующий SE-EMPTY-BLOCK (новых
// кодов не вводим).
func TestParseTryEmptyArm(t *testing.T) {
	_, el := parseProgramSrc(t, "пытаться:\nсловить:\n    присвоить x = 1\n")
	if !strings.Contains(el.Error(), "пустой блок не допускается") {
		t.Errorf("пустой try-арм не дал SE-EMPTY-BLOCK:\n%s", el.Error())
	}
}

// TestParseTryMissingCatch — пропуск словить после пытаться → существующий
// SE-EXPECTED 'словить' (словить-арм обязателен в v1).
func TestParseTryMissingCatch(t *testing.T) {
	_, el := parseProgramSrc(t, "пытаться:\n    присвоить x = 1\n")
	if !strings.Contains(el.Error(), "ожидалось 'словить'") {
		t.Errorf("пропуск словить не дал SE-EXPECTED 'словить':\n%s", el.Error())
	}
}
