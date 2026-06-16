package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// parseProgramSrc лексирует и разбирает целую программу; возвращает дерево и
// накопитель синтаксических ошибок (изолированный, без лексических).
func parseProgramSrc(t *testing.T, src string) (*ast.Program, *errors.ErrorList) {
	t.Helper()
	toks := lexTokens(t, src)
	el := errors.NewErrorList()
	return New(toks, el).Parse(), el
}

// parseExampleFile разбирает examples/<name> из корня репозитория; возвращает
// дерево, синтаксические ошибки парсера и лексические ошибки.
func parseExampleFile(t *testing.T, name string) (*ast.Program, *errors.ErrorList, *errors.ErrorList) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", name))
	if err != nil {
		t.Fatalf("читать %s: %v", name, err)
	}
	toks, lexErrs := lexer.New(string(data)).Tokenize()
	el := errors.NewErrorList()
	prog := New(toks, el).Parse()
	return prog, el, lexErrs
}

// T025: каркас программы, let/assign/выражение, печать как обычный CallExpr,
// завершение на EOF, SE-ASSIGN-TARGET, parse-clean примеров.

func TestProgramSkeleton(t *testing.T) {
	src := "пусть a = 2 + 3 * 4\nx = x + 1\nпечать(a, b)\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("неожиданные ошибки: %v", el.Error())
	}
	if len(prog.Items) != 3 {
		t.Fatalf("Items = %d, хотим 3", len(prog.Items))
	}
	if prog.EOFPos.Line < 1 || prog.EOFPos.Col < 1 {
		t.Errorf("EOFPos не зафиксирован: %+v", prog.EOFPos)
	}

	ls, ok := prog.Items[0].(*ast.LetStmt)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *LetStmt", prog.Items[0])
	}
	if ls.Name.Name != "a" || sexpr(ls.Value) != "(+ 2 (* 3 4))" {
		t.Errorf("LetStmt неверен: name=%q value=%s", ls.Name.Name, sexpr(ls.Value))
	}

	as, ok := prog.Items[1].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("Items[1] = %T, хотим *AssignStmt", prog.Items[1])
	}
	if as.Name.Name != "x" || sexpr(as.Value) != "(+ x 1)" {
		t.Errorf("AssignStmt неверен: name=%q value=%s", as.Name.Name, sexpr(as.Value))
	}

	es, ok := prog.Items[2].(*ast.ExpressionStmt)
	if !ok {
		t.Fatalf("Items[2] = %T, хотим *ExpressionStmt", prog.Items[2])
	}
	call, ok := es.Expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("ExpressionStmt.Expr = %T, хотим *CallExpr (печать — обычный вызов)", es.Expr)
	}
	if id, ok := call.Callee.(*ast.Ident); !ok || id.Name != "печать" {
		t.Errorf("Callee не Ident(печать): %T", call.Callee)
	}
	if len(call.Args) != 2 {
		t.Errorf("аргументов %d, хотим 2", len(call.Args))
	}
}

func TestPrintIsPlainCall(t *testing.T) {
	prog, el := parseProgramSrc(t, `печать("Привет, Уклад!")`+"\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	if len(prog.Items) != 1 {
		t.Fatalf("Items = %d, хотим 1", len(prog.Items))
	}
	es := prog.Items[0].(*ast.ExpressionStmt)
	if got := sexpr(es.Expr); got != `(call печать "Привет, Уклад!")` {
		t.Errorf("дерево = %s, хотим (call печать \"Привет, Уклад!\")", got)
	}
}

func TestAssignTargetError(t *testing.T) {
	tests := []struct {
		src string
		col int // колонка токена '='
	}{
		{"x.поле = 5\n", 8},
		{"x[i] = 5\n", 6},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			_, el := parseProgramSrc(t, tt.src)
			if el.Len() != 1 {
				t.Fatalf("ошибок %d, хотим 1 (SE-ASSIGN-TARGET): %v", el.Len(), el.Error())
			}
			var pe errors.ParseError
			if !errAs(el, &pe) {
				t.Fatalf("не ParseError")
			}
			if pe.Msg != msgAssignTarget {
				t.Errorf("Msg = %q, хотим %q", pe.Msg, msgAssignTarget)
			}
			if pe.Pos.Line != 1 || pe.Pos.Col != tt.col {
				t.Errorf("позиция = %+v, хотим {1,%d} (токен '=')", pe.Pos, tt.col)
			}
		})
	}
}

