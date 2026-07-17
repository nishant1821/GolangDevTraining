package auth

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtSecret — Node: process.env.JWT_SECRET jaisa hi, bas yahan var init ke time hi padh lete hain.
// production mein isko hamesha env var se aana chahiye — abhi ke liye dev fallback rakha hai.
var jwtSecret = []byte(getSecret())

func getSecret() string {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return s
	}
	return "dev-secret-change-me" // TODO: sirf local dev ke liye, kabhi commit mat karna production secret
}

// Claims — jwt.sign(payload, secret) ke "payload" wale part ka Go equivalent.
// jwt.RegisteredClaims embed karne se standard fields (exp, iat) built-in mil jaate hain —
// Node mein tu jwt.sign(payload, secret, { expiresIn: "24h" }) se ye manually option deta tha,
// yahan struct field ke through explicit set karna padta hai.
type Claims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken — login successful hone ke baad token banane ke liye.
// Node: jwt.sign({ userId }, JWT_SECRET, { expiresIn: "24h" })
func GenerateToken(userID uint) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// YOUR TURN: ParseToken likh — middleware isko call karega har protected request pe.
// Node: jwt.verify(token, JWT_SECRET) → payload return karta hai, ya throw karta hai agar invalid/expired
//
// Go mein pattern thoda alag hai:
//
//	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
//	    return jwtSecret, nil   // ye callback bata raha hai "kaunsa secret use karke verify karo"
//	})
//	if err != nil { return nil, err }
//
//	claims, ok := token.Claims.(*Claims)   // type assertion — interface{} se wapas *Claims nikaalna
//	if !ok || !token.Valid {
//	    return nil, errors.New("invalid token")
//	}
//	return claims, nil
//
// self-check: agar token expire ho chuka hai, ParseWithClaims kya karega — err return karega
// ya token.Valid == false set karega? (dono ho sakte hain, isliye upar dono check ho rahe hain)
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
