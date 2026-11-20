package server

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"viritin/internal/radiobrowser"
)

func TestRM50(t *testing.T) {
	s := newTestServer(t)
	s.rb = nil
	s.mystOn = true
	s.store.Own = filepath.Join(t.TempDir(), "stations.ini")
	if err := os.WriteFile(s.store.Own, []byte("[Radio]\nA & B=http://example.org/a.mp3?x=1&y=2\nTLS=https://example.org/b.mp3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s.store.Reload()
	token := "<EncryptedToken>0000000000000000</EncryptedToken>"
	for _, tt := range []struct {
		path, want, omit string
		status           int
	}{
		{"/setupapp/wistron/asp/browsexpa/loginXML.asp", token, "", 200},
		{"/setupapp/Wistron/asp/browsexpa/FavXML.asp", "<StationName>A &amp; B</StationName>", "<StationName>TLS</StationName>", 200},
		{"/setupapp/Wistron/asp/browsexpa/FavXML.asp?start=2&howmany=1", "<ItemCount>1</ItemCount>", "<ItemType>Station</ItemType>", 200},
		{"/setupapp/Wistron/asp/browsexpa/AFavXML.asp", "<ItemCount>0</ItemCount>", "", 200},
		{"/setupapp/denon/asp/browsexpa/FavXML.asp", "", "", 404},
		{"/setupapp/denon/loginXML.asp?token=0", "<EncryptedToken>0123456789ABCDEF</EncryptedToken>", token, 200},
	} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
		body := w.Body.String()
		if w.Code != tt.status || !strings.Contains(body, tt.want) || tt.omit != "" && strings.Contains(body, tt.omit) {
			t.Errorf("%s: status = %d, body = %s", tt.path, w.Code, body)
		}
	}
}

func TestRM50FMPlayback(t *testing.T) {
	const stream = "http://example.org/live.mp3"
	s := newTestServer(t)
	s.cfg.File.Section("RM50FM").Key("27491").SetValue(stream)
	s.cfg.File.Section("RM50FM").Key("20925").SetValue("https://example.org/unsupported")

	for _, tt := range []struct {
		path, location string
		status         int
	}{
		{"/setupapp/company/asp/func/dynamOD.asp?ex45v=&id=27491", stream, http.StatusFound},
		{"/setupapp/company/asp/func/dynamOD.asp?id=99999", "", http.StatusNotFound},
		{"/setupapp/company/asp/func/dynamOD.asp?id=20925", "", http.StatusNotFound},
		{"/setupapp/company/asp/func/dynamOD.asp?id=invalid", "", http.StatusNotFound},
		{"/setupapp/denon/asp/func/dynamOD.asp?id=27491", "", http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if w.Code != tt.status || w.Header().Get("Location") != tt.location || w.Body.Len() != 0 {
			t.Errorf("%s: status = %d, Location = %q, body = %q", tt.path, w.Code, w.Header().Get("Location"), w.Body.String())
		}
	}
}

func TestRM50Search(t *testing.T) {
	for _, tt := range []struct{ query, path, name string }{
		{"sSearchtype=1&Search=A+%26+B", "/json/stations/search", "A & B"},
		{"sSearchtype=3&Search=United+Kingdom", "/json/stations/bycountryexact/United Kingdom", ""},
	} {
		t.Run(tt.path, func(t *testing.T) {
			rb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path || r.URL.Query().Get("name") != tt.name {
					t.Errorf("unexpected Radio Browser request: %s", r.URL)
				}
				_, _ = w.Write([]byte(`[
{"stationuuid":"01234567-89ab-cdef-0123-456789abcdef","name":"HTTP station","url":"http://example.org/a.mp3","codec":"MP3","lastcheckok":1},
{"stationuuid":"12345678-9abc-def0-1234-56789abcdef0","name":"TLS station","url":"https://example.org/b.mp3","codec":"MP3","lastcheckok":1}
]`))
			}))
			defer rb.Close()
			s := newTestServer(t)
			s.rb = radiobrowser.New(rb.URL, "test", 100, 1, "catNone", 0)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/setupapp/Wistron/asp/Browsexpa/Search.asp?"+tt.query, nil))
			body := w.Body.String()
			if w.Code != 200 || !strings.Contains(body, "<ItemCount>1</ItemCount>") || !strings.Contains(body, "<StationName>HTTP station</StationName>") || strings.Contains(body, "TLS station") {
				t.Errorf("status = %d, body = %s", w.Code, body)
			}
		})
	}
}

