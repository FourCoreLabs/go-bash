package xan

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

// rowOutput streams derived results, so joins never allocate their Cartesian
// product. It also enforces an output budget when called outside the interpreter.
type rowOutput struct {
	ctx    context.Context
	writer *csv.Writer
}
type outputBudget struct {
	w         io.Writer
	remaining int
}

func (w *outputBudget) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, fmt.Errorf("output size limit exceeded")
	}
	n, err := w.w.Write(p)
	w.remaining -= n
	return n, err
}
func newRowOutput(ctx context.Context, c *command.Context) *rowOutput {
	limit := c.Limits.MaxOutputSize
	if limit <= 0 {
		limit = 10 * 1024 * 1024
	}
	return &rowOutput{ctx: ctx, writer: csv.NewWriter(&outputBudget{w: c.Stdout, remaining: limit})}
}
func (o *rowOutput) write(row []string) error {
	if err := o.ctx.Err(); err != nil {
		return err
	}
	if err := o.writer.Write(row); err != nil {
		return err
	}
	// Flush each row to stop an amplified join as soon as its budget is exhausted.
	o.writer.Flush()
	return o.writer.Error()
}
func xanError(c *command.Context, err error) command.Result {
	return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
}
func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}
func namedColumn(header []string, name string) int {
	for i, h := range header {
		if h == name {
			return i
		}
	}
	return -1
}

func runRows(ctx context.Context, c *command.Context, sub string, args []string) command.Result {
	n := 10
	column := ""
	numeric, reverse := false, false
	var files []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			files = append(files, args[i+1:]...)
			i = len(args)
		case (sub == "head" || sub == "tail") && (a == "-n" || a == "-l"):
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			var err error
			n, err = strconv.Atoi(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		case (sub == "sort" || sub == "dedup") && a == "-s":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			column = args[i]
		case sub == "sort" && (a == "-N" || a == "--numeric"):
			numeric = true
		case sub == "sort" && (a == "-R" || a == "-r" || a == "--reverse"):
			reverse = true
		case strings.HasPrefix(a, "-") && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			files = append(files, a)
		}
	}
	file, _ := popOneFile(files)
	header, rows, err := readCSV(c, file)
	if err != nil {
		if file != "-" {
			err = fmt.Errorf("%s: No such file or directory", file)
		}
		return xanError(c, err)
	}
	if err = ctx.Err(); err != nil {
		return xanError(c, err)
	}
	// Normalize ragged records for the header-oriented commands.
	for i, row := range rows {
		normalized := make([]string, len(header))
		copy(normalized, row)
		rows[i] = normalized
	}
	switch sub {
	case "head":
		end := n
		if end < 0 {
			end = len(rows) + end
		}
		if end < 0 {
			end = 0
		}
		if end > len(rows) {
			end = len(rows)
		}
		rows = rows[:end]
	case "tail":
		// JS slice(-n): n=0 retains all rows; negative n drops the first -n.
		start := 0
		if n > 0 {
			if n < len(rows) {
				start = len(rows) - n
			}
		} else if n < 0 {
			if n <= -len(rows) {
				start = len(rows)
			} else {
				start = -n
			}
		}
		rows = rows[start:]
	case "sort":
		if column == "" && len(header) > 0 {
			column = header[0]
		}
		idx := namedColumn(header, column)
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := cell(rows[i], idx), cell(rows[j], idx)
			cmp := 0
			if numeric {
				na, nb := jsParseFloat(a), jsParseFloat(b)
				if na < nb {
					cmp = -1
				} else if na > nb {
					cmp = 1
				}
			} else {
				cmp = compareText(a, b)
			}
			if reverse {
				return cmp > 0
			}
			return cmp < 0
		})
	case "dedup":
		seen := map[string]bool{}
		result := rows[:0]
		idx := namedColumn(header, column)
		for _, row := range rows {
			if err = ctx.Err(); err != nil {
				return xanError(c, err)
			}
			key := cell(row, idx)
			if column == "" {
				data, _ := json.Marshal(row)
				key = string(data)
			}
			if !seen[key] {
				seen[key] = true
				result = append(result, row)
			}
		}
		rows = result
	case "transpose":
		if len(rows) == 0 {
			result := make([][]string, len(header))
			for i, h := range header {
				result[i] = []string{h}
			}
			header, rows = []string{"column"}, result
		} else {
			if len(header) == 0 {
				return xanError(c, fmt.Errorf("transpose: missing headers"))
			}
			newHeader := []string{header[0]}
			seen := map[string]bool{header[0]: true}
			for _, row := range rows {
				name := cell(row, 0)
				if seen[name] {
					return xanError(c, fmt.Errorf("transpose: duplicate output headers"))
				}
				seen[name] = true
				newHeader = append(newHeader, name)
			}
			result := make([][]string, 0, len(header)-1)
			for i := 1; i < len(header); i++ {
				if err = ctx.Err(); err != nil {
					return xanError(c, err)
				}
				row := make([]string, len(newHeader))
				row[0] = header[i]
				for j, r := range rows {
					row[j+1] = cell(r, i)
				}
				result = append(result, row)
			}
			header, rows = newHeader, result
		}
	}
	out := newRowOutput(ctx, c)
	if len(header) > 0 {
		if err = out.write(header); err != nil {
			return xanError(c, err)
		}
	}
	for _, row := range rows {
		if err = out.write(row); err != nil {
			return xanError(c, err)
		}
	}
	return command.Result{}
}

