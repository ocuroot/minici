package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ocuroot/minici"
	"github.com/stretchr/testify/assert"
)

// mockCI implements the CI interface for testing
type mockCI struct {
	jobs      map[minici.JobID]*minici.Job
	nextJobID minici.JobID
}

func newMockCI() *mockCI {
	return &mockCI{
		jobs:      make(map[minici.JobID]*minici.Job),
		nextJobID: minici.JobID("job-1"),
	}
}

func (m *mockCI) ScheduleJob(repoURI string, commit string, command string) minici.JobID {
	jobID := m.nextJobID
	m.jobs[jobID] = &minici.Job{
		ID:      jobID,
		Status:  minici.JobStatusPending,
		RepoURI: repoURI,
		Commit:  commit,
		Command: command,
		Logs:    []string{"Job scheduled"},
	}

	// Simulate job execution
	go func() {
		job := m.jobs[jobID]
		job.Status = minici.JobStatusRunning
		job.Logs = append(job.Logs, "Job started")

		job.Status = minici.JobStatusSuccess
		job.Logs = append(job.Logs, "Job completed successfully")
	}()

	return jobID
}

func (m *mockCI) ListJobs() []minici.JobID {
	var jobIDs []minici.JobID
	for id := range m.jobs {
		jobIDs = append(jobIDs, id)
	}
	return jobIDs
}

func (m *mockCI) AllJobDetail() []minici.Job {
	var jobs []minici.Job
	for _, job := range m.jobs {
		jobs = append(jobs, *job)
	}
	return jobs
}

func (m *mockCI) JobDetail(jobID minici.JobID) minici.Job {
	if job, exists := m.jobs[jobID]; exists {
		return *job
	}
	return minici.Job{
		ID:     jobID,
		Status: minici.JobStatusFailure,
	}
}

func (m *mockCI) JobLogs(jobID minici.JobID) []string {
	if job, exists := m.jobs[jobID]; exists {
		return job.Logs
	}
	return []string{}
}

// createCompletedJob creates a job in completed state for testing
func (m *mockCI) createCompletedJob(jobID minici.JobID, repoURI, commit, command string) {
	m.jobs[jobID] = &minici.Job{
		ID:      jobID,
		Status:  minici.JobStatusSuccess,
		RepoURI: repoURI,
		Commit:  commit,
		Command: command,
		Logs:    []string{"Job scheduled", "Job started", "Job completed successfully"},
	}
}