func TestExamplesParseCleanP1(t *testing.T) {
	for _, name := range []string{"hello.ladix", "арифметика.ladix"} {
		t.Run(name, func(t *testing.T) {
			prog, el, lexErrs := parseExampleFile(t, name)
			if !lexErrs.Empty() {
				t.Fatalf("%s: лексические ошибки: %v", name, lexErrs.Error())
			}
			if !el.Empty() {
				t.Fatalf("%s: синтаксические ошибки: %v", name, el.Error())
			}
			if len(prog.Items) == 0 {
				t.Errorf("%s: пустой Program.Items", name)
			}
		})
	}
}

// firstParseError возвращает первую накопленную ошибку как ParseError.
func firstParseError(t *testing.T, el *errors.ErrorList) errors.ParseError {
	t.Helper()
	if el.Len() == 0 {
		t.Fatalf("ошибок нет")
	}
	pe, ok := el.Errors()[0].(errors.ParseError)
	if !ok {
		t.Fatalf("первая ошибка не ParseError: %T", el.Errors()[0])
	}
	return pe
}

// T029: условия/циклы/блоки, вложенность, голый/значимый вернуть, SE-EMPTY-BLOCK,
// иначеесли слитно → SE-EXPECTED, примеры условие/цикл parse-clean.

func TestIfElseChain(t *testing.T) {
	src := "если a >= 10000:\n    c = 1\nиначе если a >= 3000:\n    c = 2\nиначе:\n    c = 3\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	is, ok := prog.Items[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *IfStmt", prog.Items[0])
	}
	if sexpr(is.Cond) != "(>= a 10000)" {
		t.Errorf("Cond = %s", sexpr(is.Cond))
	}
	if is.Else == nil || is.Else.IsFinal() {
		t.Fatalf("Else должен быть ветвью 'иначе если'")
	}
	if sexpr(is.Else.Cond) != "(>= a 3000)" {
		t.Errorf("else-if Cond = %s", sexpr(is.Else.Cond))
	}
	if is.Else.Else == nil || !is.Else.Else.IsFinal() {
		t.Errorf("последняя ветвь должна быть финальным 'иначе'")
	}
}

func TestNestedBlocks(t *testing.T) {
	src := "для i в r:\n    если c:\n        прервать\n    x = 1\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	fs, ok := prog.Items[0].(*ast.ForStmt)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *ForStmt", prog.Items[0])
	}
	if fs.Var.Name != "i" {
		t.Errorf("Var = %q, хотим i", fs.Var.Name)
	}
	if len(fs.Body.Stmts) != 2 {
		t.Fatalf("тело для: %d операторов, хотим 2 (если + присваивание)", len(fs.Body.Stmts))
	}
	inner, ok := fs.Body.Stmts[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("вложенный[0] = %T, хотим *IfStmt", fs.Body.Stmts[0])
	}
	if len(inner.Then.Stmts) != 1 {
		t.Fatalf("тело если: %d, хотим 1", len(inner.Then.Stmts))
	}
	if _, ok := inner.Then.Stmts[0].(*ast.BreakStmt); !ok {
		t.Errorf("вложенный оператор = %T, хотим *BreakStmt", inner.Then.Stmts[0])
	}
}

func TestForCallIterableAndWhileBreak(t *testing.T) {
	prog, el := parseProgramSrc(t, "для i в диапазон(1, 11):\n    x = i\n")
	if !el.Empty() {
		t.Fatalf("for: ошибки %v", el.Error())
	}
	fs := prog.Items[0].(*ast.ForStmt)
	if sexpr(fs.Iterable) != "(call диапазон 1 11)" {
		t.Errorf("Iterable = %s, хотим (call диапазон 1 11)", sexpr(fs.Iterable))
	}

	prog2, el2 := parseProgramSrc(t, "пока истина:\n    прервать\n")
	if !el2.Empty() {
		t.Fatalf("while: ошибки %v", el2.Error())
	}
	ws := prog2.Items[0].(*ast.WhileStmt)
	if sexpr(ws.Cond) != "истина" {
		t.Errorf("while Cond = %s", sexpr(ws.Cond))
	}
	if _, ok := ws.Body.Stmts[0].(*ast.BreakStmt); !ok {
		t.Errorf("тело пока: %T, хотим *BreakStmt", ws.Body.Stmts[0])
	}
}

