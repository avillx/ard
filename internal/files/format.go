package files

import "fmt"

const (
	bytesPerKilobyte = 1024
	bytesPerMegabyte = bytesPerKilobyte * 1024
)

func FormatSize(n int) string {
	switch {
	case n >= bytesPerMegabyte:
		return fmt.Sprintf("%.1fmb", float64(n)/bytesPerMegabyte)
	case n >= bytesPerKilobyte:
		return fmt.Sprintf("%.1fkb", float64(n)/bytesPerKilobyte)
	default:
		return fmt.Sprintf("%db", n)
	}
}
