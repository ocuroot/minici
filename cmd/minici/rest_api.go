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
			WriteTimeout: 5 * time.Minute,
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

	s.router.HandleFunc("/api/wait", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleWait(w, r)
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

// handleWait blocks until all jobs are complete, returning 200 if all succeeded or 500 if any failed
// If no jobs are scheduled after 30s, returns 204 No Content.
// Times out 5 minutes after this request or the start of the first job, whichever is later.
func (s *RESTServer) handleWait(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	startTime := time.Now()

	fDone := make(chan struct{})

	defer func() {
		close(fDone)
	}()
	go func() {
		select {
		case <-fDone:
			fmt.Println("Function completed first")
		case <-r.Context().Done():
			fmt.Println("Context completed first")
		}
		fmt.Println("Took", time.Since(startTime))
	}()

	fmt.Println("REST server: Wait")

	// Wait up to 30s for at least one job to have started
	start := time.Now()
	for time.Since(start) < 30*time.Second {
		jobIDs := s.ci.ListJobs()
		if len(jobIDs) > 0 {
			fmt.Printf("We have %d jobs\n", len(jobIDs))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(s.ci.ListJobs()) == 0 {
		fmt.Println("No jobs scheduled")
		s.writeJSONNoContentType(w, "no jobs scheduled", http.StatusNoContent)
		return
	}

	// Wait up to 5 minutes for all jobs to complete
	start = time.Now()
	for {
		if time.Since(start) > 5*time.Minute {
			fmt.Println("Timeout waiting for jobs to complete")
			s.writeJSONNoContentType(w, "timeout waiting for jobs to complete", http.StatusRequestTimeout)
			return
		}
		allDone := true
		detail := s.ci.AllJobDetail()
		for _, job := range detail {
			if job.Status == minici.JobStatusPending || job.Status == minici.JobStatusRunning {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	for _, job := range s.ci.AllJobDetail() {
		if job.Status != minici.JobStatusSuccess {
			fmt.Printf("Job %s failed with status %s\n", job.ID, job.Status)
			s.writeJSONNoContentType(w, "one or more jobs failed", http.StatusInternalServerError)
			return
		}
	}

	fmt.Println("all complete")
	s.writeJSONNoContentType(w, "all jobs completed successfully", http.StatusOK)
}

// writeJSON writes a JSON response with the given status code
func (s *RESTServer) writeJSONNoContentType(w http.ResponseWriter, data interface{}, statusCode int) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
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
