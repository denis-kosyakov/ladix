package ir

// SchemaVersion — версия формата IR. Растёт при BREAKING-изменении формы:
// удаление/переименование поля, смена типа поля, смена семантики существующего
// поля, смена формата канонических строк выражений. АДДИТИВНЫЕ изменения
// (новое опциональное поле, новое значение Severity/Stage) версию НЕ меняют —
// потребитель обязан их толерантно игнорировать (см. doc.go).
const SchemaVersion = 1

// Словарь Severity. В v1 фронтенд эмитит ТОЛЬКО SeverityError: warning
// специфицирован SPEC §13, но фронтендом не производится (отложен).
// Константы — именованные строки, а НЕ enum-тип: потребитель обязан принимать
// неизвестное значение без падения (forward-compat).
const (
	SeverityError = "error"
)

// Словарь Stage — этап статической компиляции, на котором возникла проблема.
// Ошибки «Типа»/«Выполнения» (SPEC §13.1) сюда не попадают: типизация в LADIX
// динамическая (SPEC §4), эти категории производятся исполнением, а не Compile.
const (
	StageLexical  = "lexical"
	StageSyntax   = "syntax"
	StageSemantic = "semantic"
)

// Position — позиция в исходном тексте. Отсчёт с 1, колонки считаются в рунах
// (Unicode code points), не в байтах — язык кириллический.
//
// Собственный локальный тип пакета (дубль ast.Position / errors.Position), а не
// разделяемый: это сохраняет ir листовым — он не импортирует ни ast, ни errors.
type Position struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// Program — корень IR: результат успешной компиляции исходника LADIX.
// Коллекции идут в порядке объявления в исходнике.
type Program struct {
	SchemaVersion int       `json:"schema_version"`
	Metrics       []Metric  `json:"metrics"`
	Processes     []Process `json:"processes"`
	Triggers      []Trigger `json:"triggers"`
}

// Metric — нормализованное определение метрики. Выражения (Where/Aggregate/
// Period/ByDate) представлены КАНОНИЧЕСКИМИ СТРОКАМИ: текстонезависимой записью
// смысла выражения. Отсутствующий необязательный атрибут — пустая строка.
type Metric struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	Where     string   `json:"where"`
	Aggregate string   `json:"aggregate"`
	Period    string   `json:"period"`
	ByDate    string   `json:"by_date"`
	Pos       Position `json:"pos"`
}

// Process — нормализованное определение бизнес-процесса.
type Process struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
	Steps  []Step   `json:"steps"`
	Pos    Position `json:"pos"`
}

// Step — шаг процесса. After — имена шагов-предшественников через ", " (пусто,
// если шаг стартовый). Actions — операторы тела шага каноническими строками,
// в порядке исходника.
type Step struct {
	Name     string   `json:"name"`
	After    string   `json:"after"`
	Assignee string   `json:"assignee"`
	Deadline string   `json:"deadline"`
	Actions  []string `json:"actions"`
	Pos      Position `json:"pos"`
}

// Словарь Trigger.Kind — вид триггера. Поля, неприменимые к данному виду,
// остаются пустыми (например Event/Schedule у KindMetric).
const (
	KindMetric   = "metric"
	KindSchedule = "schedule"
	KindEvent    = "event"
	KindDeadline = "deadline"
)

// Trigger — нормализованное определение триггера. Дискриминант — Kind.
// Threshold и Schedule — канонические строки.
type Trigger struct {
	Kind      string   `json:"kind"`
	Metric    string   `json:"metric"`
	Op        string   `json:"op"`
	Threshold string   `json:"threshold"`
	Event     string   `json:"event"`
	Schedule  string   `json:"schedule"`
	Process   string   `json:"process"`
	Step      string   `json:"step"`
	Pos       Position `json:"pos"`
}

// Diagnostic — одна проблема компиляции. Message — ДОСЛОВНЫЙ русский текст
// диагностики из SPEC §13 (описание без двухстрочного заголовка: позиция едет
// отдельным полем Pos). Переформулирование запрещено.
type Diagnostic struct {
	Severity string   `json:"severity"`
	Stage    string   `json:"stage"`
	Message  string   `json:"message"`
	Pos      Position `json:"pos"`
}
