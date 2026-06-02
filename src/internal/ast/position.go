package ast

// Position — позиция узла AST в исходном тексте.
//
// Отсчёт 1-based; колонка считается в РУНАХ (Unicode code points), не в байтах
// (конституция IV) — язык кириллический. Тип ЛОКАЛЬНЫЙ для ast: структурно
// дублирует errors.Position, но НЕ разделяет его — это сохраняет листовость
// пакета ast (D1, guardrail 1). Конвертер errors.Position → ast.Position живёт
// в пакете parser (pos.go), не здесь.
//
// Инварианты для любого реального узла: Line ≥ 1, Col ≥ 1.
type Position struct {
	Line int // номер строки, 1-based
	Col  int // номер колонки в рунах, 1-based
}
