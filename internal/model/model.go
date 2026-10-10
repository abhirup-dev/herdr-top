package model

import "time"

// Value keeps unavailable process metrics distinct from legitimate zero values.
type Value struct {
	Value uint64 `json:"value,omitempty"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

type FloatValue struct {
	Value float64 `json:"value,omitempty"`
	Valid bool    `json:"valid"`
	Error string  `json:"error,omitempty"`
}

type Location struct {
	Session   string `json:"session"`
	Workspace string `json:"workspace,omitempty"`
	Tab       string `json:"tab,omitempty"`
	Pane      string `json:"pane,omitempty"`
	State     string `json:"state,omitempty"`
	CWD       string `json:"cwd,omitempty"`
}

// AgentRoot is the single launcher process that Herdr identifies for an agent pane.
type AgentRoot struct {
	PID      int32
	Kind     string
	Location Location
}

type Record struct {
	PID                int32         `json:"pid"`
	Kind               string        `json:"kind"`
	Age                time.Duration `json:"age_ns"`
	StartedAt          time.Time     `json:"started_at"`
	CPUPercent         FloatValue    `json:"cpu_percent"`
	RSSBytes           Value         `json:"rss_bytes"`
	FootprintBytes     Value         `json:"physical_footprint_bytes"`
	DiskReadBytes      Value         `json:"cumulative_disk_read_bytes"`
	DiskWriteBytes     Value         `json:"cumulative_disk_write_bytes"`
	TreeCPUPercent     FloatValue    `json:"tree_cpu_percent"`
	TreeRSSBytes       Value         `json:"tree_rss_bytes"`
	TreeFootprintBytes Value         `json:"tree_physical_footprint_bytes"`
	TreeDiskBytes      Value         `json:"tree_cumulative_disk_bytes"`
	Location           *Location     `json:"location,omitempty"`
	Diagnostics        []string      `json:"diagnostics,omitempty"`
}

type Snapshot struct {
	GeneratedAt  time.Time     `json:"generated_at"`
	SampleWindow time.Duration `json:"sample_window_ns"`
	Records      []Record      `json:"records"`
	Diagnostics  []string      `json:"diagnostics,omitempty"`
}
