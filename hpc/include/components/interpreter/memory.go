package interpreter

// File descriptor table holds the mappings of a proc and its fds
// to file names (in full path).
type fdTable struct {
	// a map of proc to fd, and fd to fname
	kv map[string]map[int]string
}

// put a new entry.
func (f *fdTable) put(proc string, fd int, fname string) {
	if _, ok := f.kv[proc]; !ok {
		f.kv[proc] = make(map[int]string)
	}

	f.kv[proc][fd] = fname
}

// replace an existing entry.
func (f *fdTable) replace(proc string, oldFd int, newFd int) {
	if val, ok := f.kv[proc]; ok {
		if fname, fok := val[oldFd]; fok {
			val[newFd] = fname
			delete(val, oldFd)
		}
	}
}

// remove an existing entry.
func (f *fdTable) remove(proc string, fd int) {
	if val, ok := f.kv[proc]; !ok {
		delete(val, fd)
	}
}

// search a filename by using procname and fd.
func (f *fdTable) search(proc string, fd int) string {
	if val, ok := f.kv[proc]; ok {
		if fname, fok := val[fd]; fok {
			return fname
		}
	}

	return ""
}

// virtual memory address space holds the mappings of a
// file descriptor to its memory addresses.
type virtualMemoryAddressSpace struct {
	blocks map[string]map[int][]int64
}

// put a new entry.
func (v *virtualMemoryAddressSpace) put(proc string, fd int, start int64, boundary int64) {
	if _, ok := v.blocks[proc]; !ok {
		v.blocks[proc] = make(map[int][]int64)
	}

	v.blocks[proc][fd] = []int64{start, start + boundary}
}

// remove an existing entry.
func (v *virtualMemoryAddressSpace) remove(proc string, fd int) {
	if val, ok := v.blocks[proc]; ok {
		delete(val, fd)
	}
}

// search an address to get its file descriptor.
func (v *virtualMemoryAddressSpace) search(proc string, address int64) int {
	if val, ok := v.blocks[proc]; ok {
		for key, value := range val {
			if value[0] <= address && value[1] >= address {
				return key
			}
		}
	}

	return -2 // -1 could be a hidden mmap
}
