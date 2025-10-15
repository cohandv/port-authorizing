package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// WebhookPayload matches the payload sent by the approval system
type WebhookPayload struct {
	RequestID    string            `json:"request_id"`
	Username     string            `json:"username"`
	ConnectionID string            `json:"connection_id"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Body         string            `json:"body,omitempty"`
	RequestedAt  string            `json:"requested_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	ApprovalURL  string            `json:"approval_url"`
}

var (
	port         = flag.Int("port", 9000, "Port to listen on")
	apiURL       = flag.String("api-url", "http://localhost:8080", "Port authorizing API URL")
	autoApprove  = flag.Bool("auto-approve", true, "Automatically approve all requests")
	interactive  = flag.Bool("interactive", false, "Interactive mode - prompt for each approval")
	approveDelay = flag.Duration("delay", 0, "Delay before approving (e.g., 2s, 1m)")
	approverName = flag.String("approver", "mock-server", "Name of the approver")
	verbose      = flag.Bool("verbose", true, "Verbose logging")
)

func main() {
	flag.Parse()

	// Validate conflicting flags
	if *interactive && *autoApprove {
		log.Println("⚠️  Both -interactive and -auto-approve are set. Disabling auto-approve for interactive mode.")
		*autoApprove = false
	}

	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/health", handleHealth)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("🚀 Mock Approval Server started on %s", addr)
	log.Printf("📡 API URL: %s", *apiURL)

	if *interactive {
		log.Printf("🎮 Interactive mode: ENABLED")
		log.Printf("   Type 'approve' or 'reject' for each request")
	} else {
		log.Printf("✅ Auto-approve: %v", *autoApprove)
	}

	if *approveDelay > 0 {
		log.Printf("⏱️  Approval delay: %v", *approveDelay)
	}
	log.Printf("👤 Approver name: %s", *approverName)
	log.Println()
	log.Println("Waiting for approval requests...")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse webhook payload
	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("❌ Failed to parse webhook payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Log the approval request
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📥 Approval Request Received")
	log.Printf("   Request ID:    %s", payload.RequestID)
	log.Printf("   User:          %s", payload.Username)
	log.Printf("   Connection:    %s", payload.ConnectionID)
	log.Printf("   Method:        %s", payload.Method)
	log.Printf("   Path:          %s", payload.Path)
	log.Printf("   Requested At:  %s", payload.RequestedAt)
	if len(payload.Metadata) > 0 {
		log.Printf("   Metadata:")
		for k, v := range payload.Metadata {
			log.Printf("     %s: %s", k, v)
		}
	}

	// Respond to webhook call immediately
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "received",
		"message": "Approval request received",
	})

	// Handle approval based on mode
	if *interactive {
		// Interactive mode - prompt user
		go promptForApproval(payload)
	} else if *autoApprove {
		// Auto-approve mode
		if *approveDelay > 0 {
			log.Printf("⏳ Waiting %v before approving...", *approveDelay)
			time.Sleep(*approveDelay)
		}
		go approveRequest(payload.RequestID, *approverName, "auto-approved")
	} else {
		// Manual mode - just log URLs
		log.Println("⏸️  Auto-approve disabled. Manual approval required.")
		log.Printf("   Approve URL: %s%s/approve", *apiURL, payload.ApprovalURL)
		log.Printf("   Reject URL:  %s%s/reject", *apiURL, payload.ApprovalURL)
	}

	if !*interactive {
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Println()
	}
}

func approveRequest(requestID, approver, reason string) {
	approvalURL := fmt.Sprintf("%s/api/approvals/%s/approve?approver=%s&reason=%s",
		*apiURL, requestID, approver, reason)

	if *verbose {
		log.Printf("🔄 Sending approval to: %s", approvalURL)
	}

	resp, err := http.Post(approvalURL, "application/json", nil)
	if err != nil {
		log.Printf("❌ Failed to approve request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("✅ Request %s APPROVED by %s", requestID, approver)
	} else {
		log.Printf("❌ Failed to approve request %s: HTTP %d", requestID, resp.StatusCode)
	}
}

func rejectRequest(requestID, approver, reason string) {
	rejectURL := fmt.Sprintf("%s/api/approvals/%s/reject?approver=%s&reason=%s",
		*apiURL, requestID, approver, reason)

	if *verbose {
		log.Printf("🔄 Sending rejection to: %s", rejectURL)
	}

	resp, err := http.Post(rejectURL, "application/json", nil)
	if err != nil {
		log.Printf("❌ Failed to reject request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("❌ Request %s REJECTED by %s", requestID, approver)
	} else {
		log.Printf("❌ Failed to reject request %s: HTTP %d", requestID, resp.StatusCode)
	}
}

func promptForApproval(payload WebhookPayload) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\n")
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("❓ Decision required for request %s\n", payload.RequestID)
		fmt.Printf("   %s %s by %s\n", payload.Method, payload.Path, payload.Username)
		fmt.Printf("\n")
		fmt.Printf("   Type your decision:\n")
		fmt.Printf("   • 'approve' or 'a' - Approve this request\n")
		fmt.Printf("   • 'reject' or 'r'  - Reject this request\n")
		fmt.Printf("   • 'skip' or 's'    - Skip (timeout)\n")
		fmt.Printf("\n")
		fmt.Printf("👉 Decision: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("❌ Error reading input: %v", err)
			return
		}

		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "approve", "a", "yes", "y":
			approveRequest(payload.RequestID, *approverName, "approved-interactively")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
			return

		case "reject", "r", "no", "n":
			rejectRequest(payload.RequestID, *approverName, "rejected-interactively")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
			return

		case "skip", "s":
			log.Printf("⏭️  Skipped - request will timeout")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
			return

		default:
			fmt.Printf("❌ Invalid input '%s'. Please type 'approve', 'reject', or 'skip'\n", input)
		}
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"service":      "mock-approval-server",
		"auto_approve": *autoApprove,
	})
}
