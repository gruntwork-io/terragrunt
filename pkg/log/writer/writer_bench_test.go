package writer_test

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
)

// tfOutputChunk mimics one read from a tofu process's stdout pipe, where many
// newline separated records arrive in a single Write.
func tfOutputChunk(lines int) []byte {
	var sb strings.Builder

	for i := range lines {
		sb.WriteString("module.vpc.aws_subnet.private[")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("]: Refreshing state... [id=subnet-0a1b2c3d4e5f6a7b8]\n")
	}

	return []byte(sb.String())
}

func BenchmarkWriterWrite(b *testing.B) {
	for _, lines := range []int{1, 16, 256} {
		b.Run(strconv.Itoa(lines)+"lines", func(b *testing.B) {
			l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(io.Discard))
			w := writer.New(writer.WithLogger(l), writer.WithMsgSeparator("\n"))
			chunk := tfOutputChunk(lines)

			b.ReportAllocs()
			b.SetBytes(int64(len(chunk)))

			for b.Loop() {
				if _, err := w.Write(chunk); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
