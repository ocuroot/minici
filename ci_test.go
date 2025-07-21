package minici

import (
	"testing"
	"time"

	"github.com/ocuroot/gittools"
)

func TestCIServer(t *testing.T) {

	// Create a bare repository for testing
	barePath, cleanup, err := gittools.CreateTestRemoteRepo("ciserver_test")
	if err != nil {
		t.Fatal(err)
	}
	if false {
		// Clean up the bare repository when the test is done
		t.Cleanup(cleanup)
	}

	// Use the barePath as the remote repository URI
	t.Logf("Created bare repo at: %s", barePath)

	// Initialize a CI server for testing
	ci := NewCIServer()

	// Verify the CI server was created
	jobs := ci.ListJobs()
	if len(jobs) != 0 {
		t.Errorf("Expected new CI server to have no jobs, but found %d", len(jobs))
	}

	for i := 1; i <= 10; i++ {
		jobID := ci.ScheduleJob(barePath, "HEAD", "echo hello")
		if jobID == "" {
			t.Errorf("Expected job ID, but got empty string")
		}

		jobs = ci.ListJobs()
		if len(jobs) != i {
			t.Errorf("Expected %d jobs, but found %d", i+1, len(jobs))
		}

		job := ci.JobDetail(jobs[i-1])
		// Jobs start immediately in goroutines, so they should be running or pending
		if job.Status != JobStatusPending && job.Status != JobStatusRunning {
			t.Errorf("Expected job status to be pending or running, but found %s", job.Status)
		}

		logs := ci.JobLogs(jobs[i-1])
		// Jobs may have logs immediately as they start execution
		if logs == nil {
			t.Errorf("Expected job logs to be initialized, but got nil")
		}
	}

	timeout := time.After(10 * time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)

		jobs := ci.ListJobs()
		for {
			completeCount := 0
			for _, jobID := range jobs {
				job := ci.JobDetail(jobID)
				if job.Status == JobStatusSuccess || job.Status == JobStatusFailure {
					completeCount++
				}
			}
			if completeCount == len(jobs) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case <-timeout:
		t.Errorf("Timed out waiting for jobs to complete")
	case <-done:
	}

	for i := 1; i <= 10; i++ {
		job := ci.JobDetail(jobs[i-1])
		if job.Status != JobStatusSuccess {
			t.Errorf("Expected job status to be success, but found %s", job.Status)
		}
	}
}
