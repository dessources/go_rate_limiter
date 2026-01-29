package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
)

type FileHidingFileSystem struct {
	http.FileSystem
}
type FileHidingFile struct {
	http.File
}

type UrlShortenerPayload struct {
	Original string `json:"original"`
}

type ErrorResponse struct {
	ErrorMessage string `json:"errorMessage"`
}

//--------- Index route -------------------

func MakeIndexHandler() http.Handler {
	fsys := FileHidingFileSystem{http.Dir("./frontend/out/")}
	return http.FileServer(fsys)

}

//------- shortener routes ------------------------

type App struct {
	shortener UrlShortener
}

func (f *App) RetrieveUrl(w http.ResponseWriter, r *http.Request) {
	short := r.PathValue("shortUrl")

	if short != "" {
		if original, err := f.shortener.RetrieveUrl(short); err != nil {

			// w.WriteHeader(http.StatusNotFound)
			http.ServeFile(w, r, "frontend/out/404.html")

		} else {
			http.Redirect(w, r, original, http.StatusTemporaryRedirect)
		}
	} else {
		http.Redirect(w, r, "/", http.StatusPermanentRedirect)
	}

}

func (f *App) ShortenUrl(w http.ResponseWriter, r *http.Request) {
	var payload UrlShortenerPayload

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if message, ok := ValidateUrl(payload.Original); !ok {
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	if shortUrl, err := Shorten(f.shortener, payload.Original); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		response := map[string]string{
			"shortCode": shortUrl,
		}

		json.NewEncoder(w).Encode(response)
		// w.WriteHeader(http.StatusCreated)
		// fmt.Fprintf(w, "Short URL: %s\n", shortUrl)
	}

}

//------------- handler utils----------------------

func containsTxtFile(name string) bool {
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if strings.HasSuffix(part, ".txt") {
			return true
		}
	}
	return false
}

func (fsys FileHidingFileSystem) Open(name string) (http.File, error) {
	if containsTxtFile(name) {
		// If txt file, return 403 response
		return nil, fs.ErrPermission
	}

	file, err := fsys.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	return FileHidingFile{file}, nil
}

func ValidateUrl(s string) (string, bool) {
	if len(s) > maxUrlLength {
		return fmt.Sprintf("Provided url exceeds max-length of %d", maxUrlLength), false
	}

	u, err := url.Parse(s)

	if err != nil || u.Host == "" {
		return "Invalid url provided", false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "Invalid protocol provided. Only http:// or https:// allowed", false
	}

	return "", true
}
