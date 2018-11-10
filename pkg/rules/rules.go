package rules

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/runes"
	"unicode"
)

// IsLiteral returns true if the rune is a Literal
func IsLiteral(rune rune) bool {
	return IsLetter(rune) || IsDigit(rune) || rune == runes.UNDERSCORE
}

// IsDigit returns true if the rune is a unicode Digit
func IsDigit(rune rune) bool {
	return unicode.IsDigit(rune)
}

// IsLetter returns true if the rune is a unicode letter
func IsLetter(rune rune) bool {
	return unicode.IsLetter(rune)
}
