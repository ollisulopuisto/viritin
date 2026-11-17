package server

import (
	"strings"
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

func TestRBUIDMatchesPreviousMapping(t *testing.T) {
	s := &Server{}
	for _, id := range []string{"", "a-b-c", "abcdef01-2345-6789-abcd-ef0123456789", "ABCDEF012345", "---------------", "éàö-ß-hello", "12345678901éx", string([]byte{0xff, 'a', '-', 'z'})} {
		want := strings.ToUpper(strings.ReplaceAll(id, "-", ""))
		if len(want) > 12 {
			want = want[:12]
		}
		if got := s.rbUID(radiobrowser.Station{ID: id}); got != "RB_"+want {
			t.Errorf("id %q: got %q want %q", id, got, "RB_"+want)
		}
	}
}
