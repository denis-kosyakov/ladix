// Command ladix — CLI интерпретатора Ladix (фича 003).
//
// Подкоманда run исполняет файл .ladix: конвейер лексер → парсер (накопленные
// синтаксические ошибки → печать и выход) → Analyze (семпроход, первая ошибка →
// печать и выход) → Interpreter.Run. Флаг --max-depth N задаёт лимит глубины
// рекурсии. Граница подкоманды обёрнута recover-барьером: штатные ошибки Ladix →
// stderr/код 1; Go-паника → «внутренняя ошибка интерпретатора» без stack trace/
// код 1; ошибка использования CLI → код 2; успех → код 0 (§10).
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
)

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = "использование: ladix run [--max-depth N] <файл>"

// realMain разбирает аргументы и запускает подкоманду; возвращает код возврата.
// Вынесен из main и параметризован вводом/выводом для тестируемости.
func realMain(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] != "run" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	maxDepth := eval.DefaultMaxDepth
	file := ""
	rest := args[1:]
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
