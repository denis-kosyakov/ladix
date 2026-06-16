package parser

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// parseFunctionDecl: функция Ident "(" ParamList? ")" ":" Block. Только верхний
// уровень (grammar §4). Pos() = токен функция.
func (p *Parser) parseFunctionDecl() *ast.FunctionDecl {
	fnTok := p.advance() // функция
	nameTok, _ := p.expect(lexer.IDENT, "имя функции")
	name := p.identFrom(nameTok)
	p.expect(lexer.LPAREN, "(")
	params := p.parseParamList()
	p.expect(lexer.RPAREN, ")")
	p.expect(lexer.COLON, ":")
	body := p.parseBlock()
	return ast.NewFunctionDecl(toASTPos(fnTok.Pos), *name, params, body)
}

// parseParamList разбирает позиционные параметры до ")" (висящая запятая).
func (p *Parser) parseParamList() []ast.Ident {
	var params []ast.Ident
	for !p.check(lexer.RPAREN) && !p.check(lexer.EOF) {
		nameTok, ok := p.expect(lexer.IDENT, "имя параметра")
		if !ok {
			break
		}
		params = append(params, *p.identFrom(nameTok))
		if !p.match(lexer.COMMA) {
			break
		}
	}
	return params
}

// sourceAttrName сопоставляет лексему ведущего токена строки атрибута источника
// фикс-набору {файл,тип,поля} — унифицированно по лексеме (зеркало metricAttrName,
// §SC-D-PARSE-1). `файл`→KW_FILE, `тип`→KW_TYPE, `поля`→IDENT — все матчатся по
// тексту лексемы, а не по типу токена.
func sourceAttrName(lexeme string) bool {
	switch lexeme {
	case "файл", "тип", "поля":
		return true
	}
	return false
}

// parseSourceDecl: источник Ident ":" NEWLINE INDENT SourceAttr+ DEDENT (§SM-3/
// §SC-4). SourceAttr ::= файл ":" STRING NEWLINE | тип ":" IDENT NEWLINE | поля ":"
// NEWLINE INDENT FieldDef+ DEDENT (§SC-D-PARSE). Неизвестный атрибут → §SM-9.A
// msgUnknownAttr; повтор → §SM-9.A msgDuplicateAttr (через seen map). Значение
// тип: — голый IDENT (валидация json|csv|ndjson семантическая, §SC-D-PARSE-2).
// Пустой блок → msgEmptyBlock. Pos() = токен источник.
func (p *Parser) parseSourceDecl() *ast.SourceDecl {
	srcTok := p.advance() // источник
	nameTok, _ := p.expect(lexer.IDENT, "имя источника")
	name := p.identFrom(nameTok)
	p.expect(lexer.COLON, ":")
	sd := ast.NewSourceDecl(toASTPos(srcTok.Pos), *name, ast.StringLit{}, ast.Position{})
	if !p.openAttrBlock() {
		return sd
	}
	var file *ast.StringLit
	var filePos lexer.Token
	seen := make(map[string]bool, 3)
	for !p.check(lexer.DEDENT) && !p.check(lexer.EOF) {
		before := p.pos
		attrTok := p.peek()
		lexeme := attrTok.Lexeme
		if !sourceAttrName(lexeme) {
			p.error(attrTok.Pos, msgUnknownAttrHint(lexeme, sourceAttrVocab))
			break
		}
		if seen[lexeme] {
			p.error(attrTok.Pos, msgDuplicateAttr(lexeme))
			break
		}
		seen[lexeme] = true
		p.advance() // ключевое слово/имя атрибута (файл|тип|поля)
		p.expect(lexer.COLON, ":")
		switch lexeme {
		case "файл":
			strTok, ok := p.expect(lexer.STRING, "путь к файлу")
			if ok {
				file = p.buildStringLit(strTok)
				filePos = strTok
			}
			p.expect(lexer.NEWLINE, "конец строки")
		case "тип":
			// §SC-D-PARSE-2: значение — голый IDENT; множество json|csv|ndjson
			// валидируется семантически (Analyze), не парсером.
			valTok, ok := p.expect(lexer.IDENT, "значение типа")
			if ok {
				sd.Type = *p.identFrom(valTok)
				sd.TypePos = toASTPos(attrTok.Pos)
			}
			p.expect(lexer.NEWLINE, "конец строки")
		case "поля":
			// §SC-D-PARSE-3: вложенный блок объявлений; позиция атрибута — слово поля.
			sd.FieldsPos = toASTPos(attrTok.Pos)
			sd.Fields = p.parseFieldBlock()
		}
		if p.pos == before {
			p.advance() // backstop: гарантия прогресса
		}
	}
	p.expect(lexer.DEDENT, "конец блока")
	if file != nil {
		sd.File = *file
		sd.FilePos = toASTPos(filePos.Pos)
	}
	return sd
}

