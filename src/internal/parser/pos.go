package parser

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
)

// toASTPos конвертирует позицию лексера/ошибок в локальную позицию AST. Helper
// живёт ИМЕННО в parser (не в ast): парсер — единственное место, где видны обе
// стороны (читает Token.Pos, строит узлы ast); это сохраняет листовость ast (D1).
func toASTPos(p errors.Position) ast.Position {
	return ast.Position{Line: p.Line, Col: p.Col}
}
