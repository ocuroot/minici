# minici

[![GoDoc](https://pkg.go.dev/badge/github.com/ocuroot/minici)](https://pkg.go.dev/github.com/ocuroot/minici)

A tiny, git-integrated CI server that can be started locally to test workflows.
This is currently used to test Ocuroot end-to-end.

## Concepts

minici executes "jobs" on request. A job is defined by a command run on a particular commit in a git repository.

Jobs are executed concurrently in background goroutines. The status of every job executed since minici was started is
retained in memory, along with a slice of logs.

minici can be run as a standalone process with a REST server, or as a library that can be integrated directly into Go tests.

# Running as a library

You can import minici as a library and use it in your own Go code in a test or part of another service.

Get the library with this command:

```
go get github.com/ocuroot/minici
```

You can then create a new CI server and schedule a job with the following code:

```go
ciServer := minici.NewCIServer()
jobID := ciServer.ScheduleJob("https://github.com/ocuroot/minici", "main", "go test ./...")
fmt.Println("Scheduled job with ID:", jobID)
```

See the godoc for more information: https://pkg.go.dev/github.com/ocuroot/minici

# Running as a server

A server can be started locally with the following command:

```
go run github.com/ocuroot/minici/cmd/minici@latest --port 8080
```

## REST API

The API is available at `/api`. So in the example above it would be available at `http://localhost:8080/api`.

### Schedule a job

To schedule a new job, run the following command:

```
curl -X POST http://localhost:8080/api/jobs -H "Content-Type: application/json" -d '{"repo_uri": "https://github.com/ocuroot/minici", "commit": "main", "command": "go test ./..."}'
```

This will return a JSON object containing the job ID as a ULID:

```json
{
    "id": "01GZM9XJN00000000000000000"
}
```

### List jobs

To list all jobs, run the following command:

```
curl http://localhost:8080/api/jobs
```

This will return a JSON object containing the job IDs as an array:

```json
{
    "jobs": [
        "01GZM9XJN00000000000000000",
        "01GZM9XJN00000000000000001"
    ]
}
```

### Get job status

To get the status of a job, use the /api/jobs/<id> endpoint:

```
curl http://localhost:8080/api/jobs/01GZM9XJN00000000000000000
```

This will return a JSON object containing the job ID, status, and job configuration:

```json
{
    "id": "01K0Q8PQSN6YQSYNEGYCE80ES5",
    "status": "success",
    "repo_uri": "https://github.com/ocuroot/minici",
    "commit": "main",
    "command": "go test ./..."
}
```

### Get job logs

To get the logs of a job, use the /api/jobs/<id>/logs endpoint:

```
curl http://localhost:8080/api/jobs/01GZM9XJN00000000000000000/logs
```

This will return a JSON object containing the job ID and a slice of logs:

```json
{
    "id": "01GZM9XJN00000000000000000",
    "logs": [
        "Job scheduled",
        "Job started",
        "Job completed successfully"
    ]
}
```
