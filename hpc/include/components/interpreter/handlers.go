package interpreter

import (
	"fmt"
	"strconv"
	"time"
)

// system-call handler function type
type syscallHandlerFunc func(string, string, string, map[string]string)

// handling all read/write system calls.
func (i *Interpreter) handleIOSyscall(ts, proc, operand string, kv map[string]string) {
	// ignore failed calls
	ret, _ := strconv.Atoi(kv["ret"])
	if ret < 0 {
		return
	}

	// extract data
	fd, _ := strconv.Atoi(kv["fd"])
	fname := i.fdt.search(proc, fd)

	if len(fname) == 0 {
		fname = fmt.Sprintf("unknown (fd=%d)", fd)
	}

	// convert ts to datetime
	tsInt64, _ := strconv.ParseInt(ts, 10, 64)
	timeDuration, _ := strconv.ParseInt(kv["diff"], 10, 64)

	startDate := i.toDatetime(tsInt64)
	endDate := i.toDatetime(tsInt64 + timeDuration)

	// write into the output file
	fmt.Fprintf(
		i.fd,
		"%s - %d [%s : %s] %s bytes from file: %s\n",
		startDate.Format(time.RFC3339Nano),
		endDate.Sub(startDate).Nanoseconds(),
		proc,
		operand,
		kv["count"],
		fname,
	)
}

// handling all system calls that add or remove entries from fd table.
func (i *Interpreter) handleFdTableSyscall(ts, proc, operand string, kv map[string]string) {
	// ignore failed calls
	ret, _ := strconv.Atoi(kv["ret"])
	if ret < 0 {
		return
	}

	// handle close separatly
	if operand == "close" {
		fd, _ := strconv.Atoi(kv["fd"])
		i.fdt.remove(proc, fd)
	} else {
		i.fdt.put(proc, ret, kv["fname"])
	}
}

// handling all system calls that update the fd table.
func (i *Interpreter) handleUpdateFdSyscall(ts, proc, operand string, kv map[string]string) {
	// ignore failed calls
	ret, _ := strconv.Atoi(kv["ret"])
	if ret < 0 {
		return
	}

	// extract replace data
	newFd, _ := strconv.Atoi(kv["newfd"])
	oldFd, _ := strconv.Atoi(kv["oldfd"])

	i.fdt.replace(proc, oldFd, newFd)
}

// handling all memory access system calls.
func (i *Interpreter) handleMemorySyscall(ts, proc, operand string, kv map[string]string) {
	// ignore failed calls
	ret, _ := strconv.Atoi(kv["ret"])
	if ret < 0 {
		return
	}

	// extract data
	address, _ := strconv.ParseInt(kv["addr"], 10, 64)
	fd := i.vm.search(proc, address)

	if fd == -2 {
		return
	}

	fname := i.fdt.search(proc, fd)

	if len(fname) == 0 {
		fname = fmt.Sprintf("unknown (fd=%d)", fd)
	}

	// convert ts to datetime
	tsInt64, _ := strconv.ParseInt(ts, 10, 64)
	timeDuration, _ := strconv.ParseInt(kv["diff"], 10, 64)

	startDate := i.toDatetime(tsInt64)
	endDate := i.toDatetime(tsInt64 + timeDuration)

	// write into the output file
	fmt.Fprintf(
		i.fd,
		"%s - %d [%s : memory_access] from file: %s\n",
		startDate.Format(time.RFC3339Nano),
		endDate.Sub(startDate).Nanoseconds(),
		proc,
		fname,
	)
}

// handling all system calls that change virtual memory address space.
func (i *Interpreter) handleAddressSpaceSyscall(ts, proc, operand string, kv map[string]string) {
	// ignore failed calls
	ret, _ := strconv.Atoi(kv["ret"])
	if ret < 0 {
		return
	}

	// extract data
	address, _ := strconv.ParseInt(kv["ret"], 10, 64)
	boundary, _ := strconv.ParseInt(kv["len"], 10, 64)

	// handle mmap and munmap
	if operand == "mmap" {
		fd, _ := strconv.Atoi(kv["fd"])
		i.vm.put(proc, fd, address, boundary)
	} else {
		fd := i.vm.search(proc, address)
		i.vm.remove(proc, fd)
	}
}
