package native

import (
	"fmt"
	"strings"

	gosqlx "github.com/ajitpratap0/GoSQLX/pkg/gosqlx"
	sqlfmt "github.com/ajitpratap0/GoSQLX/pkg/formatter"
	astfmt "github.com/ajitpratap0/GoSQLX/pkg/sql/ast"
	"github.com/formatter/formatter/src/formatter"
)

// SQLFormatter 使用 GoSQLX (github.com/ajitpratap0/GoSQLX) 实现 SQL 格式化。
//
// 两阶段处理:
//  1. GoSQLX 解析 SQL 为 AST，以 NewlinePerClause 风格渲染 (关键字大写、子句换行)
//  2. reindentSQL 对 GoSQLX 输出进行二次缩进美化:
//     - 子查询 (括号内 SELECT) 内容缩进
//     - CASE/WHEN/THEN/ELSE/END 分行缩进
//     - AND/OR 条件分行
//     - CTE 定义体缩进
type SQLFormatter struct{}

func NewSQLFormatter() *SQLFormatter {
	return &SQLFormatter{}
}

func (f *SQLFormatter) Format(input []byte, lang string, opts formatter.FormatOptions) ([]byte, error) {
	// 解析 SQL 为 AST
	parsedAST, err := gosqlx.Parse(string(input))
	if err != nil {
		return nil, fmt.Errorf("sql format (gosqlx parse): %w", err)
	}

	// 构建 AST 级别格式化选项 (比顶层 gosqlx.FormatOptions 更多控制)
	indentSize := opts.TabWidth
	if indentSize <= 0 {
		indentSize = 2
	}

	// 阶段 1: GoSQLX 基础格式化 (关键字大写、子句换行)
	astOpts := astfmt.FormatOptions{
		IndentStyle:      astfmt.IndentSpaces,
		IndentWidth:      indentSize,
		KeywordCase:      astfmt.KeywordUpper, // 关键字大写 (SQL 通用规范)
		LineWidth:        80,
		NewlinePerClause: true, // 每个子句 (FROM/WHERE/GROUP BY 等) 独占一行
		AddSemicolon:     false,
	}
	raw := sqlfmt.FormatAST(parsedAST, astOpts)
	if raw == "" {
		return nil, fmt.Errorf("sql format: empty result")
	}

	// 阶段 2: 二次缩进美化
	// TabWidth == -1 (TabIndent) 时使用 Tab 缩进; 否则使用空格
	result := reindentSQL(raw, indentSize, opts.TabWidth == formatter.TabIndent)
	if result == "" {
		return []byte(raw), nil
	}
	return []byte(result), nil
}

// ---- SQL tokenizer (用于二次格式化) ----

type sqlTokenKind int

const (
	stkWord     sqlTokenKind = iota
	stkNumber
	stkString
	stkComment
	stkPunct   // ( ) , ; .
	stkOperator
)

type sqlToken2 struct {
	kind sqlTokenKind
	text string
}

