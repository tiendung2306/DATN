package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxJobLogLines = 240

type jobRunner struct {
	mu   sync.Mutex
	jobs map[string]*JobStatus
}

func newJobRunner() *jobRunner {
	return &jobRunner{jobs: make(map[string]*JobStatus)}
}

func (jr *jobRunner) snapshot() []JobStatus {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	out := make([]JobStatus, 0, len(jr.jobs))
	for _, job := range jr.jobs {
		if job == nil {
			continue
		}
		clone := *job
		clone.LogTail = append([]string(nil), job.LogTail...)
		out = append(out, clone)
	}
	sortJobs(out)
	return out
}

func (jr *jobRunner) get(jobID string) *JobStatus {
	jr.mu.Lock()
	defer jr.mu.Unlock()
	job := jr.jobs[jobID]
	if job == nil {
		return nil
	}
	clone := *job
	clone.LogTail = append([]string(nil), job.LogTail...)
	return &clone
}

func (jr *jobRunner) start(jobID, lane, kind, title string, emit func(JobStatus), fn func(*jobContext) error) error {
	jr.mu.Lock()
	if existing := jr.jobs[jobID]; existing != nil && existing.State == "running" {
		jr.mu.Unlock()
		return fmt.Errorf("%s is already running", title)
	}
	job := &JobStatus{
		ID:          jobID,
		Lane:        lane,
		Kind:        kind,
		Title:       title,
		State:       "running",
		StartedAtMs: time.Now().UnixMilli(),
	}
	jr.jobs[jobID] = job
	jr.mu.Unlock()

	if emit != nil {
		emit(*job)
	}

	go func() {
		ctx := &jobContext{runner: jr, jobID: jobID, emit: emit}
		err := fn(ctx)
		jr.mu.Lock()
		current := jr.jobs[jobID]
		if current != nil {
			if err != nil {
				current.State = "failed"
				current.Summary = err.Error()
				ctx.appendLocked(current, "ERROR: "+err.Error())
			} else {
				current.State = "success"
				if current.Summary == "" {
					current.Summary = "Completed successfully"
				}
			}
			current.EndedAtMs = time.Now().UnixMilli()
			snapshot := *current
			snapshot.LogTail = append([]string(nil), current.LogTail...)
			jr.mu.Unlock()
			if emit != nil {
				emit(snapshot)
			}
			return
		}
		jr.mu.Unlock()
	}()

	return nil
}

type jobContext struct {
	runner *jobRunner
	jobID  string
	emit   func(JobStatus)
}

func (jc *jobContext) append(line string) {
	if jc == nil || jc.runner == nil {
		return
	}
	jc.runner.mu.Lock()
	job := jc.runner.jobs[jc.jobID]
	if job == nil {
		jc.runner.mu.Unlock()
		return
	}
	jc.appendLocked(job, line)
	snapshot := *job
	snapshot.LogTail = append([]string(nil), job.LogTail...)
	jc.runner.mu.Unlock()
	if jc.emit != nil {
		jc.emit(snapshot)
	}
}

func (jc *jobContext) setSummary(summary string) {
	if jc == nil || jc.runner == nil {
		return
	}
	jc.runner.mu.Lock()
	job := jc.runner.jobs[jc.jobID]
	if job == nil {
		jc.runner.mu.Unlock()
		return
	}
	job.Summary = strings.TrimSpace(summary)
	snapshot := *job
	snapshot.LogTail = append([]string(nil), job.LogTail...)
	jc.runner.mu.Unlock()
	if jc.emit != nil {
		jc.emit(snapshot)
	}
}

func (jc *jobContext) runCommand(dir string, name string, args ...string) error {
	label := strings.TrimSpace(name + " " + strings.Join(args, " "))
	jc.append("$ " + label)

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go jc.copyStream(stdout, &wg)
	go jc.copyStream(stderr, &wg)
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func (jc *jobContext) copyStream(reader io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		jc.append(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		jc.append("stream error: " + err.Error())
	}
}

func (jc *jobContext) appendLocked(job *JobStatus, line string) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return
	}
	job.LogTail = append(job.LogTail, line)
	if len(job.LogTail) > maxJobLogLines {
		job.LogTail = append([]string(nil), job.LogTail[len(job.LogTail)-maxJobLogLines:]...)
	}
}

func sortJobs(jobs []JobStatus) {
	for i := 0; i < len(jobs)-1; i++ {
		for j := i + 1; j < len(jobs); j++ {
			if jobs[j].StartedAtMs > jobs[i].StartedAtMs {
				jobs[i], jobs[j] = jobs[j], jobs[i]
			}
		}
	}
}

func (a *App) emitJobUpdate(job JobStatus) {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "demo:job", job)
	}
}