// Number.parseFloat accepts a numeric prefix, unlike strconv.ParseFloat.
var numericPrefix = regexp.MustCompile(`^[+-]?(?:Infinity|(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?)`)

func jsParseFloat(s string) float64 {
	prefix := numericPrefix.FindString(strings.TrimSpace(s))
	if prefix == "" {
		return math.NaN()
	}
	n, _ := strconv.ParseFloat(prefix, 64)
	return n
}

// Match the common ASCII localeCompare ordering (case-insensitive primary
// weights, lowercase before uppercase). Full ICU collation is not available.
func compareText(a, b string) int {
	if cmp := strings.Compare(strings.ToLower(a), strings.ToLower(b)); cmp != 0 {
		return cmp
	}
	return -strings.Compare(a, b)
}

func runJoin(ctx context.Context, c *command.Context, args []string) command.Result {
	mode := "inner"
	def := ""
	var pos []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--left", "--right", "--full":
			mode = strings.TrimPrefix(a, "--")
		case "-D", "--default":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			def = args[i]
		case "--":
			pos = append(pos, args[i+1:]...)
			i = len(args)
		default:
			if strings.HasPrefix(a, "-") && a != "-" {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			pos = append(pos, a)
		}
	}
	if len(pos) != 4 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	if pos[1] == "-" && pos[3] == "-" {
		return xanError(c, fmt.Errorf("join: cannot read both inputs from stdin"))
	}
	h1, r1, err := readCSV(c, pos[1])
	if err != nil {
		return xanError(c, err)
	}
	h2, r2, err := readCSV(c, pos[3])
	if err != nil {
		return xanError(c, err)
	}
	k1, k2 := namedColumn(h1, pos[0]), namedColumn(h2, pos[2])
	if k1 < 0 || k2 < 0 {
		return xanError(c, fmt.Errorf("join: key column not found"))
	}
	header := append([]string(nil), h1...)
	var unique []int
	for i, h := range h2 {
		if namedColumn(h1, h) < 0 {
			header = append(header, h)
			unique = append(unique, i)
		}
	}
	index := map[string][]int{}
	for i, row := range r2 {
		if err = ctx.Err(); err != nil {
			return xanError(c, err)
		}
		key := cell(row, k2)
		index[key] = append(index[key], i)
	}
	out := newRowOutput(ctx, c)
	if err = out.write(header); err != nil {
		return xanError(c, err)
	}
	matched := map[string]bool{}
	emit := func(left, right []string) error {
		row := make([]string, len(header))
		for i, h := range h1 {
			if left != nil {
				row[i] = cell(left, i)
			} else if j := namedColumn(h2, h); j >= 0 {
				row[i] = cell(right, j)
			} else {
				row[i] = def
			}
		}
		for j, i := range unique {
			if right != nil {
				row[len(h1)+j] = cell(right, i)
			} else {
				row[len(h1)+j] = def
			}
		}
		return out.write(row)
	}
	for _, row := range r1 {
		if err = ctx.Err(); err != nil {
			return xanError(c, err)
		}
		key := cell(row, k1)
		matches := index[key]
		if len(matches) > 0 {
			matched[key] = true
			for _, j := range matches {
				if err = emit(row, r2[j]); err != nil {
					return xanError(c, err)
				}
			}
		} else if mode == "left" || mode == "full" {
			if err = emit(row, nil); err != nil {
				return xanError(c, err)
			}
		}
	}
	if mode == "right" || mode == "full" {
		for _, row := range r2 {
			if !matched[cell(row, k2)] {
				if err = emit(nil, row); err != nil {
					return xanError(c, err)
				}
			}
		}
	}
	return command.Result{}
}
