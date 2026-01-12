package githubactions

import "time"

// WorkflowRun represents a GitHub Actions workflow run
type WorkflowRun struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	RunNumber    int       `json:"run_number"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	HTMLURL      string    `json:"html_url"`
	HeadBranch   string    `json:"head_branch"`
	HeadSHA      string    `json:"head_sha"`
	Event        string    `json:"event"` // push, pull_request, schedule, workflow_dispatch
	CreatedAt    time.Time `json:"created_at"`
	RunStartedAt time.Time `json:"run_started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// WorkflowJob represents a job within a workflow run
type WorkflowJob struct {
	ID         int64     `json:"id"`
	RunID      int64     `json:"run_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	StartedAt  time.Time `json:"started_at"`
	Steps      []Step    `json:"steps"`
}

// Step represents a step within a job
type Step struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Number     int    `json:"number"`
}

// Artifact represents a workflow artifact
type Artifact struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	SizeInBytes        int64     `json:"size_in_bytes"`
	ArchiveDownloadURL string    `json:"archive_download_url"`
	Expired            bool      `json:"expired"`
	CreatedAt          time.Time `json:"created_at"`
}

// WorkflowJobsResponse is the API response for listing jobs
type WorkflowJobsResponse struct {
	TotalCount int           `json:"total_count"`
	Jobs       []WorkflowJob `json:"jobs"`
}

// ArtifactsResponse is the API response for listing artifacts
type ArtifactsResponse struct {
	TotalCount int        `json:"total_count"`
	Artifacts  []Artifact `json:"artifacts"`
}

// Commit represents a GitHub commit with file changes
type Commit struct {
	SHA     string       `json:"sha"`
	Message string       `json:"message"`
	Author  CommitAuthor `json:"author"`
	Files   []CommitFile `json:"files"`
}

// CommitAuthor represents the author of a commit
type CommitAuthor struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// CommitFile represents a file changed in a commit
type CommitFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"` // added, removed, modified, renamed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch,omitempty"` // The actual diff content
}
