package engine

import "time"

// Clock — время движка (D-2, §EN-3). НЕ путать с eval.Clock (дневной, value.Дата).
// Используется для CreatedAt/UpdatedAt/CompletedAt, абсолютизации дедлайна, «просрочена».
type Clock interface {
	Now() time.Time
}

// SystemClock — продовый Clock; ЕДИНСТВЕННОЕ легальное time.Now() движка (D-2).
// Экспортируется: его дёргает CLI (tasks берёт now из SystemClock{}, инвариант D-22).
type SystemClock struct{}

// Now возвращает текущее время — единственный вызов time.Now() в пакете engine.
func (SystemClock) Now() time.Time { return time.Now() }

// Option — функциональная опция конструктора NewEngine.
type Option func(*Engine)

// WithClock подменяет часы (тесты/golden: фиксированный момент).
func WithClock(c Clock) Option {
	return func(e *Engine) { e.clock = c }
}

// WithExternalCaller подменяет драйвер внешних эффектов «вызвать»/«уведомить»
// (B2, §AU-4.1): прод — webhookCaller (CLI под --вебхук/LADIX_WEBHOOK), тесты —
// фейк/httptest. Применяется в NewEngine ПОСЛЕ дефолта printCaller.
func WithExternalCaller(c ExternalCaller) Option {
	return func(e *Engine) { e.caller = c }
}
