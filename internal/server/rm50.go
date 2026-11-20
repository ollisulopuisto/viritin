package server

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"viritin/internal/radiobrowser"
	"viritin/internal/vtuner"
)

const (
	rm50Prefix        = "/setupapp/wistron/asp/browsexpa/"
	rm50Nav           = rm50Prefix + "navxml.asp"
	rm50Countries     = rm50Nav + "?gofile=LocationLevelTwo"
	rm50FMPath        = "/setupapp/company/asp/func/dynamod.asp"
	rm50SearchTimeout = 5 * time.Second
)

func (s *Server) rm50Route(w http.ResponseWriter, r *http.Request, path string) bool {
	switch path {
	case rm50FMPath:
		s.rm50FM(w, r)
	case rm50Prefix + "loginxml.asp":
		s.noteWebClient(r)
		// The RM50 asks for a token even when it does not send token=0.
		s.respond(w, http.StatusOK, "application/xml", []byte(vtuner.RM50XMLHeader+vtuner.Token("0000000000000000")))
	case rm50Prefix + "favxml.asp":
		s.rm50Favorites(w, r)
	case rm50Prefix + "afavxml.asp":
		s.rm50Page(w, r, nil)
	case rm50Prefix + "search.asp":
		s.rm50Search(w, r)
	case rm50Nav:
		s.rm50Navigation(w, r)
	default:
		return false
	}
	return true
}

func (s *Server) rm50FM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.respond(w, http.StatusMethodNotAllowed, "text/html; charset=utf-8", nil)
		return
	}
	id := r.URL.Query().Get("id")
	if id != "" && len(id) <= 20 && strings.Trim(id, "0123456789") == "" {
		raw := strings.TrimSpace(s.cfg.Str("RM50FM", id))
		u, err := url.Parse(raw)
		switch {
		case raw == "":
			log.Printf("RM50 FM: no stream set for station ID %s", id)
		case err != nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil:
			log.Printf("RM50 FM: invalid HTTP stream for station ID %s", id)
		default:
			w.Header().Set("Location", u.String())
			s.respond(w, http.StatusFound, "text/html; charset=utf-8", nil)
			return
		}
	}
	s.respond(w, http.StatusNotFound, "text/html; charset=utf-8", nil)
}

func (s *Server) rm50Navigation(w http.ResponseWriter, r *http.Request) {
	// The RM50 does not follow numeric hosts in country menu links.
	const base = "http://wistron.vtuner.com"
	back := base + rm50Countries
	var dirs []vtuner.Directory
	var stations []vtuner.Station
	q := r.URL.Query()
	if s.rb != nil {
		a, mac := s.avrFor(r)
		switch q.Get("gofile") {
		case "", "LocationLevelTwo":
			back = base + rm50Prefix + "loginxml.asp"
			countries, _ := s.rb.Categories(radiobrowser.Country, a, mac, 0, -1)
			for _, c := range countries {
				dirs = append(dirs, vtuner.Directory{
					Title: c.Name, ItemCount: c.Count,
					Destination: base + rm50Nav + "?gofile=LocationLevelThree&country=" + urlEncode(c.Name),
				})
			}
		case "LocationLevelThree":
			country := strings.TrimSpace(q.Get("country"))
			if country == "" {
				http.Error(w, "Missing country", http.StatusBadRequest)
				return
			}
			list, _ := s.rb.Stations(radiobrowser.Country, urlEncode(country), a, mac, 0, -1)
			stations = s.rm50StationList(r, list)
		default:
			http.NotFound(w, r)
			return
		}
	}
	total := len(dirs) + len(stations)
	if total == 0 {
		// The RM50 shows its own empty-list message and handles the Back button.
		s.rm50Page(w, r, nil)
		return
	}
	body := vtuner.RM50Navigation(back, total, rm50PageItems(q, dirs), rm50PageItems(q, stations))
	s.respond(w, http.StatusOK, "application/xml", []byte(body))
}

