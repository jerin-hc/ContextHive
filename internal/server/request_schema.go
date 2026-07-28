package server

type ContentRequest struct {
	// Git PR fields
	PRTitle       string `json:"pr_title,omitempty"`
	PRDescription string `json:"pr_description,omitempty"`
	PRDiff        string `json:"pr_diff,omitempty"`
	PRComments    string `json:"pr_comments,omitempty"`

	// Jira fields
	JiraIssueKey    string `json:"jira_issue_key,omitempty"`
	JiraSummary     string `json:"jira_summary,omitempty"`
	JiraDescription string `json:"jira_description,omitempty"`
	JiraComments    string `json:"jira_comments,omitempty"`

	// General message
	Message string `json:"message,omitempty"`
}

type QueryRequest struct {
	Text        string  `json:"text"`
	TopK        int     `json:"topK,omitempty"`
	MaxDistance float32 `json:"maxDistance,omitempty"`
}
