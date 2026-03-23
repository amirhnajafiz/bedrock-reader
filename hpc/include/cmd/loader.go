package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/amirhnajafiz/flak-dashboard/include/components/loader"
	"github.com/amirhnajafiz/flak-dashboard/include/configs"
	"github.com/amirhnajafiz/flak-dashboard/pkg/files"
	"github.com/amirhnajafiz/flak-dashboard/pkg/models"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// LoaderCMD starts the loader process.
type LoaderCMD struct {
	Cfg configs.Config
}

// Command returns the cobra command instance.
func (l LoaderCMD) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "loader",
		Short: "Loading data logs",
		Long:  "Loader reads the logs from a data directory and creates the chunked data for interpreter",
		Run:   l.main,
	}
}

// main function will be called for loader process.
func (l LoaderCMD) main(_ *cobra.Command, _ []string) {
	// get log files
	files, err := l.getLogFile()
	if err != nil {
		logrus.WithField("error", err).Panic("loader failed")
	}

	// create a loader coordinator
	coor := loader.Coordinator{
		Readers:   l.Cfg.NumberOfReaders,
		Reductors: l.Cfg.NumberOfReductors,
		Writers:   l.Cfg.NumberOfReaders,
	}

	// start loading the data for each file sequentially
	for _, file := range files {
		if err := coor.Begin(file); err != nil {
			logrus.WithFields(logrus.Fields{
				"file":  file.Name,
				"error": err,
			}).Panic("coordinator failed")
		}
	}
}

// get log files returns the files that loader needs to process.
func (l LoaderCMD) getLogFile() ([]*models.File, error) {
	// array of files to be returned
	records := make([]*models.File, 0)

	// patterns
	patterns := []string{
		"trace_io_*.log",
		"trace_memory_*.log",
	}

	// for each pattern get the file names and build a file instance
	for _, pattern := range patterns {
		// create the output dir
		outPath := fmt.Sprintf(
			"%s/%s",
			l.Cfg.DataPath,
			strings.Replace(pattern, "*.log", "chunks", 1),
		)

		if err := os.MkdirAll(
			outPath,
			os.ModePerm); err != nil {
			return nil, fmt.Errorf("failed to create `%s` directory: %v", pattern, err)
		}

		// get the file names
		fileNames, err := files.GetFileNamesByWildcardMatch(l.Cfg.DataPath, pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to read `%s` files: %v", pattern, err)
		}

		for index, name := range fileNames {
			// create a new file model
			file := models.File{
				Id:        index,
				Name:      name,
				Path:      fmt.Sprintf("%s/%s", l.Cfg.DataPath, name),
				OutputDir: outPath,
			}

			// get file size
			size, err := files.GetFileSize(file.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to get file `%s` stats: %v", file.Path, err)
			}

			file.FileSize = size
			file.ChunkSize = int(size / int64(l.Cfg.NumberOfReaders))

			// store the file
			records = append(records, &file)
		}
	}

	return records, nil
}
