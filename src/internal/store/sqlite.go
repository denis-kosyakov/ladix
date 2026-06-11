package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore — персистентная реализация Store поверх modernc.org/sqlite (чистый
// Go без CGO, FR-005, §EN-2). Variables кодируются type-tagged JSON (codec.go),
// времена — RFC3339 (секундная точность). Mint id — персистентный счётчик в
// таблице counters (D-10).
type SQLiteStore struct {
	db *sql.DB
}

// ddl — схема БД (§EN-2). Исполняется при открытии (CREATE TABLE IF NOT EXISTS +
// сид counters через INSERT OR IGNORE — без сида UPDATE минта на свежей БД
// обновил бы 0 строк).
const ddl = `
CREATE TABLE IF NOT EXISTS instances (
    id           TEXT PRIMARY KEY,
    process_name TEXT NOT NULL,
    status       TEXT NOT NULL,
    current_step TEXT NOT NULL,
    variables    TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks (
    id           TEXT PRIMARY KEY,
    instance_id  TEXT NOT NULL REFERENCES instances(id),
    step_name    TEXT NOT NULL,
    assignee     TEXT NOT NULL,
    deadline     TEXT,
    status       TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    completed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_tasks_pending ON tasks(assignee, status);
CREATE TABLE IF NOT EXISTS counters (
    name  TEXT PRIMARY KEY,
    value INTEGER NOT NULL
);
INSERT OR IGNORE INTO counters(name, value) VALUES ('instance', 0), ('task', 0);
`

// pragmas исполняются при открытии (EM-7).
const pragmas = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
`

// NewSQLiteStore открывает БД и ЯВНО исполняет PRAGMA + DDL (включая сид counters;
// database/sql ленив — без явного Exec ошибка открытия не всплыла бы). Первая
// ошибка возвращается наружу — это источник CLI-текста «не удалось открыть
// хранилище» (§EN-8.B).
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Один процесс — одно соединение: сериализует весь доступ к файлу, делает
	// PRAGMA busy_timeout «липкими» (иначе пул database/sql применил бы их лишь
	// к одному соединению, а прочие падали бы на SQLITE_BUSY) и гарантирует
	// атомарность mint счётчика (D-10) под конкурентной нагрузкой. Ни один путь
	// кода не удерживает два соединения одновременно — дедлока нет.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(pragmas); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(ddl); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

// Close — метод конкретного типа (НЕ интерфейса Store); CLI делает defer Close()
// после успешного открытия.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) SaveInstance(inst *ProcessInstance) error {
	vars, err := encodeVariables(inst.Variables)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO instances (id, process_name, status, current_step, variables, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   process_name = excluded.process_name,
		   status       = excluded.status,
		   current_step = excluded.current_step,
		   variables    = excluded.variables,
		   created_at   = excluded.created_at,
		   updated_at   = excluded.updated_at`,
		inst.ID, inst.ProcessName, string(inst.Status), inst.CurrentStep, vars,
		inst.CreatedAt.Format(time.RFC3339), inst.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *SQLiteStore) LoadInstance(id string) (*ProcessInstance, error) {
	row := s.db.QueryRow(
		`SELECT process_name, status, current_step, variables, created_at, updated_at
		 FROM instances WHERE id = ?`, id)
	var processName, status, currentStep, vars, createdAt, updatedAt string
	if err := row.Scan(&processName, &status, &currentStep, &vars, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInstanceNotFound
		}
		return nil, err
	}
	variables, err := decodeVariables(vars)
	if err != nil {
		return nil, err
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	updated, err := parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	return &ProcessInstance{
		ID:          id,
		ProcessName: processName,
		Status:      Status(status),
		CurrentStep: currentStep,
		Variables:   variables,
		CreatedAt:   created,
		UpdatedAt:   updated,
	}, nil
}

