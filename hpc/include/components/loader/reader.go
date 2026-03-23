package loader

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/amirhnajafiz/flak-dashboard/pkg/hashing"
	"github.com/amirhnajafiz/flak-dashboard/pkg/models"
)

var (
	// Regex for parsing the log entries.
	re = regexp.MustCompile(`^(\d+)\s+\{pid=(\d+)\s+tid=(\d+)\s+proc=([^\}]+)\}\{(EN|EX)\s+([^\}]+)\}(?:\{([^\}]*)\})?$`)
)

// reader gets a file, an offset, and a chunk size to read a partition of the given file.
type reader struct {
	id                       int
	numberOfReductors        int
	readerReductorInFlightWg *sync.WaitGroup
	reductorChannels         map[int]chan *models.Packet

	// read parameters
	offset    int64
	chunkSize int64
	fileSize  int64
	filePath  string

	// validation params
	readLogs int
	sentLogs int
}

// start the reader worker.
func (r *reader) start() error {
	// open the log file
	fd, err := os.Open(r.filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer fd.Close()

	// seek to chunk start
	_, err = fd.Seek(r.offset, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek: %v", err)
	}

	// create a reader of 64Kb size
	reader := bufio.NewReaderSize(fd, 64*1024)

	// set the parameters
	chunkEnd := r.offset + r.chunkSize
	var currentPos int64 = r.offset

	// if not the first chunk, skip partial first line
	if r.offset > 0 {
		line, _ := reader.ReadString('\n')
		if len(line) > 0 {
			currentPos += int64(len(line))
		}
	}

	// read the partition line by line
	for {
		line, err := reader.ReadString('\n')
		r.readLogs++

		if len(line) > 0 {
			currentPos += int64(len(line))

			// call the log handler
			r.logHandler(line)

			// if we passed the chunk end and finished a line, stop
			if currentPos >= chunkEnd {
				break
			}
		}

		if err != nil {
			break
		}
	}

	return nil
}

// In log handler, a line is matched with the regex and skipped if not matching.
// Otherwise, handler builds a packet and sends it to the reductor.
func (r *reader) logHandler(line string) {
	// trim the line
	line = strings.TrimSpace(line)

	// match with the regex
	match := re.FindStringSubmatch(line)
	if match == nil {
		return
	}

	// extract metadata
	timestamp, _ := strconv.ParseInt(match[1], 10, 64)
	pid, _ := strconv.Atoi(match[2])
	tid, _ := strconv.Atoi(match[3])

	// parse the key-value block
	kv := make(map[string]string)
	parts := strings.SplitSeq(match[7], " ")
	for part := range parts {
		kvPair := strings.SplitN(part, "=", 2)
		if len(kvPair) == 2 {
			kv[kvPair[0]] = kvPair[1]
		}
	}

	// create a packet
	pkt := &models.Packet{
		PartitionIndex: r.id,
		PartitionName:  r.filePath,
		TraceKey:       fmt.Sprintf("%d-%d-%s-%s", pid, tid, match[4], match[6]),
		TraceEventRaw:  line,
		TraceEvent: models.TraceEvent{
			Timestamp: timestamp,
			PID:       pid,
			TID:       tid,
			Proc:      match[4],
			EventType: match[5],
			Event:     match[6],
			KV:        kv,
		},
	}

	// find the reductor using a hash range function
	index := hashing.HashToRange(pkt.TraceKey, uint64(r.numberOfReductors))

	// submit the event
	r.readerReductorInFlightWg.Add(1)
	r.reductorChannels[index] <- pkt
	r.sentLogs++
}
