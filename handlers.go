package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

func Index(ctx HTTPContext) {
	ctx.w.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.w, "<h1>URL shortener is running!</h1>\n")
}

func RetrieveUrl(s UrlShortener, ctx HTTPContext) {
	short := ctx.req.PathValue("shortUrl")

	if short != "" {
		if original, err := s.RetrieveUrl(short); err != nil {
			http.Error(ctx.w, err.Error(), http.StatusNotFound)
		} else {
			http.Redirect(ctx.w, ctx.req, original, http.StatusTemporaryRedirect)
		}
	} else {
		http.Error(ctx.w, "Provided short url is empty", http.StatusBadRequest)
	}

}

func ShortenUrl(s UrlShortener, ctx HTTPContext) {
	var payload UrlShortenerPayload

	if err := json.NewDecoder(ctx.req.Body).Decode(&payload); err != nil {
		http.Error(ctx.w, err.Error(), http.StatusBadRequest)
		return
	}

	if message, ok := ValidateUrl(payload.Original); !ok {
		http.Error(ctx.w, message, http.StatusBadRequest)
		return
	}

	if shortUrl, err := Shorten(s, payload.Original); err != nil {
		http.Error(ctx.w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		ctx.w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(ctx.w, "Short URL: %s\n", shortUrl)
	}

}

// handler utils
func createUrlShortener() (UrlShortener, RouteHandler, RouteHandler) {
	s, err := NewUrlShortener(InMemory, 100000, time.Hour)
	if err != nil {
		log.Fatal(err)
	}
	return s,
		func(ctx HTTPContext) {
			ShortenUrl(s, ctx)
		},
		func(ctx HTTPContext) {
			RetrieveUrl(s, ctx)
		}
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
