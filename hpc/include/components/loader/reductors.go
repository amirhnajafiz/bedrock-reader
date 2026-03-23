package loader

import (
	"fmt"
	"maps"
	"sync"

	"github.com/amirhnajafiz/flak-dashboard/pkg/models"
)

// reductor job is to merge and submit tracing events to the writers.
type reductor struct {
	// map memory for merging
	memory map[string]*models.Packet

	// wait groups
	readerReductorInFlightWg *sync.WaitGroup
	reductorWriterInFlightWg *sync.WaitGroup

	// channels
	terminationChannel chan int
	inputChannel       chan *models.Packet
	writerChannels     map[int]chan *models.Packet

	// validations params
	readLogs int
	sentLogs int
}

// start the reductor worker.
func (r *reductor) start() {
	for {
		select {
		case <-r.terminationChannel:
			return
		case pkt := <-r.inputChannel:
			r.readerReductorInFlightWg.Done()
			r.readLogs++

			// check if there is a match
			if val, ok := r.memory[pkt.TraceKey]; ok {
				var mergedPkt *models.Packet
				if val.TraceEvent.EventType == "EN" {
					mergedPkt = r.merge(val, pkt)
				} else {
					mergedPkt = r.merge(pkt, val)
				}

				r.reductorWriterInFlightWg.Add(1)
				r.writerChannels[mergedPkt.PartitionIndex] <- mergedPkt
				r.sentLogs++

				delete(r.memory, pkt.TraceKey)
			} else {
				// save the packet into memory if no match
				r.memory[pkt.TraceKey] = pkt
			}
		}
	}
}

// merge two trace events and place the results in the start packet.
func (r *reductor) merge(start *models.Packet, end *models.Packet) *models.Packet {
	// calculate the difference
	diff := end.TraceEvent.Timestamp - start.TraceEvent.Timestamp
	start.TraceEvent.KV["diff"] = fmt.Sprintf("%d", diff)

	// merge the key values
	maps.Copy(start.TraceEvent.KV, end.TraceEvent.KV)

	return start
}
