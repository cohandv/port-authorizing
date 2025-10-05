package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRespondError_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
	}{
		{
			name:       "400 Bad Request",
			statusCode: http.StatusBadRequest,
			message:    "Bad request error",
		},
		{
			name:       "401 Unauthorized",
			statusCode: http.StatusUnauthorized,
			message:    "Unauthorized access",
		},
		{
			name:       "403 Forbidden",
			statusCode: http.StatusForbidden,
			message:    "Forbidden resource",
		},
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			message:    "Resource not found",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			message:    "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			respondError(w, tt.statusCode, tt.message)

			if w.Code != tt.statusCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.statusCode)
			}

			var response map[string]string
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response["error"] != tt.message {
				t.Errorf("error message = %s, want %s", response["error"], tt.message)
			}

			// Check content type
			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %s, want application/json", contentType)
			}
		})
	}
}

func TestRespondJSON_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		data       interface{}
	}{
		{
			name:       "200 OK with map",
			statusCode: http.StatusOK,
			data: map[string]string{
				"status": "success",
			},
		},
		{
			name:       "201 Created",
			statusCode: http.StatusCreated,
			data: map[string]interface{}{
				"id":   "123",
				"name": "test",
			},
		},
		{
			name:       "204 No Content",
			statusCode: http.StatusNoContent,
			data:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			respondJSON(w, tt.statusCode, tt.data)

			if w.Code != tt.statusCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.statusCode)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %s, want application/json", contentType)
			}
		})
	}
}

func TestRespondJSON_ComplexData(t *testing.T) {
	w := httptest.NewRecorder()

	data := map[string]interface{}{
		"users": []map[string]interface{}{
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		},
		"total": 2,
		"metadata": map[string]string{
			"page": "1",
		},
	}

	respondJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", response["total"])
	}

	users := response["users"].([]interface{})
	if len(users) != 2 {
		t.Errorf("users count = %d, want 2", len(users))
	}
}

func TestRespondError_EmptyMessage(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "")

	var response map[string]string
	json.NewDecoder(w.Body).Decode(&response)

	if response["error"] != "" {
		t.Errorf("Empty error message should be preserved, got %s", response["error"])
	}
}

func TestRespondJSON_EmptyData(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]interface{}{})

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response) != 0 {
		t.Errorf("Empty data should result in empty object, got %v", response)
	}
}

func BenchmarkRespondError(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		respondError(w, http.StatusBadRequest, "test error")
	}
}

func BenchmarkRespondJSON_Small(b *testing.B) {
	data := map[string]string{"status": "success"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		respondJSON(w, http.StatusOK, data)
	}
}

func BenchmarkRespondJSON_Large(b *testing.B) {
	data := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		data[string(rune(i))] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		respondJSON(w, http.StatusOK, data)
	}
}
