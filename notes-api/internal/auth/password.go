package auth

import "golang.org/x/crypto/bcrypt"

// Node: bcrypt.hash(password, saltRounds) — yahan "cost" wahi saltRounds hai
// Python: bcrypt.hashpw(password.encode(), bcrypt.gensalt())
//
// HashPassword — plain password ko hash mein convert karta hai, register ke time use hoga.
func HashPassword(plain string) (string, error) {
	// bcrypt.DefaultCost == 10, same default jo Node ki bcrypt library mein hota hai
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// YOUR TURN: ComparePassword likh — login ke time use hoga.
// Node: await bcrypt.compare(plainPassword, hashedPassword) → boolean return karta hai
// Go mein alag hai: bcrypt.CompareHashAndPassword(hash, plain []byte) error return karta hai
//   - error == nil → password sahi hai
//   - error != nil (ErrMismatchedHashAndPassword) → password galat hai
//
// GOTCHA: argument order Node se ulta hai — Go mein PEHLE hash, BAAD mein plain password.
// (bcrypt.compare(plain, hash) — Node; bcrypt.CompareHashAndPassword(hash, plain) — Go)
//
// func signature already di hai, bas body likh:
func ComparePassword(hashed, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
}
