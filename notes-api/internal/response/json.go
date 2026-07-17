// internal/response/json.go
package response

import (
	"encoding/json"
	"net/http"
)

// Capital W — taaki dusre packages use kar sakein (exported)
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}