func (s *SQLiteStore) SaveTask(t *Task) error {
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, instance_id, step_name, assignee, deadline, status, created_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   instance_id  = excluded.instance_id,
		   step_name    = excluded.step_name,
		   assignee     = excluded.assignee,
		   deadline     = excluded.deadline,
		   status       = excluded.status,
		   created_at   = excluded.created_at,
		   completed_at = excluded.completed_at`,
		t.ID, t.InstanceID, t.StepName, t.Assignee, nullableTime(t.Deadline),
		string(t.Status), t.CreatedAt.Format(time.RFC3339), nullableTime(t.CompletedAt),
	)
	return err
}

func (s *SQLiteStore) LoadTask(id string) (*Task, error) {
	row := s.db.QueryRow(
		`SELECT instance_id, step_name, assignee, deadline, status, created_at, completed_at
		 FROM tasks WHERE id = ?`, id)
	return scanTask(id, row)
}

func (s *SQLiteStore) ListPendingTasks(assignee string) ([]*Task, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if assignee == "" {
		rows, err = s.db.Query(
			`SELECT id, instance_id, step_name, assignee, deadline, status, created_at, completed_at
			 FROM tasks WHERE status = ? ORDER BY id ASC`, string(TaskPending))
	} else {
		rows, err = s.db.Query(
			`SELECT id, instance_id, step_name, assignee, deadline, status, created_at, completed_at
			 FROM tasks WHERE status = ? AND assignee = ? ORDER BY id ASC`, string(TaskPending), assignee)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Task
	for rows.Next() {
		var (
			tid, instanceID, stepName, asg, status, createdAt string
			deadline, completedAt                             sql.NullString
		)
		if err := rows.Scan(&tid, &instanceID, &stepName, &asg, &deadline, &status, &createdAt, &completedAt); err != nil {
			return nil, err
		}
		t, err := buildTask(tid, instanceID, stepName, asg, deadline, status, createdAt, completedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) MarkTaskCompleted(id string, completedAt time.Time) error {
	res, err := s.db.Exec(
		`UPDATE tasks SET status = ?, completed_at = ? WHERE id = ? AND status = ?`,
		string(TaskCompleted), completedAt.Format(time.RFC3339), id, string(TaskPending),
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	// 0 строк: либо задачи нет, либо уже завершена. Различаем явным чтением.
	var dummy string
	row := s.db.QueryRow(`SELECT id FROM tasks WHERE id = ?`, id)
	if err := row.Scan(&dummy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskNotFound
		}
		return err
	}
	return ErrTaskAlreadyCompleted
}

func (s *SQLiteStore) NextInstanceID() (string, error) {
	n, err := s.nextCounter("instance")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("p-%06d", n), nil
}

func (s *SQLiteStore) NextTaskID() (string, error) {
	n, err := s.nextCounter("task")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("t-%06d", n), nil
}

// nextCounter инкрементирует счётчик и читает новое значение в одной транзакции
// (D-10; сид counters гарантирует наличие строки).
func (s *SQLiteStore) nextCounter(name string) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE counters SET value = value + 1 WHERE name = ?`, name); err != nil {
		return 0, err
	}
	var n int64
	if err := tx.QueryRow(`SELECT value FROM counters WHERE name = ?`, name).Scan(&n); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// rowScanner — общий интерфейс *sql.Row / *sql.Rows для scanTask.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(id string, row rowScanner) (*Task, error) {
	var (
		instanceID, stepName, assignee, status, createdAt string
		deadline, completedAt                             sql.NullString
	)
	if err := row.Scan(&instanceID, &stepName, &assignee, &deadline, &status, &createdAt, &completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return buildTask(id, instanceID, stepName, assignee, deadline, status, createdAt, completedAt)
}

func buildTask(id, instanceID, stepName, assignee string, deadline sql.NullString, status, createdAt string, completedAt sql.NullString) (*Task, error) {
	created, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	t := &Task{
		ID:         id,
		InstanceID: instanceID,
		StepName:   stepName,
		Assignee:   assignee,
		Status:     TaskStatus(status),
		CreatedAt:  created,
	}
	if deadline.Valid {
		d, err := parseTime(deadline.String)
		if err != nil {
			return nil, err
		}
		t.Deadline = &d
	}
	if completedAt.Valid {
		c, err := parseTime(completedAt.String)
		if err != nil {
			return nil, err
		}
		t.CompletedAt = &c
	}
	return t, nil
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// nullableTime превращает *time.Time в sql-аргумент: nil → NULL, иначе RFC3339.
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}
