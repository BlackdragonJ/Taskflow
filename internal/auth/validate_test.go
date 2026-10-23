package auth

import "testing"

func TestValidEmail(t *testing.T) {
	valid := []string{"ana@correo.com", "j.perez+qa@universidad.edu.gt", "x_1@dominio.io"}
	invalid := []string{"", "ana", "ana@", "@correo.com", "ana@correo", "ana correo@x.com",
		"Ana <ana@correo.com>", "ana@@correo.com"}

	for _, e := range valid {
		if !ValidEmail(e) {
			t.Errorf("%q debería ser válido", e)
		}
	}
	for _, e := range invalid {
		if ValidEmail(e) {
			t.Errorf("%q debería ser inválido", e)
		}
	}
}

func TestPasswordProblems(t *testing.T) {
	cases := map[string]int{
		"Segura#2026":  0,
		"corta1!":      2, // longitud y mayúscula
		"sinnumeros!":  2, // mayúscula y número
		"SOLOMAYUS1!":  1, // minúscula
		"Sinespecial1": 1,
		"":             5,
	}
	for pw, want := range cases {
		if got := PasswordProblems(pw); len(got) != want {
			t.Errorf("PasswordProblems(%q) = %v, want %d problemas", pw, got, want)
		}
	}
}
