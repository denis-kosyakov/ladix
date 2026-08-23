package ast

// Program — корень дерева: элементы верхнего уровня в порядке исходника плюс
// позиция завершающего токена EOF (разбор валидной программы завершается ровно
// на EOF, FR-007, C-1.1). Корень не является Node — отдельной позиции не несёт.
type Program struct {
	Items  []TopLevelItem
	EOFPos Position // позиция токена EOF
}

// Block — тело если/пока/для/функции: непустая последовательность операторов на
// виртуальных токенах INDENT/DEDENT. Pos() — позиция INDENT/первого оператора.
// Пустые блоки запрещены (минимум 1 оператор) — узел добавляется в US3.
type Block struct {
	base
	Stmts []Statement
}

// NewBlock строит блок; pos — позиция INDENT/первого оператора (D4).
func NewBlock(pos Position, stmts []Statement) *Block {
	return &Block{base: base{position: pos}, Stmts: stmts}
}
