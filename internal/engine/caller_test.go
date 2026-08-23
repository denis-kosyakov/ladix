package engine

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	lerrors "github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// --- C-CALLER-1 (T008): контракт интерфейса/реализаций ---

// Компиляц.-замки: обе реализации удовлетворяют ExternalCaller; шов ProcessRuntime цел.
var (
	_ ExternalCaller      = printCaller{}
	_ ExternalCaller      = webhookCaller{}
	_ eval.ProcessRuntime = (*Engine)(nil)
)

// --- C-EN7-1 (T009): форматы стаба байт-точно ---

// TestPrintCallerFormats — printCaller печатает §EN-7 байт-в-байт и не ошибается.
func TestPrintCallerFormats(t *testing.T) {
	// Call с аргументами: "[вызов] crm(клиент, 5)\n", (None, nil).
	var out bytes.Buffer
	pc := printCaller{out: &out}
	v, err := pc.Call("crm", []value.Value{value.Строка{V: "клиент"}, value.Целое{V: 5}})
	if err != nil {
		t.Fatalf("Call: неожиданная ошибка %v", err)
	}
	if _, ok := v.(value.Пусто); !ok {
		t.Errorf("Call вернул %#v, хотим value.None", v)
	}
	if got := out.String(); got != "[вызов] crm(клиент, 5)\n" {
		t.Errorf("Call формат = %q, хотим %q", got, "[вызов] crm(клиент, 5)\n")
	}

	// Notify с аргументом: "[уведомление] ИТ: x\n".
	out.Reset()
	if err := pc.Notify("ИТ", []value.Value{value.Строка{V: "x"}}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := out.String(); got != "[уведомление] ИТ: x\n" {
		t.Errorf("Notify формат = %q, хотим %q", got, "[уведомление] ИТ: x\n")
	}

	// Notify без аргументов: "[уведомление] дежурный\n" (без двоеточия/хвоста).
	out.Reset()
	if err := pc.Notify("дежурный", nil); err != nil {
		t.Fatalf("Notify(пусто): %v", err)
	}
	if got := out.String(); got != "[уведомление] дежурный\n" {
		t.Errorf("Notify(пусто) формат = %q, хотим %q", got, "[уведомление] дежурный\n")
	}

	// Call без аргументов: "[вызов] пинг()\n".
	out.Reset()
	if _, err := pc.Call("пинг", nil); err != nil {
		t.Fatalf("Call(пусто): %v", err)
	}
	if got := out.String(); got != "[вызов] пинг()\n" {
		t.Errorf("Call(пусто) формат = %q, хотим %q", got, "[вызов] пинг()\n")
	}
}

// --- C-OPT-1 (T010): дефолт + Option ---

// fakeCaller фиксирует вызовы для проверки делегирования (НЕ печатает, НЕ сетует).
type fakeCaller struct {
	calls    int
	notifies int
	lastTgt  string
	ret      value.Value
}

func (f *fakeCaller) Call(target string, args []value.Value) (value.Value, error) {
	f.calls++
	f.lastTgt = target
	if f.ret != nil {
		return f.ret, nil
	}
	return value.None, nil
}

func (f *fakeCaller) Notify(target string, args []value.Value) error {
	f.notifies++
	f.lastTgt = target
	return nil
}

// TestDefaultCallerIsPrintCaller — NewEngine БЕЗ WithExternalCaller → дефолт printCaller:
// методы движка печатают стаб в out (сеть не трогается).
func TestDefaultCallerIsPrintCaller(t *testing.T) {
	var out bytes.Buffer
	e := &Engine{out: &out}
	// Симуляция дефолта NewEngine: printCaller ставится ДО opts. Проверяем тип.
	e.caller = printCaller{out: e.out}
	if _, ok := e.caller.(printCaller); !ok {
		t.Fatalf("дефолт caller = %T, хотим printCaller", e.caller)
	}
	if _, err := e.CallExternalResult("crm", nil); err != nil {
		t.Fatalf("CallExternalResult: %v", err)
	}
	if got := out.String(); got != "[вызов] crm()\n" {
		t.Errorf("дефолт-стаб печать = %q, хотим [вызов] crm()", got)
	}
}

// TestDefaultCallerViaNewEngine — полный путь NewEngine ставит printCaller{out}.
func TestDefaultCallerViaNewEngine(t *testing.T) {
	var out bytes.Buffer
	e := NewEngine(nil, nil, &out)
	pc, ok := e.caller.(printCaller)
	if !ok {
		t.Fatalf("NewEngine дефолт caller = %T, хотим printCaller", e.caller)
	}
	if pc.out != &out {
		t.Errorf("printCaller.out не равен e.out")
	}
}

// TestWithExternalCallerOverrides — WithExternalCaller(fake) → методы движка зовут fake,
// стаб НЕ печатается.
func TestWithExternalCallerOverrides(t *testing.T) {
	var out bytes.Buffer
	f := &fakeCaller{}
	e := NewEngine(nil, nil, &out, WithExternalCaller(f))
	if e.caller != f {
		t.Fatalf("caller = %#v, хотим fake", e.caller)
	}
	if _, err := e.CallExternalResult("crm", nil); err != nil {
		t.Fatalf("CallExternalResult: %v", err)
	}
	if f.calls != 1 {
		t.Errorf("fake.Call вызван %d раз, хотим 1", f.calls)
	}
	if out.Len() != 0 {
		t.Errorf("стаб печатал под fake-драйвером: %q", out.String())
	}
}

// --- C-SEAM-DELEG (T011): делегирование без двойного эффекта ---

// TestCallExternalDelegatesNoDouble — CallExternal под стабом печатает РОВНО одну строку.
func TestCallExternalDelegatesNoDouble(t *testing.T) {
	var out bytes.Buffer
	e := NewEngine(nil, nil, &out)
	if err := e.CallExternal("crm", []value.Value{value.Целое{V: 1}}); err != nil {
		t.Fatalf("CallExternal: %v", err)
	}
	if got := out.String(); got != "[вызов] crm(1)\n" {
		t.Errorf("CallExternal печать = %q, хотим одну строку [вызов] crm(1)", got)
	}
}

// TestCallExternalDelegatesSingleEffect — под fake CallExternal зовёт Call ровно один раз.
func TestCallExternalDelegatesSingleEffect(t *testing.T) {
	var out bytes.Buffer
	f := &fakeCaller{}
	e := NewEngine(nil, nil, &out, WithExternalCaller(f))
	if err := e.CallExternal("crm", nil); err != nil {
		t.Fatalf("CallExternal: %v", err)
	}
	if f.calls != 1 {
		t.Errorf("CallExternal → fake.Call вызван %d раз, хотим 1 (нет двойного эффекта)", f.calls)
	}
}

// TestCallExternalResultReturnsNone — CallExternalResult под стабом → (None, nil).
func TestCallExternalResultReturnsNone(t *testing.T) {
	var out bytes.Buffer
	e := NewEngine(nil, nil, &out)
	v, err := e.CallExternalResult("crm", nil)
	if err != nil {
		t.Fatalf("CallExternalResult: %v", err)
	}
	if _, ok := v.(value.Пусто); !ok {
		t.Errorf("CallExternalResult вернул %#v, хотим value.None", v)
	}
}

// --- C-ERR-3 (T024): стаб никогда не ошибается ---

// TestPrintCallerNeverErrors — printCaller.Call/.Notify всегда nil-ошибка.
func TestPrintCallerNeverErrors(t *testing.T) {
	var out bytes.Buffer
	pc := printCaller{out: &out}
	if _, err := pc.Call("x", nil); err != nil {
		t.Errorf("printCaller.Call вернул ошибку %v, хотим nil", err)
	}
	if err := pc.Notify("x", nil); err != nil {
		t.Errorf("printCaller.Notify вернул ошибку %v, хотим nil", err)
	}
}

// --- C-WIRE-1 (T017): POST + тело ---

// capturedReq фиксирует то, что получил httptest-сервер.
type capturedReq struct {
	method      string
	contentType string
	body        string
}

// webhookFixture поднимает httptest-сервер, возвращающий respBody, и драйвер на него.
func webhookFixture(t *testing.T, respBody string, statusCode int) (*webhookCaller, *capturedReq, func()) {
	t.Helper()
	cap := &capturedReq{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.contentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		cap.body = string(b)
		if statusCode != 0 {
			w.WriteHeader(statusCode)
		}
		io.WriteString(w, respBody)
	}))
	wc := &webhookCaller{baseURL: srv.URL, httpClient: srv.Client()}
	return wc, cap, srv.Close
}