// parseFieldBlock разбирает вложенный блок поля: — { IDENT(имя) ":" IDENT(тип)
// NEWLINE }+ (§SC-D-PARSE-3). Открытие — openAttrBlock() (NEWLINE + INDENT);
// пустой блок → msgEmptyBlock и nil. Собственный seen map на имена полей: дубль →
// msgDuplicateField + continue (восстановление как parseStepDecl — собрать
// несколько FieldDef, а не падать на первой). Закрытие — expect(DEDENT).
func (p *Parser) parseFieldBlock() []ast.FieldDef {
	if !p.openAttrBlock() {
		return nil
	}
	var fields []ast.FieldDef
	seen := make(map[string]bool, 4)
	for !p.check(lexer.DEDENT) && !p.check(lexer.EOF) {
		p.suppress = false // граница строки поля (доктрина recover.go)
		before := p.pos
		nameTok, ok := p.expect(lexer.IDENT, "имя поля")
		if !ok {
			if p.pos == before {
				p.advance() // backstop: гарантия прогресса
			}
			continue
		}
		p.expect(lexer.COLON, ":")
		typeTok, _ := p.expect(lexer.IDENT, "тип поля")
		p.expect(lexer.NEWLINE, "конец строки")
		if seen[nameTok.Lexeme] {
			p.error(nameTok.Pos, msgDuplicateField(nameTok.Lexeme))
			continue // строка дубля съедена synchronize; цикл полей продолжается
		}
		seen[nameTok.Lexeme] = true
		fields = append(fields, ast.FieldDef{
			Name:     *p.identFrom(nameTok),
			TypeName: *p.identFrom(typeTok),
			Pos:      toASTPos(nameTok.Pos),
		})
		if p.pos == before {
			p.advance() // backstop: гарантия прогресса
		}
	}
	p.expect(lexer.DEDENT, "конец блока")
	return fields
}

// parseSourceRef разбирает значение атрибута источник: — ровно один IDENT (D-1).
// Иной токен → §SM-9.A «ожидается имя источника». Возвращает Ident и ok.
func (p *Parser) parseSourceRef() (ast.Ident, bool) {
	if !p.check(lexer.IDENT) {
		p.error(p.peek().Pos, msgSourceName)
		return ast.Ident{}, false
	}
	tok := p.advance()
	return *p.identFrom(tok), true
}

// metricAttrName сопоставляет лексему ведущего токена строки атрибута метрики
// фикс-набору {источник,где,агрегат,период,по_дате} — унифицированно по лексеме
// (атрибуты лексируются как ключевые слова, но имя проверяется по тексту).
func metricAttrName(lexeme string) bool {
	switch lexeme {
	case "источник", "где", "агрегат", "период", "по_дате":
		return true
	}
	return false
}

