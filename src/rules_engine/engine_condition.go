package rules_engine

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	Literal = iota
	Operator
)

var SRegex = regexp.MustCompile("\\s")

type ReCepToken struct {
	Data  string
	Type  int
	Index int
}

type ReCepAST struct {
	Ori        string
	Tokens     []ReCepToken
	currTok    ReCepToken
	currIndex  int
	Err        error
	ExprAST    ExprAST
	TokenVal   map[string]bool
	AllLiteral map[string]bool
}

func ErrPos(s string, pos int) string {
	r := strings.Repeat("-", len(s)) + "\n"
	s += "\n"
	for i := 0; i < pos; i++ {
		s += " "
	}
	s += "^\n"
	return r + s + r
}

type ExprAST interface {
	toStr() string
}

type NumberExprAST struct {
	Val string
}

type BinaryExprAST struct {
	Op string
	Lhs,
	Rhs ExprAST
}

type UnaryExprAST struct {
	Op      string
	Operand ExprAST
}

func (n NumberExprAST) toStr() string {
	return fmt.Sprintf("NumberExprAST:%s", n.Val)
}

func (b BinaryExprAST) toStr() string {
	return fmt.Sprintf(
		"BinaryExprAST: (%s %s %s)",
		b.Op,
		b.Lhs.toStr(),
		b.Rhs.toStr(),
	)
}

func (u UnaryExprAST) toStr() string {
	return fmt.Sprintf(
		"UnaryExprAST: (%s %s)",
		u.Op,
		u.Operand.toStr(),
	)
}

func (a *ReCepAST) getNextToken() *ReCepToken {
	a.currIndex++
	if a.currIndex < len(a.Tokens) {
		a.currTok = a.Tokens[a.currIndex]
		return &a.currTok
	}
	return nil
}

func (a *ReCepAST) ParseExpression() ExprAST {
	lhs := a.parsePrimary()
	return a.parseBinOpRHS(0, lhs)
}

func (a *ReCepAST) parseNumber() NumberExprAST {
	n := NumberExprAST{
		Val: a.currTok.Data,
	}
	a.getNextToken()
	return n
}

func (a *ReCepAST) getTokPrecedence() int {
	if a.currTok.Type == Operator && a.currTok.Data != "(" && a.currTok.Data != ")" {
		return 1
	}
	return -1
}

func (a *ReCepAST) parsePrimary() ExprAST {
	switch a.currTok.Type {
	case Literal:
		return a.parseNumber()
	case Operator:
		if a.currTok.Data == "(" {
			a.getNextToken()
			e := a.ParseExpression()
			if e == nil {
				return nil
			}
			if a.currTok.Data != ")" {
				a.Err = errors.New(
					fmt.Sprintf("want ')' but get %s\n%s",
						a.currTok.Data,
						ErrPos(a.Ori, a.currTok.Index)))
				return nil
			}
			a.getNextToken()
			return e
		} else if a.currTok.Data == "!" {
			// Handle NOT operator
			a.getNextToken()
			operand := a.parsePrimary()
			if operand == nil {
				return nil
			}
			return UnaryExprAST{
				Op:      "!",
				Operand: operand,
			}
		} else {
			return a.parseNumber()
		}
	default:
		return nil
	}
}

func (a *ReCepAST) parseBinOpRHS(execPrev int, lhs ExprAST) ExprAST {
	for {
		tokPrev := a.getTokPrecedence()
		if tokPrev < execPrev {
			return lhs
		}
		binOp := a.currTok.Data
		if a.getNextToken() == nil {
			return lhs
		}
		rhs := a.parsePrimary()
		if rhs == nil {
			return nil
		}
		nextPrev := a.getTokPrecedence()
		if tokPrev < nextPrev {
			rhs = a.parseBinOpRHS(tokPrev+1, rhs)
			if rhs == nil {
				return nil
			}
		}
		lhs = BinaryExprAST{
			Op:  binOp,
			Lhs: lhs,
			Rhs: rhs,
		}
	}
}

func (a *ReCepAST) TokenParser(ori string) []ReCepToken {
	res := make([]ReCepToken, 0, 10)
	ori = strings.ReplaceAll(ori, "(", " ( ")
	ori = strings.ReplaceAll(ori, ")", " ) ")
	ori = strings.ReplaceAll(ori, "not", " not ")
	tokenList := SRegex.Split(ori, -1)

	for i, v := range tokenList {
		if strings.ToLower(v) == "and" {
			res = append(res, ReCepToken{Data: "&", Type: Operator, Index: i})
			continue
		}

		if strings.ToLower(v) == "or" {
			res = append(res, ReCepToken{Data: "|", Type: Operator, Index: i})
			continue
		}

		if strings.ToLower(v) == "not" {
			res = append(res, ReCepToken{Data: "!", Type: Operator, Index: i})
			continue
		}

		switch v {
		case "(":
			res = append(res, ReCepToken{Data: "(", Type: Operator, Index: i})
		case ")":
			res = append(res, ReCepToken{Data: ")", Type: Operator, Index: i})
		default:
			if len(v) > 0 {
				a.AllLiteral[v] = true
				res = append(res, ReCepToken{Data: v, Type: Literal, Index: i})
			}
		}
	}
	return res
}

func (a *ReCepAST) ExprASTResult(e ExprAST, tokenVal map[string]bool) bool {
	switch ast := e.(type) {
	case BinaryExprAST:
		l := a.ExprASTResult(ast.Lhs, tokenVal)
		r := a.ExprASTResult(ast.Rhs, tokenVal)
		switch ast.Op {
		case "&":
			return l && r
		case "|":
			return l || r
		}
	case UnaryExprAST:
		operand := a.ExprASTResult(ast.Operand, tokenVal)
		switch ast.Op {
		case "!":
			return !operand
		}
	case NumberExprAST:
		return tokenVal[ast.Val]
	}
	return false
}

func (a *ReCepAST) AddTokenVal(data map[string]int) {
	a.TokenVal = make(map[string]bool, 3)
	for k, v := range data {
		if v == 0 {
			a.TokenVal[k] = true
		} else {
			a.TokenVal[k] = false
		}
	}
}

func (a *ReCepAST) DelTokenVal() {
	a.TokenVal = nil
}

func GetAST(ori string) *ReCepAST {
	res := ReCepAST{
		Ori:        ori,
		currIndex:  0,
		AllLiteral: make(map[string]bool, 3),
	}
	res.Tokens = res.TokenParser(res.Ori)
	res.currTok = res.Tokens[0]
	res.ExprAST = res.ParseExpression()
	return &res
}
