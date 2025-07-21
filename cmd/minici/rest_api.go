package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ocuroot/minici"
)

// RESTServer wraps a CI implementation and provides HTTP endpoints to interact with it
type RESTServer struct {
	ci      minici.CI
	router  *http.ServeMux
	server  *http.Server
	address string
}

// JobRequest represents the request body for scheduling a new CI job
type JobRequest struct {
	RepoURI string `json:"repo_uri"`
	Commit  string `json:"commit"`
	Command string `json:"command"`
}

// JobResponse represents the response for job-related operations
type JobResponse struct {
	ID     string   `json:"id"`
	Status string   `json:"status,omitempty"`
	Logs   []string `json:"logs,omitempty"`

	RepoURI string `json:"repo_uri"`
	Commit  string `json:"commit"`
	Command string `json:"command"`
}

// ListJobsResponse represents the response for listing jobs
type ListJobsResponse struct {
	Jobs []string `json:"jobs"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// NewRESTServer creates a new REST API server for CI operations
func NewRESTServer(ci minici.CI, address string) *RESTServer {
	router := http.NewServeMux()

	server := &RESTServer{
		ci:      ci,
		router:  router,
		address: address,
		server: &http.Server{
			Addr:         address,
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}

	// Register routes
	server.registerRoutes()

	return server
}

// registerRoutes sets up the HTTP endpoints
func (s *RESTServer) registerRoutes() {
	// API endpoints
	s.router.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleListJobs(w, r)
		case http.MethodPost:
			s.handleScheduleJob(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Job detail handler - handles both /api/jobs/<id> and /api/jobs/<id>/logs
	s.router.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Extract path components
		path := r.URL.Path
		pathSegments := strings.Split(strings.TrimRight(path, "/"), "/")

		// Path should be either /api/jobs/<id> or /api/jobs/<id>/logs
		if len(pathSegments) < 4 {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		jobID := pathSegments[3]

		// Check if this is a logs request
		if len(pathSegments) == 5 && pathSegments[4] == "logs" {
			s.handleJobLogs(w, r, jobID)
			return
		} else if len(pathSegments) == 4 {
			s.handleJobStatus(w, r, jobID)
			return
		}

		// If we get here, it's not a valid path
		w.WriteHeader(http.StatusNotFound)
	})
}

// Start begins serving HTTP requests
func (s *RESTServer) Start() error {
	fmt.Printf("REST API server starting on %s\n", s.address)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *RESTServer) Stop() error {
	return s.server.Close()
}

// handleScheduleJob processes requests to schedule a new CI job
func (s *RESTServer) handleScheduleJob(w http.ResponseWriter, r *http.Request) {
	var req JobRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.RepoURI == "" || req.Commit == "" || req.Command == "" {
		s.writeError(w, "Missing required fields: repo_uri, commit, and command are required", http.StatusBadRequest)
		return
	}

	jobID := s.ci.ScheduleJob(req.RepoURI, req.Commit, req.Command)

	s.writeJSON(w, JobResponse{
		ID: string(jobID),
	}, http.StatusCreated)
}

// handleListJobs processes requests to list all CI jobs
func (s *RESTServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobIDs := s.ci.ListJobs()

	// Convert JobIDs to strings
	jobs := make([]string, len(jobIDs))
	for i, id := range jobIDs {
		jobs[i] = string(id)
	}

	sort.Strings(jobs)

	s.writeJSON(w, ListJobsResponse{Jobs: jobs}, http.StatusOK)
}

// handleJobStatus processes requests to get a job's status
func (s *RESTServer) handleJobStatus(w http.ResponseWriter, r *http.Request, jobIDStr string) {
	jobID := minici.JobID(jobIDStr)

	detail := s.ci.JobDetail(jobID)

	s.writeJSON(w, JobResponse{
		ID:     string(jobID),
		Status: string(detail.Status),

		RepoURI: detail.RepoURI,
		Commit:  detail.Commit,
		Command: detail.Command,
	}, http.StatusOK)
}

// handleJobLogs processes requests to get a job's logs
func (s *RESTServer) handleJobLogs(w http.ResponseWriter, r *http.Request, jobIDStr string) {
	jobID := minici.JobID(jobIDStr)

	logs := s.ci.JobLogs(jobID)

	s.writeJSON(w, JobResponse{
		ID:   string(jobID),
		Logs: logs,
	}, http.StatusOK)
}

// writeJSON writes a JSON response with the given status code
func (s *RESTServer) writeJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// writeError writes an error response with the given message and status code
func (s *RESTServer) writeError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: message}); err != nil {
		http.Error(w, message, statusCode)
	}
}
