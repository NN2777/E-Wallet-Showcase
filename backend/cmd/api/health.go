package main

import "net/http"

func (app *Application) healthHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}