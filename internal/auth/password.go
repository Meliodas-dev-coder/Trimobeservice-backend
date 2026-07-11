package auth

import "golang.org/x/crypto/bcrypt"

// dummyHash is a valid bcrypt hash (of a random value) at the same cost as real
// password hashes. It is used to spend an equivalent amount of time when a login
// targets a non-existent account, closing the user-enumeration timing channel.
var dummyHash = func() string {
	h, err := bcrypt.GenerateFromPassword([]byte("timing-equalizer-not-a-real-password"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return string(h)
}()

func hashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func checkPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
