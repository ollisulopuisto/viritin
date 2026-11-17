package server

import (
	"testing"

	"viritin/internal/radiobrowser"
)

func BenchmarkRBUID(b *testing.B) {
	s := &Server{}
	st := radiobrowser.Station{ID: "0000000a-1111-2222-3333-444444444444"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.rbUID(st)
	}
}

func TestIndexFoldASCII(t *testing.T) {
	u := "http://x.example/proxy/?q=HTTPS://y.example/a.m3u8"
	i := indexFoldASCII(u, "https://")
	if i != len("http://x.example/proxy/?q=") {
		t.Fatalf("index %d", i)
	}
	if indexFoldASCII("http://a", "https://") != -1 {
		t.Fatal("found in a plain http url")
	}
}
