package auth

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode"
)

// net/mail acepta cosas como "Juan <juan@x.com>", por eso se valida también con regex
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func ValidEmail(email string) bool {
	if len(email) > 150 || !emailRe.MatchString(email) {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

// PasswordProblems devuelve qué le falta a la contraseña; vacío = es válida.
func PasswordProblems(pw string) []string {
	var problems []string
	if len(pw) < 8 {
		problems = append(problems, "mínimo 8 caracteres")
	}
	if len(pw) > 72 { // límite de bcrypt
		problems = append(problems, "máximo 72 caracteres")
	}

	var upper, lower, digit, special bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			special = true
		}
	}
	if !upper {
		problems = append(problems, "una mayúscula")
	}
	if !lower {
		problems = append(problems, "una minúscula")
	}
	if !digit {
		problems = append(problems, "un número")
	}
	if !special {
		problems = append(problems, "un carácter especial")
	}
	return problems
}

func passwordError(pw string) string {
	p := PasswordProblems(pw)
	if len(p) == 0 {
		return ""
	}
	return "La contraseña requiere: " + strings.Join(p, ", ")
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