// parseMetricDecl: метрика Ident ":" NEWLINE INDENT MetricAttr+ DEDENT (§SM-3).
// MetricAttr = attrName ":" value NEWLINE; источник: → parseSourceRef (D-1), прочие
// → parseExpression. Неизвестное имя/повтор → §SM-9.A. Обязательность и связку
// период↔по_дате парсер НЕ проверяет (D-4, семпроход). Pos() = токен метрика.
func (p *Parser) parseMetricDecl() *ast.MetricDecl {
	mTok := p.advance() // метрика
	nameTok, _ := p.expect(lexer.IDENT, "имя метрики")
	name := p.identFrom(nameTok)
	p.expect(lexer.COLON, ":")

	var source ast.Ident
	var where, aggregate, period, byDate ast.Expression
	var attrs ast.MetricAttrPos

	if !p.openAttrBlock() {
		return ast.NewMetricDecl(toASTPos(mTok.Pos), *name, source, where, aggregate, period, byDate, attrs)
	}
	seen := make(map[string]bool, 5)
	for !p.check(lexer.DEDENT) && !p.check(lexer.EOF) {
		before := p.pos
		attrTok := p.peek()
		lexeme := attrTok.Lexeme
		if !metricAttrName(lexeme) {
			p.error(attrTok.Pos, msgUnknownAttrHint(lexeme, metricAttrVocab))
			break
		}
		if seen[lexeme] {
			p.error(attrTok.Pos, msgDuplicateAttr(lexeme))
			break
		}
		seen[lexeme] = true
		p.advance() // ключевое слово атрибута
		p.expect(lexer.COLON, ":")
		switch lexeme {
		case "источник":
			src, _ := p.parseSourceRef()
			source = src
			attrs.SourcePos = toASTPos(attrTok.Pos)
		case "где":
			where = p.parseExpression()
			attrs.WherePos = toASTPos(attrTok.Pos)
		case "агрегат":
			aggregate = p.parseExpression()
			attrs.AggregatePos = toASTPos(attrTok.Pos)
		case "период":
			period = p.parsePeriodValue()
			attrs.PeriodPos = toASTPos(attrTok.Pos)
		case "по_дате":
			byDate = p.parseExpression()
			attrs.ByDatePos = toASTPos(attrTok.Pos)
		}
		p.expect(lexer.NEWLINE, "конец строки")
		if p.pos == before {
			p.advance() // backstop: гарантия прогресса
		}
	}
	p.expect(lexer.DEDENT, "конец блока")
	return ast.NewMetricDecl(toASTPos(mTok.Pos), *name, source, where, aggregate, period, byDate, attrs)
}

// parsePeriodValue разбирает значение атрибута период: (011-A2 §MW-4/§MW-D-PARSE).
// Контекстный спец-парс ТОЛЬКО внутри ветки период: — слова «последние»/«прошлый»/
// «прошлая» остаются обычными IDENT (НЕ ключевые слова; v1-программа может звать
// переменную с таким именем — §MW-D-PARSE-1), матч по лексеме как metricAttrName.
//
//   - «последние» <DURATION> → WindowPeriodLit{Amount,Unit} (контигуальная форма,
//     §MW-D-PARSE-2: спейсовая «30 дн» даёт INT, не DURATION → expect эмитит
//     SE-EXPECTED «ожидалось 'период вида N<ед>, например 30дн'»);
//   - «прошлый»/«прошлая» <IDENT> → LastCompletedPeriodLit{Noun} (маппинг noun→
//     адверб и валидация множества — в семантике/eval);
//   - иначе → parseExpression() (адверб-константа v1: ежемесячно… — путь не меняется,
//     §MW-D-PARSE-4).
func (p *Parser) parsePeriodValue() ast.Expression {
	tok := p.peek()
	if tok.Type == lexer.IDENT && tok.Lexeme == "последние" {
		p.advance() // последние
		durTok, ok := p.expect(lexer.DURATION, "период вида N<ед>, например 30дн")
		if !ok {
			return nil
		}
		dv, _ := durTok.Value.(lexer.DurationValue)
		return ast.NewWindowPeriodLit(toASTPos(tok.Pos), dv.Amount, dv.Unit)
	}
	if tok.Type == lexer.IDENT && (tok.Lexeme == "прошлый" || tok.Lexeme == "прошлая") {
		p.advance() // прошлый/прошлая
		nounTok, ok := p.expect(lexer.IDENT, "период: день/неделя/месяц/квартал/год")
		if !ok {
			return nil
		}
		return ast.NewLastCompletedPeriodLit(toASTPos(tok.Pos), nounTok.Lexeme)
	}
	return p.parseExpression()
}

