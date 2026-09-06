package hitl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

// ApprovalRequest is the HTTP payload: ToolDiffSummary + candidate pin.
type ApprovalRequest struct {
	ID        string               `json:"id"`
	Diff      core.ToolDiffSummary `json:"diff"`
	Candidate core.Pin             `json:"candidate"`
}

// DecisionBody is POST /v1/approvals/{id}/decide.
type DecisionBody struct {
	Decision string `json:"decision"` // approve | deny
	Who      string `json:"who"`
}

type pendingHTTP struct {
	req    ApprovalRequest
	result chan httpResult
}

type httpResult struct {
	approve bool
	who     string
}

// Server is a local HTTP + in-process Approver stub (no SaaS, no broker).
type Server struct {
	mu      sync.Mutex
	pending map[string]*pendingHTTP
	next    atomic.Uint64
	who     string
}

// NewServer constructs an empty local approval stub.
func NewServer() *Server {
	return &Server{pending: make(map[string]*pendingHTTP)}
}

// Actor implements ActorAware.
func (s *Server) Actor() string { return s.who }

// RequestApproval registers a pending request and blocks until decide or ctx done.
func (s *Server) RequestApproval(ctx context.Context, diff core.ToolDiffSummary, candidate core.Pin) (string, error) {
	id := strconv.FormatUint(s.next.Add(1), 10)
	p := &pendingHTTP{
		req: ApprovalRequest{
			ID:        id,
			Diff:      diff,
			Candidate: candidate,
		},
		result: make(chan httpResult, 1),
	}
	s.mu.Lock()
	s.pending[id] = p
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()

	select {
	case <-ctx.Done():
		s.who = ""
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ErrTimeout
		}
		return "", ErrTimeout
	case r := <-p.result:
		s.who = r.who
		if !r.approve {
			return "", ErrDenied
		}
		return candidate.Version, nil
	}
}

// Handler serves the local HITL stub.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /v1/approvals", s.handleCreate)
	mux.HandleFunc("GET /v1/approvals/pending", s.handlePending)
	mux.HandleFunc("POST /v1/approvals/{id}/decide", s.handleDecide)
	return mux
}

// handleCreate long-polls RequestApproval so the CLI stub can demo without a sidecar.
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Diff       core.ToolDiffSummary `json:"diff"`
		Candidate  core.Pin             `json:"candidate"`
		TimeoutSec int                  `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if req.TimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSec)*time.Second)
		defer cancel()
	}
	ver, err := s.RequestApproval(ctx, req.Diff, req.Candidate)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		code := "APPROVAL_DENIED"
		if errors.Is(err, ErrTimeout) {
			code = "APPROVAL_DENIED" // timeout == deny (Chief H)
			w.WriteHeader(http.StatusGatewayTimeout)
		} else {
			w.WriteHeader(http.StatusConflict)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"decision":    "deny",
			"reason_code": code,
			"error":       err.Error(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{
		"decision":         "approve",
		"reason_code":      string(core.ReasonOK),
		"approved_version": ver,
	})
}

func (s *Server) handlePending(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	list := make([]ApprovalRequest, 0, len(s.pending))
	for _, p := range s.pending {
		list = append(list, p.req)
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		// Go 1.22 PathValue; also accept trailing path for older mux patterns.
		id = strings.TrimPrefix(r.URL.Path, "/v1/approvals/")
		id = strings.TrimSuffix(id, "/decide")
	}
	var body DecisionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	approve := body.Decision == string(core.OutcomeApprove)
	if body.Decision != string(core.OutcomeApprove) && body.Decision != string(core.OutcomeDeny) {
		http.Error(w, "decision must be approve or deny", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	p, ok := s.pending[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "not pending", http.StatusNotFound)
		return
	}
	select {
	case p.result <- httpResult{approve: approve, who: body.Who}:
	default:
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "decision": body.Decision})
}

// WaitReady is a tiny helper for tests / CLI.
func WaitReady(ctx context.Context, base string) error {
	cli := &http.Client{Timeout: 200 * time.Millisecond}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/health", nil)
		if err != nil {
			return err
		}
		resp, err := cli.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
