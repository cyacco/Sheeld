package response

import (
	"encoding/json"
	"net/http"
)

// JSON writes a JSON response with the given status code and payload.
func JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		json.NewEncoder(w).Encode(payload)
	}
}

// Error writes a JSON error response.
func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

// ValidationError writes a JSON error response for validation failures.
func ValidationError(w http.ResponseWriter, field string, message string) {
	JSON(w, http.StatusBadRequest, map[string]interface{}{
		"error":   "validation_error",
		"field":   field,
		"message": message,
	})
}
