# Контракт: общий хелпер минта `enqueueEvent` (рефактор D-IE-8)

Источник: якорь §IE-4.2, D-IE-8.

## Сигнатура
```go
func enqueueEvent(st store.Store, name, payload string, clock engine.Clock) (string, error)
```

## Тело (вынесено из `emitEvent`, emit.go:66-81)
```go
id, err := st.NextEventID()
if err != nil {
    return "", err
}
e := &store.Event{
    ID:          id,
    Name:        name,
    PayloadJSON: payload,
    CreatedAt:   clock.Now(),
    Processed:   false,
}
if err := st.EnqueueEvent(e); err != nil {
    return "", err
}
return id, nil
```

## Контракт
- Принимает `store.Store` (интерфейс) — работает и над SQLite (`emit`), и над Store демона (HTTP).
- **Ack-печать НЕ входит** в хелпер. Вызыватели печатают сами, тексты различны намеренно:
  - `emitEvent` → `событие %s '%s' поставлено в очередь\n` (emit.go:82, **без изменений**).
  - `eventsHandler` → `событие %s '%s' принято\n`.
- Единый путь минта ⇒ идентичная семантика `ID`/`CreatedAt` (FR-IE-3 неразличимость).

## Регресс-инвариант `emit`
- После рефактора поведение `emit` байт-идентично: оба прежних сбоя (`NextEventID`, `EnqueueEvent`) печатали один текст `ladix: сбой хранилища: <err>` и давали exit 2. Единый `err`-путь в `emitEvent` сохраняет это (`fmt.Fprintf(stderr, "ladix: сбой хранилища: %s\n", err)`; `return 2`).
- Существующие emit-замки (golden ack, emit-тесты) остаются зелёными.
