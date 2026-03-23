package logging

import (
	"bytes"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

type BacktickFormatter struct {
	TimestampFormat string
}

func (f *BacktickFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b bytes.Buffer

	// timestamp
	tsFormat := f.TimestampFormat
	if tsFormat == "" {
		tsFormat = time.RFC3339
	}

	b.WriteString("time=")
	b.WriteString(entry.Time.Format(tsFormat))

	// message
	b.WriteString(` msg="`)
	b.WriteString(entry.Message)
	b.WriteString(`"`)

	// fields in backticks
	for k := range entry.Data {
		b.WriteByte(' ')
		fmt.Fprintf(&b, "\"%s=%v\"", k, entry.Data[k])
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}