// parseProcessDecl: процесс Ident ("(" ParamList? ")")? ":" NEWLINE INDENT
// StepDecl+ DEDENT (§PM-3). Параметры опциональны (без скобок → Params=nil).
// В блоке допустимы только шаги: не-шаг → SE-UNEXPECTED на ведущем токене
// строки, цикл ПРОДОЛЖАЕТСЯ (§PM-3 п.6) — последующие шаги собираются, прогресс
// гарантирует backstop; пустой блок → msgEmptyBlock и ProcessDecl с пустыми
// Steps. Pos() = токен процесс.
func (p *Parser) parseProcessDecl() *ast.ProcessDecl {
	procTok := p.advance() // процесс
	nameTok, _ := p.expect(lexer.IDENT, "имя процесса")
	name := p.identFrom(nameTok)
	var params []ast.Ident
	if p.check(lexer.LPAREN) {
		p.advance() // (
		params = p.parseParamList()
		p.expect(lexer.RPAREN, ")")
	}
	p.expect(lexer.COLON, ":")
	if !p.openAttrBlock() {
		return ast.NewProcessDecl(toASTPos(procTok.Pos), *name, params, nil)
	}
	var steps []*ast.StepDecl
	for !p.check(lexer.DEDENT) && !p.check(lexer.EOF) {
		p.suppress = false // граница строки блока процесса (доктрина recover.go)
		before := p.pos
		if p.check(lexer.KW_STEP) {
			if sd := p.parseStepDecl(); sd != nil {
				steps = append(steps, sd)
			}
		} else {
			// Не-шаг в блоке процесса → SE-UNEXPECTED. Потребляем токен ДО error()
			// (зеркально parse_expr.go default / parse_stmt.go:29): иначе ведущее
			// sync-lead ключевое слово (если/…) остаётся на курсоре, synchronize
			// стопорится на нём, и следующая итерация ре-диспетчит ТОТ ЖЕ токен →
			// каскад same-line (DX1, фича 012; контроль: не-sync-lead и так давал 1).
			bad := p.advance()
			p.error(bad.Pos, msgUnexpected(bad))
		}
		if p.pos == before {
			p.advance() // backstop: гарантия прогресса
		}
	}
	p.expect(lexer.DEDENT, "конец блока")
	return ast.NewProcessDecl(toASTPos(procTok.Pos), *name, params, steps)
}

