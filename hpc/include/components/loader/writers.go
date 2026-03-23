package loader

import (
	"fmt"
	"os"
	"sync"

	"github.com/amirhnajafiz/flak-dashboard/pkg/files"
	"github.com/amirhnajafiz/flak-dashboard/pkg/models"
)

// writer worker submits the events by writing them into files.
// each writer writes into a single file.
type writer struct {
	// file
	path string

	// wait groups
	reductorWriterInFlightWg *sync.WaitGroup

	// channels
	termincationChannel chan int
	inputChannel        chan *models.Packet

	// validation params
	sentLogs int
}

// start the writer worker.
func (w *writer) start() error {
	// open the output file
	fd, err := os.Create(w.path)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer fd.Close()

	for {
		select {
		case <-w.termincationChannel:
			return files.SortFile(w.path)
		case pkt := <-w.inputChannel:
			w.reductorWriterInFlightWg.Done()
			w.sentLogs++

			fd.WriteString(pkt.TraceEvent.ToStr() + "\n")
		}
	}
}