func TestReturnBareAndValue(t *testing.T) {
	// внутри функции тела (используем блок если, чтобы вернуть был в блоке)
	prog, el := parseProgramSrc(t, "если c:\n    вернуть\nиначе:\n    вернуть x + 1\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	is := prog.Items[0].(*ast.IfStmt)
	bare, ok := is.Then.Stmts[0].(*ast.ReturnStmt)
	if !ok || bare.Value != nil {
		t.Errorf("голый вернуть: %+v", is.Then.Stmts[0])
	}
	withVal := is.Else.Body.Stmts[0].(*ast.ReturnStmt)
	if withVal.Value == nil || sexpr(withVal.Value) != "(+ x 1)" {
		t.Errorf("вернуть E: %v", withVal.Value)
	}
}

func TestEmptyBlockError(t *testing.T) {
	// тело если пустое: следующая строка на том же уровне (INDENT не эмитится)
	_, el := parseProgramSrc(t, "если x:\nпечать(1)\n")
	pe := firstParseError(t, el)
	if pe.Msg != msgEmptyBlock {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, msgEmptyBlock)
	}
	if pe.Pos.Line != 2 || pe.Pos.Col != 1 {
		t.Errorf("позиция SE-EMPTY-BLOCK = %+v, хотим {2,1}", pe.Pos)
	}
}

func TestElseIfGluedError(t *testing.T) {
	// 'иначеесли' слитно — это IDENT, не KW_ELSE KW_IF → SE-EXPECTED 'конец строки'
	src := "если x:\n    a = 1\nиначеесли C:\n    b = 2\n"
	_, el := parseProgramSrc(t, src)
	pe := firstParseError(t, el)
	if pe.Msg != "ожидалось 'конец строки', получено 'C'" {
		t.Errorf("Msg = %q, хотим \"ожидалось 'конец строки', получено 'C'\"", pe.Msg)
	}
	if pe.Pos.Line != 3 || pe.Pos.Col != 11 {
		t.Errorf("позиция = %+v, хотим {3,11} (токен 'C' после 'иначеесли ')", pe.Pos)
	}
}

func TestExamplesParseCleanP2a(t *testing.T) {
	for _, name := range []string{"условие.ladix", "цикл.ladix"} {
		t.Run(name, func(t *testing.T) {
			prog, el, lexErrs := parseExampleFile(t, name)
			if !lexErrs.Empty() {
				t.Fatalf("%s: лексические ошибки: %v", name, lexErrs.Error())
			}
			if !el.Empty() {
				t.Fatalf("%s: синтаксические ошибки: %v", name, el.Error())
			}
			if len(prog.Items) == 0 {
				t.Errorf("%s: пустой Program.Items", name)
			}
		})
	}
}

