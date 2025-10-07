package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/davidcohan/port-authorizing/internal/approval"
	"github.com/gorilla/mux"
)

// handleApproveRequest handles approval of a pending request
func (s *Server) handleApproveRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	requestID := vars["request_id"]

	if requestID == "" {
		http.Error(w, "request_id is required", http.StatusBadRequest)
		return
	}

	// Get approver info (from token if authenticated, or from query param)
	approver := r.URL.Query().Get("approver")
	if approver == "" {
		// Try to get from auth token
		username, ok := r.Context().Value("username").(string)
		if ok {
			approver = username
		} else {
			approver = "unknown"
		}
	}

	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "approved via API"
	}

	// Submit approval
	err := s.approvalMgr.SubmitApproval(requestID, approval.DecisionApproved, approver, reason)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to approve request: %v", err), http.StatusBadRequest)
		return
	}

	// Return success page or JSON based on Accept header
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "approved",
			"request_id":  requestID,
			"approved_by": approver,
		})
	} else {
		// Return HTML success page
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Request Approved</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 600px; margin: 50px auto; text-align: center; }
        .success { color: #28a745; font-size: 48px; }
        .message { font-size: 18px; margin: 20px 0; }
        .details { background: #f8f9fa; padding: 20px; border-radius: 5px; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="success">✅</div>
    <h1>Request Approved</h1>
    <div class="message">The request has been successfully approved and will be executed.</div>
    <div class="details">
        <p><strong>Request ID:</strong> %s</p>
        <p><strong>Approved by:</strong> %s</p>
    </div>
</body>
</html>
`, requestID, approver)
	}
}

// handleRejectRequest handles rejection of a pending request
func (s *Server) handleRejectRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	requestID := vars["request_id"]

	if requestID == "" {
		http.Error(w, "request_id is required", http.StatusBadRequest)
		return
	}

	// Get approver info
	approver := r.URL.Query().Get("approver")
	if approver == "" {
		username, ok := r.Context().Value("username").(string)
		if ok {
			approver = username
		} else {
			approver = "unknown"
		}
	}

	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "rejected via API"
	}

	// Submit rejection
	err := s.approvalMgr.SubmitApproval(requestID, approval.DecisionRejected, approver, reason)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to reject request: %v", err), http.StatusBadRequest)
		return
	}

	// Return success page or JSON
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "rejected",
			"request_id":  requestID,
			"rejected_by": approver,
		})
	} else {
		// Return HTML success page
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Request Rejected</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 600px; margin: 50px auto; text-align: center; }
        .rejected { color: #dc3545; font-size: 48px; }
        .message { font-size: 18px; margin: 20px 0; }
        .details { background: #f8f9fa; padding: 20px; border-radius: 5px; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="rejected">❌</div>
    <h1>Request Rejected</h1>
    <div class="message">The request has been rejected and will not be executed.</div>
    <div class="details">
        <p><strong>Request ID:</strong> %s</p>
        <p><strong>Rejected by:</strong> %s</p>
    </div>
</body>
</html>
`, requestID, approver)
	}
}

// handleGetPendingApprovals returns list of pending approvals (for admin)
func (s *Server) handleGetPendingApprovals(w http.ResponseWriter, r *http.Request) {
	count := s.approvalMgr.GetPendingRequestsCount()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pending_count": count,
	})
}
