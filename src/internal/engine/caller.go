package engine

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/jsonval"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// ExternalCaller — драйвер внешних эффектов «вызвать»/«уведомить» (B2, §AU-4.1).
// Инстанс-поле движка (DI, Принцип V); НЕ часть шва eval.ProcessRuntime — eval о нём
// не знает, движок делегирует ему свои методы CallExternal*/Notify. Две реализации:
// дефолтная печать-стаб printCaller (§EN-7) и реальная HTTP-доставка webhookCaller.
type ExternalCaller interface {
	Call(target string, args []value.Value) (value.Value, error) // вызвать → результат
	Notify(target string, args []value.Value) error              // уведомить → эффект
}

// printCaller — дефолтный драйвер (§AU-4.2 / §EN-7): печать одной строки в out, без
// сети. Перенос печать-логики из runtime.go (B1) байт-в-байт. Call → (None, nil);
// Notify → печать + nil (best-effort). НИКОГДА не возвращает ошибку (стаб v1 цел).
type printCaller struct {
	out io.Writer
}

// Call печатает «[вызов] <имя>(<арг1, арг2, …>)» (разделитель ", "; без аргументов
// «[вызов] <имя>()») и возвращает (value.None, nil): захват результата под стабом → Пусто.
func (p printCaller) Call(target string, args []value.Value) (value.Value, error) {
	parts := make([]string, len(args))
	for k, a := range args {
		parts[k] = value.String(a)
	}
	fmt.Fprintf(p.out, "[вызов] %s(%s)\n", target, strings.Join(parts, ", "))
	return value.None, nil
}

// Notify печатает «[уведомление] <получатель>: <арг1 арг2 …>» (разделитель " ") при
// ≥1 аргументе, иначе «[уведомление] <получатель>» (без двоеточия и хвостовых
// пробелов). Всегда nil (best-effort).
func (p printCaller) Notify(target string, args []value.Value) error {
	if len(args) == 0 {
		fmt.Fprintf(p.out, "[уведомление] %s\n", target)
		return nil
	}
	parts := make([]string, len(args))
	for k, a := range args {
		parts[k] = value.String(a)
	}
	fmt.Fprintf(p.out, "[уведомление] %s: %s\n", target, strings.Join(parts, " "))
	return nil
}

// webhookCaller — реальный драйвер (§AU-4.3): POST plain-JSON {"цель","данные"} на
// baseURL через httpClient. Единственный потребитель сети в движке; изолирован за
// ExternalCaller. Реальный HTTP в тестах — только под net/http/httptest.
type webhookCaller struct {
	baseURL    string
	httpClient *http.Client
}

// Call делает POST тела вебхука и декодирует ответ (§AU-4.3): тело —
// jsonval.EncodeBody (нетегированный plain-JSON); сетевой/HTTP-сбой → error (несётся
// наверх, оборачивается в ОшибкаВыполнения на eval-точке). ПУСТОЕ тело ответа → Пусто
// (проверка ДО декода: декодер на пустом потоке вернул бы ошибку); иначе DecodeValue.
func (w webhookCaller) Call(target string, args []value.Value) (value.Value, error) {
	resp, err := w.post(target, args)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение ответа вебхука: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return value.None, nil // пустое тело → Пусто (§AU-4.3)
	}
	v, err := jsonval.DecodeValue(jsonval.NewDecoder(bytes.NewReader(body)))
	if err != nil {
		return nil, fmt.Errorf("декод ответа вебхука: %w", err)
	}
	return v, nil
}

// Notify делает POST тела вебхука best-effort: сетевой/HTTP-сбой → error; ответ
// игнорируется (тело закрывается, не декодируется).
func (w webhookCaller) Notify(target string, args []value.Value) error {
	resp, err := w.post(target, args)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// post строит и отправляет POST {"цель","данные"} на baseURL; общий помощник
// Call/Notify. HTTP-статус ≥400 → ошибка (сбой доставки виден eval-точке как
// ОшибкаВыполнения).
func (w webhookCaller) post(target string, args []value.Value) (*http.Response, error) {
	body := jsonval.EncodeBody(target, args)
	resp, err := w.httpClient.Post(w.baseURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("POST на вебхук %s: %w", w.baseURL, err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("вебхук %s вернул статус %d", w.baseURL, resp.StatusCode)
	}
	return resp, nil
}