func sqlTokenize2(src string) []sqlToken2 {
	var tokens []sqlToken2
	i, n := 0, len(src)
	for i < n {
		ch := src[i]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote := ch
			start := i
			i++
			for i < n {
				if src[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				if src[i] == quote {
					if quote == '\'' && i+1 < n && src[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			tokens = append(tokens, sqlToken2{stkString, src[start:i]})
			continue
		}
		if ch == '-' && i+1 < n && src[i+1] == '-' {
			start := i
			for i < n && src[i] != '\n' {
				i++
			}
			tokens = append(tokens, sqlToken2{stkComment, src[start:i]})
			continue
		}
		if ch == '/' && i+1 < n && src[i+1] == '*' {
			start := i
			i += 2
			for i+1 < n {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			if i >= n {
				i = n
			}
			tokens = append(tokens, sqlToken2{stkComment, src[start:i]})
			continue
		}
		if ch == '(' || ch == ')' || ch == ',' || ch == ';' || ch == '.' {
			tokens = append(tokens, sqlToken2{stkPunct, string(ch)})
			i++
			continue
		}
		if i+1 < n {
			switch src[i : i+2] {
			case ">=", "<=", "!=", "<>", "||", "==", "->", "=>":
				tokens = append(tokens, sqlToken2{stkOperator, src[i : i+2]})
				i += 2
				continue
			}
		}
		if strings.ContainsRune("+-*/=<>%", rune(ch)) {
			tokens = append(tokens, sqlToken2{stkOperator, string(ch)})
			i++
			continue
		}
		if ch >= '0' && ch <= '9' {
			start := i
			for i < n && ((src[i] >= '0' && src[i] <= '9') || src[i] == '.') {
				i++
			}
			tokens = append(tokens, sqlToken2{stkNumber, src[start:i]})
			continue
		}
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' {
			start := i
			for i < n && ((src[i] >= 'a' && src[i] <= 'z') || (src[i] >= 'A' && src[i] <= 'Z') ||
				(src[i] >= '0' && src[i] <= '9') || src[i] == '_' || src[i] == '$' || src[i] == '#') {
				i++
			}
			tokens = append(tokens, sqlToken2{stkWord, src[start:i]})
			continue
		}
		tokens = append(tokens, sqlToken2{stkWord, string(ch)})
		i++
	}
	return tokens
}

// parenType 记录括号类型: 函数调用 vs 子查询/列表
type parenType int

const (
	parenFunc     parenType = iota // 函数调用: COUNT( ... )
	parenSubquery                  // 子查询: (SELECT ...), IN (...)
	parenList                      // 列表: VALUES (...), users (...)
)

// reindentSQL 对 GoSQLX 输出进行二次缩进美化
// useTabs=true 时使用 Tab 缩进, 否则使用 indentSize 个空格 × depth
func reindentSQL(src string, indentSize int, useTabs bool) string {
	tokens := sqlTokenize2(src)
	if len(tokens) == 0 {
		return ""
	}

	var sb strings.Builder
	depth := 0           // 缩进层级
	caseDepth := 0       // CASE 嵌套深度
	atLineStart := true
	var parenStack []parenType // 括号类型栈

	indent := func() string {
		if useTabs {
			return strings.Repeat("\t", depth)
		}
		return strings.Repeat(" ", depth*indentSize)
	}

	write := func(s string) {
		if atLineStart {
			sb.WriteString(indent())
			atLineStart = false
		}
		sb.WriteString(s)
	}

	newLine := func() {
		sb.WriteByte('\n')
		atLineStart = true
	}

	isFunc := func(w string) bool {
		switch strings.ToUpper(w) {
		case "COUNT", "SUM", "AVG", "MIN", "MAX", "COALESCE", "NULLIF",
			"IFNULL", "IF", "CAST", "CONVERT", "EXTRACT", "ROW_NUMBER",
			"RANK", "DENSE_RANK", "GROUP_CONCAT", "CONCAT", "SUBSTRING",
			"DATE_SUB", "DATE_ADD", "DATE_FORMAT", "NOW", "LENGTH",
			"VARCHAR", "INTEGER", "INT", "DECIMAL", "CHAR", "TEXT",
			"TIMESTAMP", "DATETIME", "BOOLEAN", "BLOB", "FLOAT", "DOUBLE",
			"TRIM", "LOWER", "UPPER", "REPLACE", "ROUND":
			return true
		}
		return false
	}

	isMajorClause := func(w string) bool {
		switch strings.ToUpper(w) {
		case "SELECT", "FROM", "WHERE", "VALUES", "SET",
			"GROUP", "ORDER", "HAVING", "LIMIT", "OFFSET",
			"UNION", "INTERSECT", "EXCEPT",
			"INSERT", "UPDATE", "DELETE",
			"CREATE", "ALTER", "DROP", "WITH":
			return true
		}
		return false
	}

	isJoinPrefix := func(w string) bool {
		switch strings.ToUpper(w) {
		case "LEFT", "RIGHT", "INNER", "FULL", "CROSS":
			return true
		}
		return false
	}

	// inFuncCall 判断当前是否在函数调用的括号内
	inFuncCall := func() bool {
		for _, pt := range parenStack {
			if pt == parenFunc {
				return true
			}
		}
		return false
	}

	for idx := 0; idx < len(tokens); idx++ {
		t := tokens[idx]
		upper := ""
		if t.kind == stkWord {
			upper = strings.ToUpper(t.text)
		}

		// ---- 换行判断 (仅在非函数调用内) ----
		if idx > 0 && !inFuncCall() {
			prev := tokens[idx-1]

			// CASE 表达式分支
			if t.kind == stkWord {
				switch upper {
				case "CASE":
					newLine()
					caseDepth++
				case "WHEN", "ELSE":
					newLine()
				case "END":
					newLine()
					if caseDepth > 0 {
						caseDepth--
					}
				}
			}

			// AND/OR 条件分行 (不在 CASE 块内)
			if t.kind == stkWord && (upper == "AND" || upper == "OR") && caseDepth == 0 {
				newLine()
			}

			// 主要子句前换行
			if t.kind == stkWord && isMajorClause(upper) {
				skipNewline := false
				if upper == "BY" && prev.kind == stkWord {
					pu := strings.ToUpper(prev.text)
					if pu == "GROUP" || pu == "ORDER" {
						skipNewline = true
					}
				}
				if upper == "INTO" && prev.kind == stkWord &&
					strings.ToUpper(prev.text) == "INSERT" {
					skipNewline = true
				}
				if upper == "ALL" && prev.kind == stkWord &&
					strings.ToUpper(prev.text) == "UNION" {
					skipNewline = true
				}
				if upper == "RECURSIVE" && prev.kind == stkWord &&
					strings.ToUpper(prev.text) == "WITH" {
					skipNewline = true
				}
				if !skipNewline {
					newLine()
				}
			}

			// JOIN 前换行
			if t.kind == stkWord && isJoinPrefix(upper) {
				if idx+1 < len(tokens) && tokens[idx+1].kind == stkWord {
					next := strings.ToUpper(tokens[idx+1].text)
					if next == "JOIN" || next == "OUTER" {
						newLine()
					}
				}
			}
			if t.kind == stkWord && upper == "ON" && caseDepth == 0 {
				newLine()
			}

			// ) 后跟 , (CTE 分隔) → 换行
			if prev.kind == stkPunct && prev.text == ")" && t.kind == stkPunct && t.text == "," {
				newLine()
			}
			// ) 后跟主要子句 → 换行
			if prev.kind == stkPunct && prev.text == ")" && t.kind == stkWord && isMajorClause(upper) {
				newLine()
			}
		}

		// ---- 括号处理 ----
		if t.kind == stkPunct && t.text == "(" {
			// 判断括号类型
			pt := parenList
			if idx > 0 && tokens[idx-1].kind == stkWord {
				prevWord := strings.ToUpper(tokens[idx-1].text)
				if isFunc(prevWord) {
					pt = parenFunc
				} else if idx+1 < len(tokens) && tokens[idx+1].kind == stkWord &&
					strings.ToUpper(tokens[idx+1].text) == "SELECT" {
					pt = parenSubquery
				}
			}

			// 空格判断: 非 函数调用前需要空格
			if idx > 0 && !atLineStart {
				prev := tokens[idx-1]
				if pt != parenFunc {
					// 前一个 token 是 word 或 ) 需要空格
					if prev.kind == stkWord || (prev.kind == stkPunct && prev.text == ")") {
						write(" ")
					}
				}
			}

			write("(")
			parenStack = append(parenStack, pt)
			if pt == parenSubquery {
				depth++
				newLine()
			}
			continue
		}
		if t.kind == stkPunct && t.text == ")" {
			if len(parenStack) > 0 {
				pt := parenStack[len(parenStack)-1]
				parenStack = parenStack[:len(parenStack)-1]
				if pt == parenSubquery {
					if !atLineStart {
						newLine()
					}
					if depth > 0 {
						depth--
					}
				}
			}
			write(")")
			continue
		}

		// ---- 逗号处理 (仅在非函数调用内换行) ----
		if t.kind == stkPunct && t.text == "," {
			write(",")
			if !inFuncCall() {
				newLine()
			} else {
				write(" ")
			}
			continue
		}

		// ---- 分号处理 ----
		if t.kind == stkPunct && t.text == ";" {
			write(";")
			if idx < len(tokens)-1 {
				newLine()
			}
			continue
		}

		// ---- 空格判断 ----
		if idx > 0 && !atLineStart {
			prev := tokens[idx-1]
			needSpace := true
			if prev.kind == stkPunct {
				switch prev.text {
				case "(", ".":
					needSpace = false
				}
			}
			if t.kind == stkPunct {
				switch t.text {
				case ")", ",", ";", ".":
					needSpace = false
				}
			}
			if prev.kind == stkOperator || t.kind == stkOperator {
				needSpace = true
			}
			// word 后跟 . 不加空格
			if prev.kind == stkWord && t.kind == stkPunct && t.text == "." {
				needSpace = false
			}
			if needSpace {
				write(" ")
			}
		}

		write(t.text)
	}

	result := strings.TrimSpace(sb.String())
	// 清理多余空行
	for strings.Contains(result, "\n\n") {
		result = strings.ReplaceAll(result, "\n\n", "\n")
	}
	return result
}

func (f *SQLFormatter) SupportedLanguages() []string {
	return []string{"sql"}
}

func (f *SQLFormatter) Name() string {
	return "native-sql"
}

func (f *SQLFormatter) Type() formatter.BackendType {
	return formatter.NativeGo
}
