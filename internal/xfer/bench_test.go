package xfer

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func BenchmarkRead(b *testing.B) {
	for _, tc := range []struct {
		name string
		size int
	}{{"64B", 64}, {"4K", 4 << 10}, {"256K", 256 << 10}, {"4M", 4 << 20}} {
		b.Run(tc.name, func(b *testing.B) {
			payload := bytes.Repeat([]byte{'x'}, tc.size)
			b.ReportAllocs()
			b.SetBytes(int64(tc.size))
			for b.Loop() {
				r, w, err := os.Pipe()
				if err != nil {
					b.Fatal(err)
				}
				go func() {
					_, _ = w.Write(payload)
					_ = w.Close()
				}()
				data, err := Read(r, MaxPayload, time.Minute)
				if err != nil || len(data) != tc.size {
					b.Fatalf("read %d bytes, %v", len(data), err)
				}
				_ = r.Close()
			}
		})
	}
}
