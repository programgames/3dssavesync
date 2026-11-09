package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
)

// remote is the server of a Session: HTTP, and the GET /saves list of the current pass, shared by every Client of the
// Session. The list is read once per pass, and so is an error (network down: a single timeout for the whole pass); it
// is read again after an Upload or a 412.
type remote struct {
	cfg      *Config
	dir      string // log
	http     *http.Client
	list     []Version
	listErr  error
	listRead bool
}

// request sends a request to the server for a Client (X-Client: clientName).
func (r *remote) request(client, method, path string, body []byte, headers map[string]string) (int, []byte, error) {
	var b io.Reader
	if body != nil {
		b = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, strings.TrimRight(r.cfg.URL, "/")+path, b)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.cfg.Token)
	req.Header.Set("X-Client", clientName(client))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rep, err := r.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer rep.Body.Close()
	if rep.StatusCode == http.StatusUnauthorized {
		return 0, nil, &httpError{method + " " + path, 401}
	}
	buf, err := io.ReadAll(rep.Body)
	if err != nil { // cut-off response: the network is at fault, like a request with no response (classify)
		return 0, nil, &neturl.Error{Op: method, URL: path, Err: err}
	}
	return rep.StatusCode, buf, nil
}

// httpError is a server response other than the expected one. Error() keeps the technical detail, for the log.
type httpError struct {
	request string
	code    int
}

func (e *httpError) Error() string { return fmt.Sprintf("%s : %d", e.request, e.code) }

// clientName returns the Client name for the server ([a-z0-9]{1,16}): two emulators = two Clients.
func clientName(profile string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(profile) {
		if (c >= 'a' && c <= 'z' || c >= '0' && c <= '9') && b.Len() < 16 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// serverVersion returns the Current version of a Game on the server, or nil.
func (r *remote) serverVersion(client, key string) (*Version, error) {
	if !r.listRead {
		r.list, r.listErr = r.readList(client)
		r.listRead = true
	}
	if r.listErr != nil {
		return nil, r.listErr
	}
	for i := range r.list {
		if r.list[i].Code == key {
			v := r.list[i]
			return &v, nil
		}
	}
	return nil, nil
}

func (r *remote) readList(client string) ([]Version, error) {
	st, o, err := r.request(client, "GET", "/saves", nil, nil)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		return nil, &httpError{"GET /saves", st}
	}
	var list []Version
	if err := json.Unmarshal(o, &list); err != nil { // Wi-Fi portal page, wrong address: unexpected response
		logTo(r.dir, "GET /saves illisible : "+err.Error())
		return nil, &httpError{"GET /saves", st}
	}
	return list, nil
}

// setName gives the game name to the server, for the History (ADR 0018). A failure does not block the Sync: it is
// logged, and the name is marked as known so that this Session does not try again.
func (r *remote) setName(client string, j *Game) {
	if strings.HasSuffix(j.Key, "-"+j.Name) { // a 3DS game known only by its title ID: a later ROM gives its name
		return
	}
	if st, _, err := r.request(client, "PUT", "/saves/"+j.Key+"/name", []byte(j.Name), nil); err != nil || st != 204 {
		logTo(r.dir, fmt.Sprintf("nom de %s non envoyé : %d %v", j.Key, st, err))
	}
	for i := range r.list {
		if r.list[i].Code == j.Key {
			r.list[i].Name = j.Name
		}
	}
}
