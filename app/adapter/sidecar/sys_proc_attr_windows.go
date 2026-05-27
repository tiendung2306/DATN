//go:build windows

package sidecar

import (
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCreateJobObject          = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")

	jobObjectHandle uintptr
)

const (
	JobObjectExtendedLimitInformation  = 9
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x2000

	PROCESS_SET_QUOTA = 0x0100
	PROCESS_TERMINATE = 0x0001
)

type JOBOBJECT_BASIC_LIMIT_INFORMATION struct {
	ActiveProcessLimit uint32
	Affinity           uintptr
	PriorityClass      uint32
	SchedulingClass    uint32
	LimitFlags         uint32
}

type IO_COUNTERS struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type JOBOBJECT_EXTENDED_LIMIT_INFORMATION struct {
	BasicLimitInformation JOBOBJECT_BASIC_LIMIT_INFORMATION
	IoInfo                IO_COUNTERS
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryLimit uintptr
	PeakJobMemoryLimit    uintptr
}

func init() {
	h, _, _ := procCreateJobObject.Call(0, 0)
	if h != 0 {
		var info JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE

		size := uint32(unsafe.Sizeof(info))
		r, _, _ := procSetInformationJobObject.Call(
			h,
			JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uintptr(size),
		)
		if r != 0 {
			jobObjectHandle = h
		} else {
			syscall.CloseHandle(syscall.Handle(h))
		}
	}
}

// setSysProcAttr configures the process attributes to hide the console window on Windows.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// trackProcess registers the child process in a Windows Job Object to guarantee
// that it is killed when the Go parent process terminates.
func trackProcess(pid int) {
	if jobObjectHandle == 0 {
		return
	}
	h, err := syscall.OpenProcess(PROCESS_SET_QUOTA|PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(h)

	procAssignProcessToJobObject.Call(jobObjectHandle, uintptr(h))
}
