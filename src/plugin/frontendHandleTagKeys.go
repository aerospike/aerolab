package plugin

import "net/http"

func (p *Plugin) handleTagKeys(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("not implemented"))
}
