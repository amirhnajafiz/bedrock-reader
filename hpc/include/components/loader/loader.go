package loader

/*
The loader module is for reading the log files from the data source,
removing the bad logs, merging EN and EX entries, and write the output
into chunks of data so the interpreter can process them sequentially.

A data entry would go through this pipeline as follow:

1. reader reads a partition of the file
2. passes the logs to a reductor based on a hashed key
3. reductor stores the logs in its memory and merges the EN and EX entries
4. passes the merged logs to a writer based on a partition id (which readers set)
5. the writer writes logs into files
*/

import (
	"fmt"
	"sync"

	"github.com/amirhnajafiz/flak-dashboard/pkg/files"
	"github.com/amirhnajafiz/flak-dashboard/pkg/models"

	"github.com/sirupsen/logrus"
)

// Coordinator is responsible for managing a data loading operation from
// a file, using readers, reductors, and writers.
type Coordinator struct {
	// number of go-routines
	Readers   int
	Reductors int
	Writers   int

	// communication channels
	chanReductors map[int]chan *models.Packet
	chanWriters   map[int]chan *models.Packet

	// termination channels
	tchanReductors map[int]chan int
	tchanWriters   map[int]chan int

	// wait groups for syncing
	wgReaders                sync.WaitGroup
	wgReductors              sync.WaitGroup
	wgWriters                sync.WaitGroup
	wgReaderReductorInFlight sync.WaitGroup
	wgReductorWriterInFlight sync.WaitGroup

	// metrics for validation
	metricReadersReadLogs   int
	metricReadersSentLogs   int
	metricReductorsReadLogs int
	metricReductorsSentLogs int
	metricWriterSentLogs    int

	// metrics variables mutex
	lockMetrics sync.Mutex
}

// initialize the coordinator variables.
func (c *Coordinator) initVars() {
	// communication channels for writers
	c.chanWriters = make(map[int]chan *models.Packet, c.Writers)
	c.tchanWriters = make(map[int]chan int, c.Writers)

	// communication channels for reductors
	c.chanReductors = make(map[int]chan *models.Packet, c.Reductors)
	c.tchanReductors = make(map[int]chan int, c.Reductors)

	// validation metrics
	c.metricReadersReadLogs = 0
	c.metricReadersSentLogs = 0
	c.metricReductorsReadLogs = 0
	c.metricReductorsSentLogs = 0
	c.metricWriterSentLogs = 0
}

// start a new reader instance.
func (c *Coordinator) newReader(id int, file *models.File) {
	defer c.wgReaders.Done()

	r := reader{
		id:                       id,
		offset:                   int64(id) * int64(file.ChunkSize),
		chunkSize:                int64(file.ChunkSize),
		fileSize:                 file.FileSize,
		filePath:                 file.Path,
		readLogs:                 0,
		sentLogs:                 0,
		reductorChannels:         c.chanReductors,
		numberOfReductors:        c.Reductors,
		readerReductorInFlightWg: &c.wgReaderReductorInFlight,
	}
	if err := r.start(); err != nil {
		logrus.WithFields(logrus.Fields{
			"path":  file.Path,
			"id":    id,
			"error": err,
		}).Panic("writer failed")
	}

	// update validation metrics
	c.lockMetrics.Lock()
	c.metricReadersReadLogs += r.readLogs
	c.metricReadersSentLogs += r.sentLogs
	c.lockMetrics.Unlock()
}

// start a new reductor instance.
func (c *Coordinator) newReductor(id int) {
	defer close(c.chanReductors[id])
	defer c.wgReductors.Done()

	rd := reductor{
		readLogs:                 0,
		sentLogs:                 0,
		memory:                   make(map[string]*models.Packet),
		inputChannel:             c.chanReductors[id],
		terminationChannel:       c.tchanReductors[id],
		writerChannels:           c.chanWriters,
		readerReductorInFlightWg: &c.wgReaderReductorInFlight,
		reductorWriterInFlightWg: &c.wgReductorWriterInFlight,
	}
	rd.start()

	// update validation metrics
	c.lockMetrics.Lock()
	c.metricReductorsReadLogs += rd.readLogs
	c.metricReductorsSentLogs += rd.sentLogs
	c.lockMetrics.Unlock()
}

