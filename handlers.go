package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func Index(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>URL shortener is running!</h1>\n")
}

type Features struct {
	shortener UrlShortener
}

func (f *Features) RetrieveUrl(w http.ResponseWriter, r *http.Request) {
	short := r.PathValue("shortUrl")

	if short != "" {
		if original, err := f.shortener.RetrieveUrl(short); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Redirect(w, r, original, http.StatusTemporaryRedirect)
		}
	} else {
		http.Error(w, "Provided short url is empty", http.StatusBadRequest)
	}

}

func (f *Features) ShortenUrl(w http.ResponseWriter, r *http.Request) {
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
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "Short URL: %s\n", shortUrl)
	}

}

//------------- handler utils----------------------

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