// parseStepDecl: шаг Ident StepAfter? ":" NEWLINE INDENT StepLine+ DEDENT, где
// StepLine ::= StepAttr | Statement, StepAttr ::= ("исполнитель" | "срок") ":"
// Expression NEWLINE (§PM-3). Атрибуты и операторы чередуются свободно;
// «неизвестного атрибута» НЕТ (в отличие от metric) — любая не-исполнитель/срок
// строка разбирается как Statement. Повтор атрибута → msgDuplicateAttr + continue
// (D-8, пересмотр ревью №2: строка дубля съедена synchronize, цикл StepLine
// продолжается — break терял следующий шаг из AST); пустой блок → msgEmptyBlock.
// Pos() = токен шаг.
func (p *Parser) parseStepDecl() *ast.StepDecl {
	stepTok := p.advance() // шаг
	nameTok, _ := p.expect(lexer.IDENT, "имя шага")
	name := p.identFrom(nameTok)
	var after []ast.Ident
	if p.check(lexer.KW_AFTER) {
		p.advance() // после
		after = p.parseAfterList()
	}
	p.expect(lexer.COLON, ":")

	var assignee, deadline ast.Expression
	var attrs ast.StepAttrPos
	var body []ast.Statement

	if !p.openAttrBlock() {
		return ast.NewStepDecl(toASTPos(stepTok.Pos), *name, after, assignee, deadline, attrs, body)
	}
	seen := make(map[string]bool, 2)
	for !p.check(lexer.DEDENT) && !p.check(lexer.EOF) {
		p.suppress = false // граница StepLine (доктрина recover.go)
		before := p.pos
		if p.check(lexer.KW_ASSIGNEE) || p.check(lexer.KW_DEADLINE) {
			attrTok := p.peek()
			lexeme := attrTok.Lexeme
			if seen[lexeme] {
				p.error(attrTok.Pos, msgDuplicateAttr(lexeme))
				continue // D-8 ревью №2: исполнитель/срок не sync-lead → synchronize съел ≥1 токен, прогресс без backstop
			}
			p.advance() // ключевое слово атрибута
			p.expect(lexer.COLON, ":")
			if attrTok.Type == lexer.KW_ASSIGNEE {
				assignee = p.parseExpression()
				attrs.AssigneePos = toASTPos(attrTok.Pos)
			} else {
				deadline = p.parseExpression()
				attrs.DeadlinePos = toASTPos(attrTok.Pos)
			}
			p.expect(lexer.NEWLINE, "конец строки")
			seen[lexeme] = true
		} else if s := p.parseStatement(); s != nil {
			body = append(body, s)
		}
		if p.pos == before {
			p.advance() // backstop: гарантия прогресса
		}
	}
	p.expect(lexer.DEDENT, "конец блока")
	return ast.NewStepDecl(toASTPos(stepTok.Pos), *name, after, assignee, deadline, attrs, body)
}

// parseAfterList разбирает список предшественников после ключевого слова после:
// Ident ("," Ident)* БЕЗ скобок (отличие от parseParamList — терминатор не RPAREN,
// а отсутствие COMMA). Висящая запятая допускается best-effort (§PM-3 п.3, как
// parseParamList): после ',' не-IDENT → стоп без ошибки, собранный список
// остаётся. после без имени → SE-EXPECTED от первого expect, After пуст.
func (p *Parser) parseAfterList() []ast.Ident {
	var after []ast.Ident
	for {
		nameTok, ok := p.expect(lexer.IDENT, "имя шага")
		if !ok {
			break
		}
		after = append(after, *p.identFrom(nameTok))
		if !p.match(lexer.COMMA) {
			break
		}
		if !p.check(lexer.IDENT) {
			break // висящая запятая: best-effort, без ошибки
		}
	}
	return after
}

// parseTriggerDecl: когда TriggerSpec ":" Block (§TR-1, шов A). Вид формы — по
// второму токену (после когда): метрика/событие/расписание; иначе SE-TRIGGER-KIND.
// Тело — индентный Block (как функция/процесс), НЕ {}-скобки. Контекст-гарды
// (значение/событие, действия-шага) — целиком на семпроходе. Pos() = токен когда.
func (p *Parser) parseTriggerDecl() *ast.TriggerDecl {
	whenTok := p.advance() // когда
	var spec ast.TriggerSpec
	switch p.peek().Type {
	case lexer.KW_METRIC:
		spec = p.parseMetricTrigger()
	case lexer.KW_EVENT:
		spec = p.parseEventTrigger()
	case lexer.KW_SCHEDULE:
		spec = p.parseScheduleTrigger()
	default:
		p.error(p.peek().Pos, msgExpected(msgTriggerKind, p.peek()))
		return nil // поглощённая ошибочная конструкция (synchronize в error)
	}
	p.expect(lexer.COLON, ":")
	body := p.parseBlock()
	return ast.NewTriggerDecl(toASTPos(whenTok.Pos), spec, body)
}

