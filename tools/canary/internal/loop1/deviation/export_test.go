package deviation

import (
	"context"
	"io"
)

// RunWith runs the verb with another stamper.
func RunWith(ctx context.Context, args []string, stdout, stderr io.Writer, stamper Stamping) int {
	return run(ctx, args, stdout, stderr, stamper)
}
