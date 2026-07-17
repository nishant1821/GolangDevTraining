package auth

import (
	"context"
	"net/http"
	"strings"

	"notes-api/internal/response"
)

// contextKey — apna type banaya taaki context.WithValue mein key collision na ho
// (agar plain string "userID" use karte aur kal koi aur package bhi "userID" string key use kare
// toh dono clash kar sakte the — custom type ye guarantee deta hai ki sirf ye package hi is key ko access kare)
type contextKey string

const userIDKey contextKey = "userID"

// GetUserID — handler ke andar context se userID nikaalne ke liye.
// Node: middleware ne req.userId set kiya tha, handler seedha req.userId padh leta tha.
// Go mein request object immutable hai, isliye value context ke through pass hoti hai —
// isi liye middleware ek naya request banata hai (r.WithContext) jisme ye value chhupi hoti hai.
func GetUserID(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value(userIDKey).(uint)
	return id, ok
}

// YOUR TURN: RequireAuth likh — ye chi middleware hai, protected routes ke aage r.Use(auth.RequireAuth) laga denge.
//
// Node:
//
//	function authMiddleware(req, res, next) {
//	  const token = req.headers.authorization?.split(" ")[1]
//	  try {
//	    req.userId = jwt.verify(token, SECRET).userId
//	    next()
//	  } catch {
//	    res.status(401).json({ error: "unauthorized" })
//	  }
//	}
//
// Go mein middleware signature hamesha aisi hoti hai: func(next http.Handler) http.Handler
// (ek handler leta hai, naya handler wrap karke return karta hai)
//
// steps:
//  1. header padh: authHeader := r.Header.Get("Authorization")
//  2. "Bearer " prefix check/strip kar: strings.TrimPrefix(authHeader, "Bearer ")
//     (agar header hi khali hai ya prefix match nahi karta → seedha unauthorized bhej de)
//  3. ParseToken(tokenString) call kar
//  4. error → response.Error(w, http.StatusUnauthorized, "unauthorized"); return (next.ServeHTTP MAT karna!)
//  5. success → naya context banao: ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
//     naya request banao: r.WithContext(ctx)
//     next.ServeHTTP(w, <naya request>) call karo
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. header padho
		authHeader := r.Header.Get("Authorization")

		// 2. "Bearer " prefix check + strip
		//    HasPrefix se pehle verify — warna khali/galat header pe bhi TrimPrefix
		//    chup-chaap kuch return kar dega aur galat token ParseToken tak chala jaayega
		if !strings.HasPrefix(authHeader, "Bearer ") {
			response.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// 3 + 4. parse karo, fail → 401 aur YAHIN ruk jao (next call MAT karo)
		claims, err := ParseToken(tokenString)
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// 5. userID context mein daalo, naya request banao, chain aage badhao
		ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