func TestRM50NameAndGenreShareSearchType(t *testing.T) {
	rb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/json/stations/search" && r.URL.Query().Get("name") == "Alternative":
			_, _ = w.Write([]byte(`[{"stationuuid":"0123456789ab","name":"Alternative FM","url":"http://example.org/a","lastcheckok":1}]`))
		case r.URL.Path == "/json/stations/bytagexact/alternative":
			_, _ = w.Write([]byte(`[
{"stationuuid":"0123456789ab","name":"Alternative FM","url":"http://example.org/a","lastcheckok":1},
{"stationuuid":"123456789abc","name":"Genre only FM","url":"http://example.org/b","lastcheckok":1},
{"stationuuid":"23456789abcd","name":"TLS only","url":"https://example.org/c","lastcheckok":1}]`))
		default:
			t.Errorf("unexpected Radio Browser request: %s", r.URL)
		}
	}))
	defer rb.Close()
	s := newTestServer(t)
	s.rb = radiobrowser.New(rb.URL, "test", 100, 1, "catNone", 0)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/setupapp/Wistron/asp/Browsexpa/Search.asp?sSearchtype=2&Search=Alternative", nil))
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "<ItemCount>2</ItemCount>") || !strings.Contains(body, "<StationName>Genre only FM</StationName>") || strings.Contains(body, "TLS only") {
		t.Errorf("status = %d, body = %s", w.Code, body)
	}
}

func TestRM50SearchKeepsResultsWhenOtherLookupTimesOut(t *testing.T) {
	for _, slowPath := range []string{"/json/stations/search", "/json/stations/bytagexact/alternative"} {
		t.Run(slowPath, func(t *testing.T) {
			release := make(chan struct{})
			rb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == slowPath {
					select {
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				_, _ = w.Write([]byte(`[{"stationuuid":"0123456789ab","name":"Fast station","url":"http://example.org/a","lastcheckok":1}]`))
			}))
			defer rb.Close()
			defer close(release)
			s := newTestServer(t)
			s.rb = radiobrowser.New(rb.URL, "test", 100, 1, "catNone", 0)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/setupapp/wistron/asp/browsexpa/search.asp?sSearchtype=2&Search=Alternative", nil).WithContext(ctx))
			}()
			select {
			case <-done:
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<StationName>Fast station</StationName>") {
					t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("search did not return after the request deadline")
			}
		})
	}
}

func TestRM50Navigation(t *testing.T) {
	const root = "/setupapp/Wistron/asp/browsexpa/navXML.asp?gofile=LocationLevelTwo"
	rb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/tags", "/json/languages":
			_, _ = w.Write([]byte(`[]`))
		case "/json/countries":
			_, _ = w.Write([]byte(`[{"name":"United Kingdom","stationcount":2}]`))
		case "/json/stations/bycountryexact/United Kingdom":
			_, _ = w.Write([]byte(`[
{"stationuuid":"0123456789ab","name":"HTTP station","url":"http://example.org/a","lastcheckok":1},
{"stationuuid":"123456789abc","name":"TLS only","url":"https://example.org/b","lastcheckok":1}]`))
		case "/json/stations/bycountryexact/France":
			_, _ = w.Write([]byte(`[{"stationuuid":"123456789abc","name":"TLS only","url":"https://example.org/b","lastcheckok":1}]`))
		default:
			t.Errorf("unexpected Radio Browser request: %s", r.URL)
		}
	}))
	defer rb.Close()
	s := newTestServer(t)
	s.rb = radiobrowser.New(rb.URL, "test", 100, 1, "catNone", 0)
	page := func(path string) (p struct {
		ItemCount int
		Items     []struct{ ItemType, Title, UrlDir, UrlPrevious, StationName string } `xml:"Item"`
	}) {
		t.Helper()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if err := xml.Unmarshal(w.Body.Bytes(), &p); w.Code != 200 || err != nil {
			t.Fatalf("%s: status = %d, XML error = %v, body = %s", path, w.Code, err, w.Body.String())
		}
		return p
	}
	countries := page(root)
	if countries.ItemCount != 2 || len(countries.Items) != 2 || countries.Items[0].ItemType != "Previous" ||
		countries.Items[1].ItemType != "Dir" || countries.Items[1].Title != "United Kingdom" ||
		!strings.HasPrefix(countries.Items[1].UrlDir, "http://wistron.vtuner.com/") {
		t.Fatalf("unexpected country list: %+v", countries)
	}
	stations := page(countries.Items[1].UrlDir)
	if stations.ItemCount != 2 || len(stations.Items) != 2 || stations.Items[0].ItemType != "Previous" ||
		!strings.EqualFold(stations.Items[0].UrlPrevious, "http://wistron.vtuner.com"+root) ||
		stations.Items[1].ItemType != "Station" || stations.Items[1].StationName != "HTTP station" {
		t.Fatalf("unexpected country station list: %+v", stations)
	}
	empty := page("/setupapp/Wistron/asp/browsexpa/navXML.asp?gofile=LocationLevelThree&country=France")
	if empty.ItemCount != 0 || len(empty.Items) != 0 {
		t.Errorf("expected an empty country list with no extra items: %+v", empty)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/setupapp/wistron/asp/browsexpa/radiobrowser/country", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("old RM50 country route: status = %d", w.Code)
	}
	denon := page("/setupapp/denon/navXML.asp")
	for _, entry := range denon.Items {
		if entry.UrlDir == "http://localhost/viritin/radiobrowser/country?viritin=true" {
			return
		}
	}
	t.Errorf("Denon country link missing: %+v", denon)
}
