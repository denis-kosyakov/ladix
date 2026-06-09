// Command ladix — CLI интерпретатора Ladix (фичи 003/004).
//
// Подкоманда run исполняет файл .ladix: конвейер лексер → парсер (накопленные
// синтаксические ошибки → печать и выход) → Analyze (семпроход, первая ошибка →
// печать и выход) → Interpreter.Run. Подкоманда metric (фича 004) вычисляет одну
// именованную метрику из файла и печатает её значение в stdout: лексер → парсер →
// Analyze (регистрация источников/метрик) → EvalMetricByName → value.String. Флаг
// --max-depth N задаёт лимит глубины рекурсии. Граница подкоманды обёрнута
// recover-барьером: штатные ошибки Ladix → stderr/код 1; Go-паника → «внутренняя
// ошибка интерпретатора» без stack trace/код 1; ошибка использования CLI → код 2;
// успех → код 0 (§10, §SM-11/§SM-9.D).
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = "использование: ladix run [--max-depth N] <файл> | ladix metric [--max-depth N] <файл> <имя>"

// realMain — диспетчер подкоманд (§SM-11 CM-2): ветвление по args[0]. Возвращает
// код возврата. Вынесен из main и параметризован вводом/выводом для тестируемости.
func realMain(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		return runMain(args[1:], stdout, stderr)
	case "metric":
		return metricMain(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

// runMain разбирает аргументы подкоманды run и исполняет файл (логика фичи 003 без
// изменений). rest — аргументы ПОСЛЕ «run».
func runMain(rest []string, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
	file := ""
	for k := 0; k < len(rest); k++ {
		a := rest[k]
		switch {
		case a == "--max-depth":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --max-depth требует значение")
				return 2
			}
			n, err := strconv.Atoi(rest[k+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(stderr, "ladix: неверное значение --max-depth")
				return 2
			}
			maxDepth = n
			k++
		case strings.HasPrefix(a, "--max-depth="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--max-depth="))
			if err != nil || n <= 0 {
				fmt.Fprintln(stderr, "ladix: неверное значение --max-depth")
				return 2
			}
			maxDepth = n
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "ladix: неизвестный флаг %s\n", a)
			return 2
		default:
			if file != "" {
				fmt.Fprintln(stderr, "ladix: лишний аргумент "+a)
				return 2
			}
			file = a
		}
	}
	if file == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	return runFile(file, maxDepth, stdout, stderr)
}

// metricMain разбирает аргументы подкоманды metric и вычисляет одну метрику (§SM-11
// CM-3). rest — аргументы ПОСЛЕ «metric»: два позиционных (файл, имя метрики) +
// опциональный --max-depth. Прод-Clock — eval.SystemClock{} (R4, фиксируется на
// запуск). Меньше/больше двух позиционных, неверный флаг, лишний аргумент → код 2.
func metricMain(rest []string, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
	var positional []string
	for k := 0; k < len(rest); k++ {
		a := rest[k]
		switch {
		case a == "--max-depth":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --max-depth требует значение")
				return 2
			}
			n, err := strconv.Atoi(rest[k+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(stderr, "ladix: неверное значение --max-depth")
				return 2
			}
			maxDepth = n
			k++
		case strings.HasPrefix(a, "--max-depth="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--max-depth="))
			if err != nil || n <= 0 {
				fmt.Fprintln(stderr, "ladix: неверное значение --max-depth")
				return 2
			}
			maxDepth = n
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "ladix: неизвестный флаг %s\n", a)
			return 2
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 2 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	return runMetric(positional[0], positional[1], maxDepth, eval.SystemClock{}, stdout, stderr)
}

// runFile исполняет один файл и возвращает код возврата (0/1/2).
func runFile(path string, maxDepth int, stdout, stderr io.Writer) int {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "ladix: не удалось прочитать файл %q\n", path)
		return 2 // ошибка использования CLI
	}
	tokens, errList := lexer.New(string(src)).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		fmt.Fprintln(stderr, errList.Error())
		return 1 // накопленные лексические/синтаксические ошибки
	}
	return guard(stderr, func() int {
		interp := eval.NewInterpreter(stdout, maxDepth, eval.SystemClock{})
		if err := interp.Run(prog); err != nil {
			// Все типы eval/lexer/parser реализуют канонический двухстрочный Error() (§8.1).
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		return 0
	})
}

// runMetric вычисляет одну именованную метрику из файла и печатает её значение в
// stdout (§SM-11 CM-3/CM-5). Конвейер: чтение файла → лексер → парсер → (под guard)
// Analyze → EvalMetricByName → value.String + '\n'. Clock инжектируется (прод —
// SystemClock; тест — FixedClock для детерминированного golden, аналог runFile).
// Коды: чтение файла → 2 (использование); лекс/синт, Analyze, поиск метрики
// (§SM-9.D), загрузка/вычисление (§SM-9.B/C), Go-паника → 1; успех → 0.
func runMetric(path, metricName string, maxDepth int, clock eval.Clock, stdout, stderr io.Writer) int {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "ladix: не удалось прочитать файл %q\n", path)
		return 2 // ошибка использования CLI (вне recover-барьера, CM-5)
	}
	tokens, errList := lexer.New(string(src)).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		fmt.Fprintln(stderr, errList.Error())
		return 1 // накопленные лексические/синтаксические ошибки
	}
	return guard(stderr, func() int {
		interp := eval.NewInterpreter(stdout, maxDepth, clock)
		if err := interp.Analyze(prog); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1 // семпроход (§SM-4/§SM-9.A)
		}
		v, err := interp.EvalMetricByName(metricName)
		if err != nil {
			// §SM-9.D (поиск метрики) / §SM-9.B/C (загрузка/вычисление):
			// канонический двухстрочный Error().
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		fmt.Fprintln(stdout, value.String(v))
		return 0
	})
}

// guard — recover-барьер на границе подкоманды (§10.2): непредвиденная Go-паника
// → дженерик «внутренняя ошибка интерпретатора» без stack trace, код 1.
func guard(stderr io.Writer, fn func() int) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(stderr, "внутренняя ошибка интерпретатора")
			code = 1
		}
	}()
	return fn()
}
