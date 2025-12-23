package runtime

import "io"

// ActivityMonitorReader is an interface for activity monitoring during execution.
// Implementations wrap readers to track I/O activity for idle timeout detection.
type ActivityMonitorReader interface {
	MonitorReader(r io.Reader, output io.Writer) io.Reader
}
