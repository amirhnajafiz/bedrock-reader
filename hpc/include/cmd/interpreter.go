package cmd

import (
	"fmt"
	"sync"

	"github.com/amirhnajafiz/flak-dashboard/include/components/interpreter"
	"github.com/amirhnajafiz/flak-dashboard/include/configs"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// InterpreterCMD starts the interpreter process.
type InterpreterCMD struct {
	Cfg configs.Config
}

// Command returns the cobra command instance.
func (i InterpreterCMD) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "interpreter",
		Short: "Interpreting data chunks",
		Long:  "Interpreter reads the data chunks created by loader and builds a human readable log file",
		Run:   i.main,
	}
}

// main function will be called for interpreter process.
func (i InterpreterCMD) main(_ *cobra.Command, _ []string) {
	var wg sync.WaitGroup
	wg.Add(2)

	// start the two interpreters instances
	go i.runInterpreterInstance(fmt.Sprintf("%s/trace_io_chunks", i.Cfg.DataPath), &wg)
	go i.runInterpreterInstance(fmt.Sprintf("%s/trace_memory_chunks", i.Cfg.DataPath), &wg)

	// wait for them to finish
	wg.Wait()
}

// run an interpreter instance.
func (i *InterpreterCMD) runInterpreterInstance(
	path string,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	logrus.WithFields(logrus.Fields{
		"path": path,
	}).Info("interpreter start")

	// create a new interpreter and start
	output := path + "/replay.hrd"
	ii := interpreter.Interpreter{
		DataDirPath:     path,
		OutputFilePath:  output,
		ReferenceTSPath: i.Cfg.DataPath + "/reference_timestamps.json",
	}

	// check for any internal errors
	if err := ii.Begin(); err != nil {
		logrus.WithFields(logrus.Fields{
			"path":  path,
			"error": err,
		}).Panic("interpreter failed")
	}

	logrus.WithFields(logrus.Fields{
		"path": path,
	}).Info("interpreter finished")
}