// TestWebhookCallerCallPostsBody — Call → POST application/json тело {"цель","данные"}.
func TestWebhookCallerCallPostsBody(t *testing.T) {
	wc, cap, done := webhookFixture(t, "", 0)
	defer done()
	if _, err := wc.Call("crm", []value.Value{value.Строка{V: "клиент"}}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if cap.method != http.MethodPost {
		t.Errorf("метод = %q, хотим POST", cap.method)
	}
	if cap.contentType != "application/json" {
		t.Errorf("Content-Type = %q, хотим application/json", cap.contentType)
	}
	if want := `{"цель":"crm","данные":["клиент"]}`; cap.body != want {
		t.Errorf("тело = %q, хотим %q", cap.body, want)
	}
}

// TestWebhookCallerNotifyPostsBody — Notify → POST тело {"цель","данные"}, ответ игнор.
func TestWebhookCallerNotifyPostsBody(t *testing.T) {
	wc, cap, done := webhookFixture(t, `{"любой":"ответ"}`, 0)
	defer done()
	if err := wc.Notify("ИТ", []value.Value{value.Строка{V: "x"}}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if cap.method != http.MethodPost {
		t.Errorf("метод = %q, хотим POST", cap.method)
	}
	if want := `{"цель":"ИТ","данные":["x"]}`; cap.body != want {
		t.Errorf("тело = %q, хотим %q", cap.body, want)
	}
}

// --- C-WIRE-2 (T018): декод ответа ---

// TestWebhookCallerDecodesObject — ответ-объект → Запись.
func TestWebhookCallerDecodesObject(t *testing.T) {
	wc, _, done := webhookFixture(t, `{"статус":"ок"}`, 0)
	defer done()
	v, err := wc.Call("crm", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rec, ok := v.(value.Запись)
	if !ok {
		t.Fatalf("ответ = %#v, хотим value.Запись", v)
	}
	if got, ok := rec.Get("статус").(value.Строка); !ok || got.V != "ок" {
		t.Errorf("статус = %#v, хотим Строка{ок}", rec.Get("статус"))
	}
}

// TestWebhookCallerEmptyBodyIsNone — пустое тело ответа → Пусто (guard ДО декода).
func TestWebhookCallerEmptyBodyIsNone(t *testing.T) {
	wc, _, done := webhookFixture(t, "", 0)
	defer done()
	v, err := wc.Call("crm", nil)
	if err != nil {
		t.Fatalf("Call: %v (пустое тело должно дать Пусто, не ошибку)", err)
	}
	if _, ok := v.(value.Пусто); !ok {
		t.Errorf("пустое тело → %#v, хотим value.None", v)
	}
}

// TestWebhookCallerDecodesScalar — скалярный ответ (не объект) → Value (DecodeValue,
// НЕ PayloadToRecord, который потребовал бы верхний объект).
func TestWebhookCallerDecodesScalar(t *testing.T) {
	wc, _, done := webhookFixture(t, `"готово"`, 0)
	defer done()
	v, err := wc.Call("crm", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if s, ok := v.(value.Строка); !ok || s.V != "готово" {
		t.Errorf("скалярный ответ = %#v, хотим Строка{готово}", v)
	}
}

// --- C-WIRE-3 (T019): типы тела (plain-JSON, не тегированный) ---

// TestWebhookBodyArgTypes — аргументы всех типов сериализуются в plain-JSON в данные[].
func TestWebhookBodyArgTypes(t *testing.T) {
	wc, cap, done := webhookFixture(t, "", 0)
	defer done()
	rec := value.NewRecord([]string{"k"}, map[string]value.Value{"k": value.Целое{V: 1}})
	args := []value.Value{
		value.Целое{V: 7},
		value.Дробное{V: 2.5},
		value.Строка{V: "с"},
		value.Булево{V: true},
		value.None,
		value.NewList([]value.Value{value.Целое{V: 9}}),
		rec,
	}
	if _, err := wc.Call("t", args); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := `{"цель":"t","данные":[7,2.5,"с",true,null,[9],{"k":1}]}`
	if cap.body != want {
		t.Errorf("тело = %q,\nхотим %q (plain-JSON, БЕЗ обёртки т/зн)", cap.body, want)
	}
}

// --- C-ERR-2 (T023): сбой реального драйвера → непустая ошибка ---

// TestWebhookCallerCallErrorPropagates — сервер 5xx → Call/Notify возвращают ошибку.
func TestWebhookCallerCallErrorPropagates(t *testing.T) {
	wc, _, done := webhookFixture(t, "сбой", http.StatusInternalServerError)
	defer done()
	if _, err := wc.Call("crm", nil); err == nil {
		t.Errorf("Call на 5xx: хотим ошибку, получили nil")
	}
	if err := wc.Notify("crm", nil); err == nil {
		t.Errorf("Notify на 5xx: хотим ошибку, получили nil")
	}
}

// TestWebhookCallerNetworkErrorPropagates — неотвечающий адрес → ошибка (сеть мертва).
func TestWebhookCallerNetworkErrorPropagates(t *testing.T) {
	// Закрытый сервер: клиент получит connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	client := srv.Client()
	srv.Close() // адрес больше не слушает
	wc := &webhookCaller{baseURL: url, httpClient: client}
	if _, err := wc.Call("crm", nil); err == nil {
		t.Errorf("Call на мёртвый адрес: хотим ошибку, получили nil")
	}
}

// TestWebhookErrorReachesWrapViaEngine — интеграционно: сбой драйвера через движок
// доходит до eval-обёртки как ОшибкаВыполнения (связка C-ERR-1↔C-ERR-2). Прямой вызов
// CallExternalResult движка под сбойным webhookCaller возвращает ошибку, которую
// eval-точка обернёт в ОшибкаВыполнения с Cause (см. eval/stmt_test).
func TestWebhookErrorReachesWrapViaEngine(t *testing.T) {
	wc, _, done := webhookFixture(t, "", http.StatusBadGateway)
	defer done()
	e := NewEngine(nil, nil, io.Discard, WithExternalCaller(wc))
	_, err := e.CallExternalResult("crm", nil)
	if err == nil {
		t.Fatalf("движок под сбойным вебхуком: хотим ошибку, получили nil")
	}
	// Симулируем обёртку eval-точки (runtimeErrWrap): ОшибкаВыполнения с Cause.
	wrapped := lerrors.ОшибкаВыполнения{Msg: err.Error(), Cause: err}
	var re lerrors.ОшибкаВыполнения
	if !errors.As(error(wrapped), &re) {
		t.Errorf("обёртка не распознана как ОшибкаВыполнения")
	}
	if re.Cause == nil {
		t.Errorf("Cause пуст — цепочка причины потеряна")
	}
}
