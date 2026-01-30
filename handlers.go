package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"
)

type UrlShortenerPayload struct {
	Original string `json:"original"`
}

type ErrorResponse struct {
	ErrorMessage string `json:"errorMessage"`
}

type Metrics struct {
	GlobalTokenBucketCap int `json:"globalTokenBucketCap"`
	GlobalTokensUsed     int `json:"globalTokensUsed"`
	ActiveUsers          int `json:"activeUsers"`
	CurrentUrlCount      int `json:"currentUrlCount"`
}

//--------- Index route -------------------

func MakeIndexHandler() http.Handler {
	fsys := FileHidingFileSystem{http.Dir("./frontend/out/")}
	return http.FileServer(fsys)

}

//------- shortener routes ------------------------

type App struct {
	shortener            UrlShortener
	globalRateLimiter    *GlobalRateLimiter
	perClientRateLimiter *PerClientRateLimiter
}

func (app *App) RetrieveUrl(w http.ResponseWriter, r *http.Request) {
	short := r.PathValue("shortUrl")

	if short != "" {
		if original, err := app.shortener.RetrieveUrl(short); err != nil {

			//setting this header makes Go warn me that ServeFile tries to set
			//status again to 200 internally but fails silently.
			//TODO: load 404.html then return text/html with status 404 instead of ServeFile
			w.WriteHeader(http.StatusNotFound)
			http.ServeFile(w, r, "frontend/out/404.html")

		} else {
			http.Redirect(w, r, original, http.StatusTemporaryRedirect)
		}
	} else {
		http.Redirect(w, r, "/", http.StatusPermanentRedirect)
	}

}

func (app *App) ShortenUrl(w http.ResponseWriter, r *http.Request) {
	var payload UrlShortenerPayload
	errorMessage := ""

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errorMessage = "Oops, we couldn't process your request. Please try again later."
		json.NewEncoder(w).Encode(&ErrorResponse{errorMessage})
		return

	} else if message, ok := ValidateUrl(payload.Original); !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrorResponse{message})
		return
	} else {

		shortUrl, err := Shorten(app.shortener, payload.Original)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			errorMessage = "Something broke on our end. Please try again later."
			json.NewEncoder(w).Encode(&ErrorResponse{errorMessage})
			return
		} else {

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)

			response := map[string]string{
				"shortCode": shortUrl,
			}
			json.NewEncoder(w).Encode(response)
		}
	}

}

func (app *App) StreamMetrics(w http.ResponseWriter, r *http.Request) {

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrorResponse{"Metrics Streaming is currently unsupported."})
		return
	}

	w.Header().Add("Content-Type", "text/event-stream")
	w.Header().Add("Cache-Control", "no-cache")
	w.Header().Add("Connection", "keep-alive")

	metricsTicker := time.NewTicker(time.Second)
	defer metricsTicker.Stop()
	errorCount := 0

	for {
		select {
		case <-metricsTicker.C:
			globalTokenBucketCap := app.globalRateLimiter.bucket.Cap()
			globalTokensUsed := globalTokenBucketCap - app.globalRateLimiter.bucket.Len()
			activeUsers := app.perClientRateLimiter.timeLogStore.Len()
			currentUrlCount := app.shortener.Len()

			jsonData, err := json.Marshal(&Metrics{globalTokenBucketCap, globalTokensUsed, activeUsers, currentUrlCount})
			if err != nil {
				errorCount++
				if errorCount > 2 {
					json.NewEncoder(w).Encode(&ErrorResponse{"Metrics Streaming is currently unsupported."})
					return
				}
			}

			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()

		case <-r.Context().Done():
			fmt.Println("Client closed connection. Stopping Metrics stream.")
			return
		}
	}

}

func StressTest(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrorResponse{"Unexpected error occured while running tests. Please try again later."})
		return
	}

	w.Header().Add("Content-Type", "text/event-stream")
	w.Header().Add("Cache-Control", "no-cache")
	w.Header().Add("Connection", "keep-alive")

	if testServer, app, err := StartTestServer(); err != nil {
		fmt.Fprintf(w, "data: {\"error\": \"Failed to start test server. Please try again later.\"}\n\n")
		flusher.Flush()
		return
	} else {
		defer app.shortener.Offline()
		defer app.perClientRateLimiter.Offline()
		defer app.globalRateLimiter.Offline()
		defer testServer.Shutdown(context.Background())

		serverStopUnexpected := make(chan struct{})

		go func() {
			if err := testServer.ListenAndServe(); err != http.ErrServerClosed {
				close(serverStopUnexpected)
			}
		}()

		testCommand := exec.Command("./production_stress_test.sh")
		stdout, err := testCommand.StdoutPipe()
		if err != nil {
			fmt.Println(err)
			fmt.Fprintf(w, "data: {\"error\": \"Unexpected error occured while running tests. Please try again later.\"}\n\n")
			flusher.Flush()
			return
		}

		testCommand.Stderr = testCommand.Stdout

		if err := testCommand.Start(); err != nil {
			fmt.Fprintf(w, "data: {\"error\": \"Unexpected error occured while running tests. Please try again later.\"}\n\n")
			flusher.Flush()
			return
		}

		scanner := bufio.NewScanner(stdout)

		for scanner.Scan() {
			select {

			case <-r.Context().Done():
				fmt.Println("Client closed connection. Killing test...")
				testCommand.Process.Kill()
				return

			case <-serverStopUnexpected:
				fmt.Println("Test Server stopped unexpectedly")
				fmt.Fprintf(w, "data: {\"error\": \"Test server stoped unexpectedly. Please try again later.\"}\n\n")
				flusher.Flush()
				testCommand.Process.Kill()
				return

			default:
				jsonData, err := json.Marshal(map[string]string{"outputLine": scanner.Text()})
				if err != nil {
					fmt.Fprintf(w, "data: {\"error\": \"Unexpected error occured while  reading test output. Please try again later.\"}\n\n")
					flusher.Flush()
					return
				}

				fmt.Fprintf(w, "data: %s\n\n", jsonData)
				flusher.Flush()
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Println("Test Server stopped unexpectedly")
			fmt.Fprintf(w, "data: {\"error\": \"Unexpected error occured while reading test output. Please try again later.\"}\n\n")
			flusher.Flush()
			return
		}

		if err := testCommand.Wait(); err != nil {
			fmt.Fprintf(w, "data: {\"error\": \"Unexpected error occured while running tests. Please try again later.\"}\n\n")
			flusher.Flush()
			return
		}
	}

}
