package interpreter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/amirhnajafiz/flak-dashboard/pkg/files"
)

// Interpreter reads the data chunks created by loader and
// converts them to human-readable format.
type Interpreter struct {
	// input/output path
	DataDirPath     string
	OutputFilePath  string
	ReferenceTSPath string

	// time references
	referenceWall int64
	referenceMono int64

	// input file
	fd *os.File

	// memory instances
	fdt *fdTable
	vm  *virtualMemoryAddressSpace

	// handlers map
	handlers map[string]syscallHandlerFunc
}

// initialize the interpreter variables.
func (i *Interpreter) initVars() {
	i.referenceMono = 0
	i.referenceWall = 0

	// read the reference timestamps from the given path
	data, err := os.ReadFile(i.ReferenceTSPath)
	if err == nil {
		type referenceTSData struct {
			RefWall float64 `json:"ref_wall"`
			RefMono float64 `json:"ref_mono"`
		}

		var rt referenceTSData
		if err := json.Unmarshal(data, &rt); err == nil {
			i.referenceWall = int64(rt.RefWall * 1e9)
			i.referenceMono = int64(rt.RefMono * 1e9)
		}
	}

	// create file descriptors table
	i.fdt = &fdTable{
		kv: make(map[string]map[int]string),
	}

	// create virtual memory address
	i.vm = &virtualMemoryAddressSpace{
		blocks: make(map[string]map[int][]int64),
	}

	// set the handlers
	i.handlers = map[string]syscallHandlerFunc{
		"open":            i.handleFdTableSyscall,
		"openat":          i.handleFdTableSyscall,
		"statfs":          i.handleFdTableSyscall,
		"statx":           i.handleFdTableSyscall,
		"newlstat":        i.handleFdTableSyscall,
		"newstat":         i.handleFdTableSyscall,
		"creat":           i.handleFdTableSyscall,
		"close":           i.handleFdTableSyscall,
		"dup":             i.handleUpdateFdSyscall,
		"dup2":            i.handleUpdateFdSyscall,
		"dup3":            i.handleUpdateFdSyscall,
		"read":            i.handleIOSyscall,
		"write":           i.handleIOSyscall,
		"readv":           i.handleIOSyscall,
		"writev":          i.handleIOSyscall,
		"pread64":         i.handleIOSyscall,
		"pwrite64":        i.handleIOSyscall,
		"preadv":          i.handleIOSyscall,
		"pwritev":         i.handleIOSyscall,
		"mmap":            i.handleAddressSpaceSyscall,
		"munmap":          i.handleAddressSpaceSyscall,
		"page_fault_user": i.handleMemorySyscall,
	}
}

// to datetime converts a nsecs to time instance.
func (i *Interpreter) toDatetime(sec int64) time.Time {
	eventWallNs := i.referenceWall + (sec - i.referenceMono)

	secs := eventWallNs / 1e9
	nsecs := eventWallNs % 1e9

	return time.Unix(secs, nsecs)
}

// process a given file.
func (i *Interpreter) process(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file `%s`: %v", path, err)
	}
	defer file.Close()

	// create scanner
	scanner := bufio.NewScanner(file)

	// scan the file
	for scanner.Scan() {
		// get each line
		line := scanner.Text()

		// split the line by the line delimeter
		parts := strings.Split(line, "####")
		if len(parts) < 4 {
			continue
		}

		// extract data
		ts := parts[0]
		proc := parts[1]
		event := parts[2]
		kvString := parts[3]

		// build the key-value data
		kv := make(map[string]string)
		for part := range strings.SplitSeq(kvString, "****") {
			if len(part) > 0 {
				data := strings.Split(part, "=")
				if len(data) == 2 {
					kv[strings.Trim(data[0], " ")] = strings.Trim(data[1], " ")
				}
			}
		}

		// call the handler based on the event
		if hd, ok := i.handlers[event]; ok {
			hd(ts, proc, event, kv)
		}
	}

	// check for scanner errors
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner failed: %v", err)
	}

	return nil
}

// Begin interpreting.
func (i *Interpreter) Begin() error {
	// call init vars
	i.initVars()

	// get the file names (aka chunks created by loader)
	names, err := files.GetFileNamesByWildcardMatch(i.DataDirPath, "*.out")
	if err != nil {
		return fmt.Errorf("failed to get file names: %v", err)
	}

	// sort the names to process them sequentially
	sort.Strings(names)

	// open the output file
	i.fd, err = os.Create(i.OutputFilePath)
	if err != nil {
		return fmt.Errorf("failed to open output file `%s`: %v", i.OutputFilePath, err)
	}

	// process each file in order
	for _, name := range names {
		if err := i.process(fmt.Sprintf("%s/%s", i.DataDirPath, name)); err != nil {
			return fmt.Errorf("failed processing `%s`: %v", name, err)
		}
	}

	return nil
}