func (s *Server) rm50Favorites(w http.ResponseWriter, r *http.Request) {
	var list []vtuner.Station
	if s.mystOn {
		for _, category := range s.store.Snapshot() {
			for _, station := range category.Stations {
				if s.rm50Stream(station.URL) {
					item := s.vtunerStation(category.Name, station, r)
					// My Stations has no codec field, but many MP3 URLs name it.
					if u, err := url.Parse(station.URL); err == nil && strings.HasSuffix(strings.ToLower(u.Path), ".mp3") {
						item.Mime = "MP3"
					}
					list = append(list, item)
				}
			}
		}
	}
	s.rm50Page(w, r, list)
}

func (s *Server) rm50Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	text := strings.TrimSpace(q.Get("Search"))
	if text == "" {
		text = strings.TrimSpace(q.Get("search"))
	}
	if text == "" || s.rb == nil {
		s.rm50Page(w, r, nil)
		return
	}
	switch q.Get("sSearchtype") {
	case "2":
		s.rm50NameAndGenre(w, r, text)
	case "3":
		// Other receivers use 3 for playback; Wistron uses it for country.
		s.rm50RadioBrowser(w, r, radiobrowser.Country, urlEncode(text))
	default:
		s.rm50RadioBrowser(w, r, radiobrowser.Search, text)
	}
}

func (s *Server) rm50NameAndGenre(w http.ResponseWriter, r *http.Request, text string) {
	ctx, cancel := context.WithTimeout(r.Context(), rm50SearchTimeout)
	defer cancel()
	a, mac := s.avrFor(r)
	genre := urlEncode(strings.ToLower(text))
	lookup := func(kind radiobrowser.Kind, value string) []radiobrowser.Station {
		list, _ := s.rb.StationsContext(ctx, kind, value, a, mac, 0, -1)
		return list
	}
	// SQLite builds receiver views on demand. Keep those local queries in order.
	if s.rb.DB != nil {
		names := lookup(radiobrowser.Search, text)
		s.rm50RadioBrowserPage(w, r, names, lookup(radiobrowser.Genre, genre))
		return
	}
	names := make(chan []radiobrowser.Station, 1)
	go func() { names <- lookup(radiobrowser.Search, text) }()
	genres := lookup(radiobrowser.Genre, genre)
	s.rm50RadioBrowserPage(w, r, <-names, genres)
}

func (s *Server) rm50RadioBrowser(w http.ResponseWriter, r *http.Request, kind radiobrowser.Kind, text string) {
	a, mac := s.avrFor(r)
	// Filter before paging so ItemCount and page offsets describe the same list.
	stations, _ := s.rb.Stations(kind, text, a, mac, 0, -1)
	s.rm50RadioBrowserPage(w, r, stations)
}

func (s *Server) rm50RadioBrowserPage(w http.ResponseWriter, r *http.Request, sources ...[]radiobrowser.Station) {
	s.rm50Page(w, r, s.rm50StationList(r, sources...))
}

func (s *Server) rm50StationList(r *http.Request, sources ...[]radiobrowser.Station) []vtuner.Station {
	var list []vtuner.Station
	seen := make(map[string]bool)
	for _, source := range sources {
		for _, station := range source {
			if seen[station.ID] || !s.rm50Stream(station.URL) {
				continue
			}
			seen[station.ID] = true
			list = append(list, s.rbStation(station, "", r))
		}
	}
	return list
}

func (s *Server) rm50Stream(raw string) bool {
	u, err := url.Parse(raw)
	// Rewriting HTTPS to HTTP does not make a TLS-only station playable.
	return err == nil && u.Hostname() != "" &&
		(u.Scheme == "http" || u.Scheme == "https" && s.cfg.Bool("Configuration", "RelayHTTPS"))
}

func (s *Server) rm50Page(w http.ResponseWriter, r *http.Request, list []vtuner.Station) {
	s.respond(w, http.StatusOK, "application/xml", []byte(vtuner.RM50Stations(len(list), rm50PageItems(r.URL.Query(), list))))
}

func rm50PageItems[T any](q url.Values, list []T) []T {
	first, last := pageRange(q, len(list))
	if first <= last && last < len(list) {
		return list[first : last+1]
	}
	return nil
}
