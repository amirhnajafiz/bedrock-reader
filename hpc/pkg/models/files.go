package models

// File holds the information of a log file.
type File struct {
	Id        int
	ChunkSize int
	FileSize  int64
	Name      string
	Path      string
	OutputDir string
}
