package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// eventsHandler — HTTP-приёмник событий (трек B, §IE-2/§IE-4/§IE-6). Тонкий сетевой
// продюсер в durable-очередь events: касается ТОЛЬКО store.Store + engine.Clock,
// НИКОГДА Engine/Interpreter (FR-IE-2 — они не потокобезопасны, а listener живёт во
// второй горутине; сериализация исполнения держится на d.mu + единственной горутине
// движка). Минт через общий enqueueEvent → семантика ID/CreatedAt идентична emit
// (FR-IE-3, неразличимость источников). token=="" → auth выключен: любой POST → 202
// (FR-IE-9, дефолт). Сигнатура (store.Store, engine.Clock, string) — статический замок
// изоляции от движка.
func eventsHandler(st store.Store, clock engine.Clock, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Метод: только POST (FR-IE-10). Свой дословный RU-текст (Принцип VIII), не
		// дефолтное тело ServeMux. Заголовок Allow (RFC 7231 §6.5.5 SHOULD) — полировка
		// сверх канона §IE-2; ставится СТРОГО до WriteHeader (после него игнорируется).
		// Тело/код 405 БАЙТ-идентичны (golden не сдвигается).
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintln(w, "ladix: метод не поддерживается, только POST")
			return
		}
		// 2. Auth (§IE-6, FR-IE-9): constant-time сравнение X-Ladix-Token. Только если
		// токен задан; иначе пропускаем (дефолт — выключено).
		if token != "" {
			got := r.Header.Get("X-Ladix-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintln(w, "ladix: неверный токен")
				return
			}
		}
		// 3. Имя = сегмент после "/events/" (r.URL.Path уже percent-декодирован net/http,
		// один проход; матч строго байтовый, без NFC — §IE-2). Пусто/с разделителем → 400.
		name := strings.TrimPrefix(r.URL.Path, "/events/")
		if name == "" || strings.Contains(name, "/") {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, "ladix: пустое имя события")
			return
		}
		// 4. Тело — сырой текст, БЕЗ парсинга на приёме (битый JSON не роняет приём,
		// FR-IE-7; валидацию делает drainEvents на тике).
		body, _ := io.ReadAll(r.Body)
		// 5. Минт через общий хелпер. 202 строго ПОСЛЕ успешного EnqueueEvent (FR-IE-6):
		// сбой Store → 500, событие не теряется молча.
		id, err := enqueueEvent(st, name, string(body), clock)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "ladix: сбой хранилища")
			return
		}
		w.WriteHeader(http.StatusAccepted)
		// Ack «принято» — НАМЕРЕННО отличается от ack emit «поставлено в очередь» (D-IE-8).
		fmt.Fprintf(w, "событие %s '%s' принято\n", id, name)
	})
}

// startEventListener поднимает HTTP-приёмник на уже открытом ln в отдельной горутине
// (§IE-5, stdlib-only: sync.WaitGroup + srv.Shutdown, БЕЗ errgroup — симметрия с
// ticker+Stop демона). Возвращает stop: грациозно гасит сервер (Shutdown — перестать
// принимать → дослить in-flight) и join-ит горутину (wg.Wait). Вызыватель обязан
// зарегистрировать defer stop() ВНУТРИ guard-замыкания serveFile, чтобы по LIFO он
// отработал ДО внешнего defer sq.Close() — иначе in-flight POST писал бы в закрытый
// Store (потеря события, нарушение at-least-once, FR-IE-6). Join гарантирует отсутствие
// утечки горутины (FR-IE-8).
func startEventListener(ln net.Listener, st store.Store, clock engine.Clock, token string) (stop func()) {
	srv := &http.Server{Handler: eventsHandler(st, clock, token)}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(ln) // при Shutdown вернёт http.ErrServerClosed — штатно
	}()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		wg.Wait()
	}
}