// start a new writer instance.
func (c *Coordinator) newWriter(id int, file *models.File) {
	defer close(c.chanWriters[id])
	defer c.wgWriters.Done()

	path := fmt.Sprintf("%s/%d.%d.out", file.OutputDir, file.Id, id)

	w := writer{
		sentLogs:                 0,
		path:                     path,
		inputChannel:             c.chanWriters[id],
		termincationChannel:      c.tchanWriters[id],
		reductorWriterInFlightWg: &c.wgReductorWriterInFlight,
	}

	if err := w.start(); err != nil {
		logrus.WithFields(logrus.Fields{
			"path":  path,
			"id":    id,
			"error": err,
		}).Panic("writer failed")
	}

	// update validation metrics
	c.lockMetrics.Lock()
	c.metricWriterSentLogs += w.sentLogs
	c.lockMetrics.Unlock()
}

// safe wait for all go-routines to finish.
func (c *Coordinator) safeWait(name string) {
	// wait for the readers
	c.wgReaders.Wait()

	logrus.WithFields(logrus.Fields{
		"file": name,
	}).Debug("readers finished")

	// wait for the reader-reductor in flight events
	c.wgReaderReductorInFlight.Wait()

	// send terminate signal to all reductors
	for _, channel := range c.tchanReductors {
		channel <- 1
		close(channel)
	}
	c.wgReductors.Wait()

	logrus.WithFields(logrus.Fields{
		"file": name,
	}).Debug("reductors finished")

	// wait for the reductor-writer in flight events
	c.wgReductorWriterInFlight.Wait()

	// send terminate signal to all writers
	for _, channel := range c.tchanWriters {
		channel <- 1
		close(channel)
	}
	c.wgWriters.Wait()

	logrus.WithFields(logrus.Fields{
		"file": name,
	}).Debug("writers finished")
}

// print out the validation metrics.
func (c *Coordinator) logValidationMetrics(name string) {
	readersFilterPercentage := 100 * float32(c.metricReadersReadLogs-c.metricReadersSentLogs) / float32(c.metricReadersReadLogs)
	reductorsFilterPercentage := 100 * float32(c.metricReductorsReadLogs-c.metricReductorsSentLogs) / float32(c.metricReductorsReadLogs)
	writersMissingPercentage := 100 * float32(c.metricReductorsSentLogs-c.metricWriterSentLogs) / float32(c.metricReductorsSentLogs)

	// print validation information
	logrus.WithFields(logrus.Fields{
		"file":                  name,
		"readers_read_logs":     c.metricReadersReadLogs,
		"readers_sent_logs":     c.metricReadersSentLogs,
		"reductors_read_logs":   c.metricReductorsReadLogs,
		"reductors_sent_logs":   c.metricReductorsSentLogs,
		"writers_wrote_logs":    c.metricWriterSentLogs,
		"readers_filter_perc":   readersFilterPercentage,
		"reductors_filter_perc": reductorsFilterPercentage,
		"writers_missing_perc":  writersMissingPercentage,
	}).Info("logs loaded")
}

// Begin loading data from a given file.
func (c *Coordinator) Begin(file *models.File) error {
	// sort the input file (file exists check)
	if err := files.SortFile(file.Path); err != nil {
		return fmt.Errorf("failed to sort: %v", err)
	}

	// call init vars on the coordinator
	c.initVars()

	// start the writers as go-routines
	for i := range c.Writers {
		c.wgWriters.Add(1)

		c.chanWriters[i] = make(chan *models.Packet)
		c.tchanWriters[i] = make(chan int)

		go c.newWriter(i, file)
	}

	logrus.WithFields(logrus.Fields{
		"writers": c.Writers,
		"file":    file.Name,
	}).Info("writers start")

	// start the reductors as go-routines
	for i := range c.Reductors {
		c.wgReductors.Add(1)

		c.chanReductors[i] = make(chan *models.Packet)
		c.tchanReductors[i] = make(chan int)

		go c.newReductor(i)
	}

	logrus.WithFields(logrus.Fields{
		"reductors": c.Reductors,
		"file":      file.Name,
	}).Info("reductors start")

	// start the readers as go-routines
	for i := range c.Readers {
		c.wgReaders.Add(1)

		go c.newReader(i, file)
	}

	logrus.WithFields(logrus.Fields{
		"readers": c.Readers,
		"file":    file.Name,
	}).Info("readers start")

	// safe wait on all go-routines
	c.safeWait(file.Name)

	// print the validation metrics
	c.logValidationMetrics(file.Name)

	return nil
}
