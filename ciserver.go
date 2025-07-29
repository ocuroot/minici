package minici

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/ocuroot/gittools"
	"github.com/oklog/ulid/v2"
)

type JobID string

func NewJobID() JobID {
	return JobID(ulid.Make().String())
}

type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusSuccess JobStatus = "success"
	JobStatusFailure JobStatus = "failure"
)

type CI interface {
	ScheduleJob(repoURI string, commit string, command string) JobID
	ListJobs() []JobID
	AllJobDetail() []Job
	JobDetail(jobID JobID) Job
	JobLogs(jobID JobID) []string
}

type Job struct {
	ID     JobID
	Status JobStatus

	RepoURI string
	Commit  string
	Command string
	Logs    []string
}

func NewCIServer() CI {
	return &CIServer{
		jobs: make(map[JobID]*Job),
	}
}

type CIServer struct {
	jobMutex sync.RWMutex

	jobs map[JobID]*Job
}

// executeCommand runs a command in the specified directory and captures its output.
// The command output is appended to the job's logs.
func executeCommand(command, dir string, job *Job) error {
	job.Logs = append(job.Logs, "Executing command: "+command)

	// Split the command string into the command and its arguments
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		job.Logs = append(job.Logs, "Error: empty command")
		return fmt.Errorf("empty command")
	}

	// Create the command
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	cmd.Dir = dir

	// Capture the combined output
	output, err := cmd.CombinedOutput()

	// Append the output to logs, line by line
	for _, line := range strings.Split(string(output), "\n") {
		if line != "" {
			job.Logs = append(job.Logs, "> "+line)
		}
	}

	if err != nil {
		job.Logs = append(job.Logs, "Command execution failed: "+err.Error())
		return err
	}

	job.Logs = append(job.Logs, "Command executed successfully")
	return nil
}

// cloneAndCheckout clones a repository and checks out a specific commit.
// It returns the path to the cloned repository and any error encountered.
// Progress and errors are logged to the job's logs.
func cloneAndCheckout(repoURI, commit string, job *Job) (string, error) {
	// Create a temporary directory for the job
	tempDir, err := os.MkdirTemp("", "ocuroot-ci-job-")
	if err != nil {
		job.Logs = append(job.Logs, "Failed to create temp directory: "+err.Error())
		return "", err
	}

	// Clone the repository
	job.Logs = append(job.Logs, "Cloning repository: "+repoURI)
	client := &gittools.Client{}
	_, err = client.Clone(repoURI, tempDir)
	if err != nil {
		job.Logs = append(job.Logs, "Failed to clone repository: "+err.Error())
		os.RemoveAll(tempDir)
		return "", err
	}

	// Open the repository
	repo, err := gittools.Open(tempDir)
	if err != nil {
		job.Logs = append(job.Logs, "Failed to open repository: "+err.Error())
		os.RemoveAll(tempDir)
		return "", err
	}

	// Checkout the specific commit
	job.Logs = append(job.Logs, "Checking out commit: "+commit)
	err = repo.Checkout(commit)
	if err != nil {
		job.Logs = append(job.Logs, "Failed to checkout commit: "+err.Error())
		os.RemoveAll(tempDir)
		return "", err
	}

	job.Logs = append(job.Logs, "Repository ready at "+tempDir)
	return tempDir, nil
}

func (s *CIServer) saveJob(job *Job) {
	s.jobMutex.Lock()
	defer s.jobMutex.Unlock()
	s.jobs[job.ID] = job
}

func (s *CIServer) ScheduleJob(repoURI string, commit string, command string) JobID {
	job := &Job{
		ID:      NewJobID(),
		Status:  JobStatusPending,
		RepoURI: repoURI,
		Commit:  commit,
		Command: command,
		Logs:    []string{},
	}
	s.saveJob(job)

	go func() {
		job.Status = JobStatusRunning
		job.Logs = append(job.Logs, "Starting job execution")

		// Clone the repository and checkout the commit
		tempDir, err := cloneAndCheckout(repoURI, commit, job)
		if err != nil {
			job.Status = JobStatusFailure
			return
		}
		defer os.RemoveAll(tempDir)

		// Repository is ready for job execution
		job.Logs = append(job.Logs, "Repository ready for job execution")

		// Execute the command in the cloned repository
		err = executeCommand(command, tempDir, job)
		if err != nil {
			job.Status = JobStatusFailure
			return
		}

		// At this point, the job completed successfully
		job.Status = JobStatusSuccess
	}()

	return job.ID
}

func (s *CIServer) ListJobs() []JobID {
	s.jobMutex.RLock()
	defer s.jobMutex.RUnlock()

	var jobIDs []JobID
	for jobID := range s.jobs {
		jobIDs = append(jobIDs, jobID)
	}

	return jobIDs
}

func (s *CIServer) AllJobDetail() []Job {
	s.jobMutex.RLock()
	defer s.jobMutex.RUnlock()

	var jobs []Job
	for _, job := range s.jobs {
		jobs = append(jobs, *job)
	}
	return jobs
}

func (s *CIServer) JobDetail(jobID JobID) Job {
	s.jobMutex.RLock()
	defer s.jobMutex.RUnlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return Job{
			ID:     jobID,
			Status: JobStatusFailure,
		}
	}
	return *job
}

func (s *CIServer) JobLogs(jobID JobID) []string {
	job, ok := s.jobs[jobID]
	if !ok {
		return []string{}
	}
	return job.Logs
}