func TestRESTServer(t *testing.T) {
	// Create a mock CI implementation
	ci := newMockCI()

	// Create the REST server with the mock CI
	restServer := NewRESTServer(ci, ":8080")

	t.Run("Schedule Job", func(t *testing.T) {
		// Create request body
		jobReq := JobRequest{
			RepoURI: "https://github.com/ocuroot/minici",
			Commit:  "main",
			Command: "go test ./...",
		}
		body, _ := json.Marshal(jobReq)

		// Create HTTP request
		req := httptest.NewRequest("POST", "/api/jobs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusCreated, rr.Code)

		var response JobResponse
		err := json.NewDecoder(rr.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, "job-1", response.ID)
	})

	t.Run("List Jobs", func(t *testing.T) {
		// Create HTTP request
		req := httptest.NewRequest("GET", "/api/jobs", nil)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		var response ListJobsResponse
		err := json.NewDecoder(rr.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Len(t, response.Jobs, 1)
	})

	t.Run("Job Status", func(t *testing.T) {
		// Create a completed job directly in the mock CI
		ci.createCompletedJob(minici.JobID("job-test-status"), "https://github.com/ocuroot/minici", "main", "go test ./...")

		// Create HTTP request
		req := httptest.NewRequest("GET", "/api/jobs/job-test-status", nil)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.server.Handler.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		var response JobResponse
		err := json.NewDecoder(rr.Body).Decode(&response)
		assert.NoError(t, err)

		// Verify specific job details from the mock CI
		assert.Equal(t, "job-test-status", response.ID)
		assert.Equal(t, "success", response.Status)
		assert.Equal(t, "https://github.com/ocuroot/minici", response.RepoURI)
		assert.Equal(t, "main", response.Commit)
		assert.Equal(t, "go test ./...", response.Command)
	})

	t.Run("Job Logs", func(t *testing.T) {
		// Create a completed job directly in the mock CI
		ci.createCompletedJob(minici.JobID("job-test-logs"), "https://github.com/ocuroot/minici", "main", "go test ./...")

		// Create HTTP request
		req := httptest.NewRequest("GET", "/api/jobs/job-test-logs/logs", nil)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.server.Handler.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		var response JobResponse
		err := json.NewDecoder(rr.Body).Decode(&response)
		assert.NoError(t, err)

		// Verify specific log messages from the mock CI
		assert.Len(t, response.Logs, 3, "Expected exactly 3 log messages")
		assert.Equal(t, "Job scheduled", response.Logs[0])
		assert.Equal(t, "Job started", response.Logs[1])
		assert.Equal(t, "Job completed successfully", response.Logs[2])

		// Verify the job ID is correct
		assert.Equal(t, "job-test-logs", response.ID)
	})

	t.Run("Job Status - Non-existent Job", func(t *testing.T) {
		// Create HTTP request for non-existent job
		req := httptest.NewRequest("GET", "/api/jobs/non-existent-job", nil)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.server.Handler.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		var response JobResponse
		err := json.NewDecoder(rr.Body).Decode(&response)
		assert.NoError(t, err)

		// Verify mock CI returns failure status for non-existent jobs
		assert.Equal(t, "non-existent-job", response.ID)
		assert.Equal(t, "failure", response.Status)
		assert.Empty(t, response.RepoURI)
		assert.Empty(t, response.Commit)
		assert.Empty(t, response.Command)
	})

	t.Run("Job Logs - Non-existent Job", func(t *testing.T) {
		// Create HTTP request for non-existent job
		req := httptest.NewRequest("GET", "/api/jobs/non-existent-job/logs", nil)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.server.Handler.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		var response JobResponse
		err := json.NewDecoder(rr.Body).Decode(&response)
		assert.NoError(t, err)

		// Verify mock CI returns empty logs for non-existent jobs
		assert.Equal(t, "non-existent-job", response.ID)
		assert.Empty(t, response.Logs)
	})
}

func TestInvalidRequests(t *testing.T) {
	// Create a mock CI implementation
	ci := newMockCI()

	// Create the REST server with the mock CI
	restServer := NewRESTServer(ci, ":8080")

	t.Run("Invalid Job Request", func(t *testing.T) {
		// Create invalid request body (missing required fields)
		jobReq := JobRequest{
			RepoURI: "", // Missing required field
			Commit:  "main",
			Command: "go test ./...",
		}
		body, _ := json.Marshal(jobReq)

		// Create HTTP request
		req := httptest.NewRequest("POST", "/api/jobs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Invalid Job ID", func(t *testing.T) {
		// Create HTTP request with non-existent job ID
		req := httptest.NewRequest("GET", "/api/jobs/nonexistent", nil)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		var response JobResponse
		err := json.NewDecoder(rr.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, "nonexistent", response.ID)
		assert.Equal(t, string(minici.JobStatusFailure), response.Status)
	})
}

func TestWait(t *testing.T) {
	// Create a mock CI implementation
	ci := newMockCI()

	// Create the REST server with the mock CI
	restServer := NewRESTServer(ci, ":8080")

	t.Run("Wait for Jobs", func(t *testing.T) {
		go func() {
			for i := 0; i < 10; i++ {
				ci.ScheduleJob("https://github.com/ocuroot/minici", "main", "go test ./...")
			}
		}()

		// Create HTTP request
		req := httptest.NewRequest("GET", "/api/wait", nil)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Handle request
		restServer.router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