// T028 (§TR-3 шов A/B, D-TR-1, §TR-10.5 п.1/п.6; diagnostics.md §TR-7.F): краевые
// случаи ВЕДУЩЕГО top-level токена после развода `когда`/`значение`/`событие` по двум
// швам парсера. Комплементарно golden-замку TestGoldenSEUnexpectedTopLevel (там
// `значение`/`событие`/`{` остаются SE-UNEXPECTED) — здесь фиксируются: (1)
// `значение`/`событие`/`{`/`}` → msgUnexpected с точной позицией; (2) приём ведущего
// `когда` (шов A → parseTriggerDecl). СИММЕТРИЯ `значение`≡`событие` на top-level
// (§TR-7.F / FR-006/FR-020/SC-007): оба — первичные выражения (шов B, parsePrimary),
// но оба отвергаются isUnexpectedTopLevel ДО разбора выражения. Отказ `значение`/
// `событие` ВНЕ контекста-триггера в теле — забота СЕМПРОХОДА (TR-VAL-CTX/TR-EVT-CTX).
func TestTopLevelLeadingTokenEdgeCases(t *testing.T) {
	// (1) значение/событие/{/} — отвергаются isUnexpectedTopLevel ДО разбора
	//     выражения → msgUnexpected; позиция = ведущий токен (строка 1, колонка 1).
	t.Run("msgUnexpected", func(t *testing.T) {
		leads := []string{"значение", "событие", "{", "}"}
		for _, lead := range leads {
			t.Run(lead, func(t *testing.T) {
				_, el := parseProgramSrc(t, lead+"\n")
				if el.Len() != 1 {
					t.Fatalf("ошибок %d, хотим 1: %v", el.Len(), el.Error())
				}
				pe := firstParseError(t, el)
				want := "неожиданный элемент '" + lead + "'"
				if pe.Msg != want {
					t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
				}
				if pe.Pos.Line != 1 || pe.Pos.Col != 1 {
					t.Errorf("позиция = %+v, хотим {1,1} (ведущий токен)", pe.Pos)
				}
			})
		}
	})

	// (2) Подтверждение: isUnexpectedTopLevel БОЛЬШЕ не содержит KW_WHEN (шов A →
	//     parseTriggerDecl), но содержит KW_VALUE/KW_EVENT/LBRACE/RBRACE (зеркало
	//     предиката, без разбора). KW_EVENT отвергается симметрично KW_VALUE (§TR-7.F).
	t.Run("isUnexpectedTopLevel предикат", func(t *testing.T) {
		cases := []struct {
			tt   lexer.TokenType
			want bool
		}{
			{lexer.KW_WHEN, false}, // снят — шов A → parseTriggerDecl
			{lexer.KW_EVENT, true}, // отвергается (симметрия с значение, §TR-7.F)
			{lexer.KW_VALUE, true}, // остаётся недопустимым на top-level
			{lexer.LBRACE, true},
			{lexer.RBRACE, true},
		}
		for _, c := range cases {
			if got := isUnexpectedTopLevel(c.tt); got != c.want {
				t.Errorf("isUnexpectedTopLevel(%v) = %v, хотим %v", c.tt, got, c.want)
			}
		}
	})

	// (3) Ведущий `когда` БОЛЬШЕ не отвергается isUnexpectedTopLevel — он
	//     диспетчеризуется в parseTriggerDecl и парсится как триггер (НЕ
	//     `неожиданный элемент 'когда'`). Полный валидный триггер → 0 ошибок, узел
	//     *ast.TriggerDecl на верхнем уровне.
	t.Run("когда принят как триггер", func(t *testing.T) {
		prog, el := parseProgramSrc(t, "когда метрика m > 1:\n    печать(1)\n")
		if !el.Empty() {
			t.Fatalf("ведущий `когда` дал ошибки (хотим 0, шов A): %v", el.Error())
		}
		if len(prog.Items) != 1 {
			t.Fatalf("Items = %d, хотим 1 (TriggerDecl)", len(prog.Items))
		}
		if _, ok := prog.Items[0].(*ast.TriggerDecl); !ok {
			t.Errorf("Items[0] = %T, хотим *ast.TriggerDecl", prog.Items[0])
		}
	})

	// (4) Симметрия шова B (компенсация): `событие` — первичное выражение
	//     (parsePrimary, как и `значение`), но на top-level отвергается
	//     isUnexpectedTopLevel ДО разбора выражения. Регресс-страж бага «шов B
	//     добавил KW_EVENT в parsePrimary, не компенсировав в isUnexpectedTopLevel»:
	//     для `событие 5` нарушитель — ВЕДУЩИЙ `событие`, а НЕ хвост `5`. (Внутри
	//     тела триггера события `событие`/`событие.поле` валидны — позитивы US1.)
	t.Run("ведущее событие — unexpected (симметрия значение)", func(t *testing.T) {
		_, el := parseProgramSrc(t, "событие 5\n")
		pe := firstParseError(t, el)
		want := "неожиданный элемент 'событие'"
		if pe.Msg != want {
			t.Errorf("Msg = %q, хотим %q (нарушитель — ведущий 'событие', не хвост '5')", pe.Msg, want)
		}
		if pe.Pos.Line != 1 || pe.Pos.Col != 1 {
			t.Errorf("позиция = %+v, хотим {1,1} (ведущий токен)", pe.Pos)
		}
	})
}
