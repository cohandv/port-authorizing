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
		username, ok := r.Context().Value(ContextKeyUsername).(string)
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "approved",
			"request_id":  requestID,
			"approved_by": approver,
		})
	} else {
		// Return HTML success page
		w.Header().Set("Content-Type", "text/html")
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Request Approved</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        }
        .container {
            background: white;
            padding: 3rem;
            border-radius: 1rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            text-align: center;
        }
        h1 { color: #667eea; margin: 0 0 1rem 0; }
        .success { font-size: 4rem; color: #2ecc71; margin-bottom: 1rem; }
        .message { font-size: 1rem; color: #555; margin: 1rem 0; }
        .details { 
            background: #f8f9fa; 
            padding: 1rem; 
            border-radius: 0.5rem; 
            margin: 1.5rem 0;
            font-size: 0.9rem;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="success">&#10004;</div>
        <h1>Request Approved</h1>
        <p class="message">The request has been successfully approved and will be executed.</p>
        <div class="details">
            <p><strong>Request ID:</strong> ` + requestID + `</p>
        </div>
        <p>You can close this window and return to your work.</p>
    </div>
    <script>setTimeout(() => window.close(), 3000);</script>
</body>
</html>`
		_, _ = w.Write([]byte(html))
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
		username, ok := r.Context().Value(ContextKeyUsername).(string)
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "rejected",
			"request_id":  requestID,
			"rejected_by": approver,
		})
	} else {
		// Return HTML success page
		w.Header().Set("Content-Type", "text/html")
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Request Rejected</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        }
        .container {
            background: white;
            padding: 3rem;
            border-radius: 1rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            text-align: center;
        }
        h1 { color: #dc3545; margin: 0 0 1rem 0; }
        .rejected { font-size: 4rem; color: #dc3545; margin-bottom: 1rem; }
        .message { font-size: 1rem; color: #555; margin: 1rem 0; }
        .details { 
            background: #f8f9fa; 
            padding: 1rem; 
            border-radius: 0.5rem; 
            margin: 1.5rem 0;
            font-size: 0.9rem;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="rejected">&#10008;</div>
        <h1>Request Rejected</h1>
        <p class="message">The request has been rejected and will not be executed.</p>
        <div class="details">
            <p><strong>Request ID:</strong> ` + requestID + `</p>
        </div>
        <p>You can close this window and return to your work.</p>
    </div>
    <script>setTimeout(() => window.close(), 3000);</script>
</body>
</html>`
		_, _ = w.Write([]byte(html))
	}
}

// handleGetPendingApprovals returns list of pending approvals (for admin)
func (s *Server) handleGetPendingApprovals(w http.ResponseWriter, r *http.Request) {
	count := s.approvalMgr.GetPendingRequestsCount()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"pending_count": count,
	})
}
