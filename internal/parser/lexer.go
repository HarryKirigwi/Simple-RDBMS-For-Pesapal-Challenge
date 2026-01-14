package parser

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenType represents the type of a token
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIllegal
	TokenIdentifier
	TokenString
	TokenNumber
	TokenInteger
	TokenFloat
	TokenBoolean
	TokenKeyword
	TokenOperator
	TokenPunctuation
)

// Token represents a lexical token
type Token struct {
	Type    TokenType
	Literal string
	Pos     int
}

// Lexer tokenizes SQL input
type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           byte
}

// NewLexer creates a new lexer
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// NextToken returns the next token
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	tok.Pos = l.position

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenOperator, Literal: "==", Pos: tok.Pos}
		} else {
			tok = Token{Type: TokenOperator, Literal: "=", Pos: tok.Pos}
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenOperator, Literal: "!=", Pos: tok.Pos}
		} else {
			tok = Token{Type: TokenOperator, Literal: "!", Pos: tok.Pos}
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenOperator, Literal: "<=", Pos: tok.Pos}
		} else {
			tok = Token{Type: TokenOperator, Literal: "<", Pos: tok.Pos}
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenOperator, Literal: ">=", Pos: tok.Pos}
		} else {
			tok = Token{Type: TokenOperator, Literal: ">", Pos: tok.Pos}
		}
	case '+':
		tok = Token{Type: TokenOperator, Literal: "+", Pos: tok.Pos}
	case '-':
		tok = Token{Type: TokenOperator, Literal: "-", Pos: tok.Pos}
	case '*':
		tok = Token{Type: TokenOperator, Literal: "*", Pos: tok.Pos}
	case '/':
		tok = Token{Type: TokenOperator, Literal: "/", Pos: tok.Pos}
	case '%':
		tok = Token{Type: TokenOperator, Literal: "%", Pos: tok.Pos}
	case '(':
		tok = Token{Type: TokenPunctuation, Literal: "(", Pos: tok.Pos}
	case ')':
		tok = Token{Type: TokenPunctuation, Literal: ")", Pos: tok.Pos}
	case ',':
		tok = Token{Type: TokenPunctuation, Literal: ",", Pos: tok.Pos}
	case ';':
		tok = Token{Type: TokenPunctuation, Literal: ";", Pos: tok.Pos}
	case '.':
		tok = Token{Type: TokenPunctuation, Literal: ".", Pos: tok.Pos}
	case '\'':
		tok.Literal = l.readString()
		tok.Type = TokenString
		return tok
	case 0:
		tok.Literal = ""
		tok.Type = TokenEOF
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = lookupIdent(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			tok = l.readNumber()
			return tok
		} else {
			tok = Token{Type: TokenIllegal, Literal: string(l.ch), Pos: tok.Pos}
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readString() string {
	position := l.position + 1
	for {
		l.readChar()
		if l.ch == '\'' {
			// Handle escaped quotes
			if l.peekChar() == '\'' {
				l.readChar()
				continue
			}
			break
		}
		if l.ch == 0 {
			break
		}
	}
	str := l.input[position:l.position]
	l.readChar()
	// Replace escaped quotes
	str = strings.ReplaceAll(str, "''", "'")
	return str
}

func (l *Lexer) readNumber() Token {
	position := l.position
	tok := Token{Pos: position}

	for isDigit(l.ch) {
		l.readChar()
	}

	if l.ch == '.' {
		l.readChar()
		for isDigit(l.ch) {
			l.readChar()
		}
		tok.Type = TokenFloat
		tok.Literal = l.input[position:l.position]
		return tok
	}

	tok.Type = TokenInteger
	tok.Literal = l.input[position:l.position]
	return tok
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_' || ch >= utf8.RuneSelf && unicode.IsLetter(rune(ch))
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

var keywords = map[string]TokenType{
	"SELECT":   TokenKeyword,
	"FROM":     TokenKeyword,
	"WHERE":    TokenKeyword,
	"INSERT":   TokenKeyword,
	"INTO":     TokenKeyword,
	"VALUES":   TokenKeyword,
	"UPDATE":   TokenKeyword,
	"SET":      TokenKeyword,
	"DELETE":   TokenKeyword,
	"CREATE":   TokenKeyword,
	"TABLE":    TokenKeyword,
	"PRIMARY":  TokenKeyword,
	"KEY":      TokenKeyword,
	"UNIQUE":   TokenKeyword,
	"NOT":      TokenKeyword,
	"NULL":     TokenKeyword,
	"AND":      TokenKeyword,
	"OR":       TokenKeyword,
	"JOIN":     TokenKeyword,
	"INNER":    TokenKeyword,
	"LEFT":     TokenKeyword,
	"RIGHT":    TokenKeyword,
	"FULL":     TokenKeyword,
	"OUTER":    TokenKeyword,
	"ON":       TokenKeyword,
	"ORDER":    TokenKeyword,
	"BY":       TokenKeyword,
	"ASC":      TokenKeyword,
	"DESC":     TokenKeyword,
	"LIMIT":    TokenKeyword,
	"IN":       TokenKeyword,
	"IS":       TokenKeyword,
	"TRUE":     TokenKeyword,
	"FALSE":    TokenKeyword,
	"INTEGER":  TokenKeyword,
	"INT":      TokenKeyword,
	"VARCHAR":  TokenKeyword,
	"TEXT":     TokenKeyword,
	"BOOLEAN":  TokenKeyword,
	"BOOL":     TokenKeyword,
	"FLOAT":    TokenKeyword,
	"DATE":     TokenKeyword,
	"TIMESTAMP": TokenKeyword,
}

func lookupIdent(ident string) TokenType {
	if tok, ok := keywords[strings.ToUpper(ident)]; ok {
		return tok
	}
	return TokenIdentifier
}

// Error represents a parsing error
type Error struct {
	Message string
	Pos     int
}

func (e *Error) Error() string {
	return fmt.Sprintf("parse error at position %d: %s", e.Pos, e.Message)
}