// parseMetricTrigger: метрика Ident CompOp Expression (§TR-1). Условие — РОВНО
// одно сравнение (FR-021): разбор плоский (имя → оператор → правая часть), порог
// читается parseComparison (НЕ parseExpression сверху), поэтому логический и/или
// НЕ поглощается — `метрика X < Y и Z`: и остаётся, далее ошибка ожидания ':'.
// Pos() спеки = токен метрика.
func (p *Parser) parseMetricTrigger() ast.TriggerSpec {
	metTok := p.advance() // метрика
	nameTok, _ := p.expect(lexer.IDENT, "имя метрики")
	op := p.expectCompOp()
	threshold := p.parseComparison()
	return ast.NewMetricTrigger(toASTPos(metTok.Pos), *p.identFrom(nameTok), op, threshold)
}

// expectCompOp потребляет ровно один токен сравнения (== != < <= > >=) через
// существующий compOpOf; иначе SE-EXPECT-COMPOP и возвращает нулевой CompOp без
// сдвига курсора (синхронизация — на вышестоящем уровне).
func (p *Parser) expectCompOp() ast.CompOp {
	binop, ok := compOpOf(p.peek().Type)
	if !ok {
		p.error(p.peek().Pos, msgExpected(msgCompOp, p.peek()))
		return ast.CompOp(0)
	}
	p.advance()
	return ast.CompOp(binop)
}

// parseEventTrigger: событие Ident (§TR-1). Pos() спеки = токен событие.
func (p *Parser) parseEventTrigger() ast.TriggerSpec {
	evtTok := p.advance() // событие
	nameTok, _ := p.expect(lexer.IDENT, "имя события")
	return ast.NewEventTrigger(toASTPos(evtTok.Pos), *p.identFrom(nameTok))
}

// parseScheduleTrigger: расписание ScheduleSpec (§TR-1). Pos() спеки = токен
// расписание.
func (p *Parser) parseScheduleTrigger() ast.TriggerSpec {
	schTok := p.advance() // расписание
	spec := p.parseScheduleSpec()
	if spec == nil {
		return nil
	}
	return ast.NewScheduleTrigger(toASTPos(schTok.Pos), spec)
}

// parseScheduleSpec: каждые DurationLiteral | в StringLiteral (§TR-1). Все 6
// единиц каждые принимаются без ограничений; содержимое строки в "…" парсером НЕ
// валидируется (формат ЧЧ:ММ — граница 007b). Иначе SE-SCHEDULE-SPEC.
func (p *Parser) parseScheduleSpec() ast.ScheduleSpec {
	switch p.peek().Type {
	case lexer.KW_EVERY:
		everyTok := p.advance() // каждые
		durTok, _ := p.expect(lexer.DURATION, "длительность")
		return ast.NewEverySchedule(toASTPos(everyTok.Pos), p.buildDurationLit(durTok))
	case lexer.KW_IN:
		inTok := p.advance() // в
		strTok, _ := p.expect(lexer.STRING, "время в кавычках")
		return ast.NewAtSchedule(toASTPos(inTok.Pos), *p.buildStringLit(strTok))
	default:
		p.error(p.peek().Pos, msgExpected(msgScheduleSpec, p.peek()))
		return nil
	}
}

// openAttrBlock потребляет NEWLINE и открывающий INDENT блока атрибутов; при
// пустом блоке (нет INDENT) эмитит msgEmptyBlock и возвращает false (как parseBlock).
func (p *Parser) openAttrBlock() bool {
	p.expect(lexer.NEWLINE, "конец строки")
	if !p.check(lexer.INDENT) {
		p.errorLocal(p.peek().Pos, msgEmptyBlock)
		return false
	}
	p.advance() // INDENT
	return true
}
