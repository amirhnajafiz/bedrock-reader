package configs

// Default returns a config instance with default values.
func Default() Config {
	return Config{
		NumberOfReaders:   1,
		NumberOfReductors: 2,
		DataPath:          "data",
		Logger: LoggerConfig{
			Level: "info",
			JSON:  false,
		},
	}
}
