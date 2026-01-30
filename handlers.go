package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	cfg                  *Config
	logger               *slog.Logger
	shortener            UrlShortener
	globalRateLimiter    *GlobalRateLimiter
	perClientRateLimiter *PerClientRateLimiter
}

var page404HTMLText = Load404Page()

func (app *App) RetrieveUrl(w http.ResponseWriter, r *http.Request) {
	short := r.PathValue("shortUrl")

	if short != "" {
		original, err := app.shortener.RetrieveUrl(short)
		if err != nil {
			app.logger.Info("short URL not found", "short_url", short, "error", err)
			w.Header().Add("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			if page404HTMLText != "" {
				fmt.Fprintf(w, "%s", page404HTMLText)
			} else {
				fmt.Fprintf(w, app.cfg.Fallback404HTML)
			}
		} else {
			app.logger.Info("redirecting short URL", "short_url", short, "original_url", original)
			http.Redirect(w, r, original, http.StatusTemporaryRedirect)
		}
	} else {
		app.logger.Info("redirecting root to homepage")
		http.Redirect(w, r, "/", http.StatusPermanentRedirect)
	}

}

func (app *App) ShortenUrl(w http.ResponseWriter, r *http.Request) {
	var payload UrlShortenerPayload
	errorMessage := ""

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		app.logger.Warn("bad request: failed to decode URL shorten payload", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		errorMessage = "Oops, we couldn't process your request. Please try again later."
		json.NewEncoder(w).Encode(&ErrorResponse{errorMessage})
		return
	} else if message, ok := ValidateUrl(payload.Original, app.cfg); !ok {
		app.logger.Warn("bad request: invalid URL", "url", payload.Original, "validation_message", message)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrorResponse{message})
		return
	} else {
		shortUrl, err := Shorten(app.shortener, payload.Original)
		if err != nil {
			app.logger.Error("failed to shorten URL", "original_url", payload.Original, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			errorMessage = "Something broke on our end. Please try again later."
			json.NewEncoder(w).Encode(&ErrorResponse{errorMessage})
			return
		} else {
			app.logger.Info("URL shortened successfully", "original_url", payload.Original, "short_url", shortUrl)
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
	app.logger.Info("client connected to metrics stream", "remote_addr", r.RemoteAddr)
	defer app.logger.Info("client disconnected from metrics stream", "remote_addr", r.RemoteAddr)

	flusher, ok := w.(http.Flusher)
	if !ok {
		app.logger.Error("metrics streaming unsupported: http.Flusher not implemented", "remote_addr", r.RemoteAddr)
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
				app.logger.Error("failed to marshal metrics data", "error", err)
				errorCount++
				if errorCount > 2 {
					SendSSEErrorEvent(w, "Metrics Streaming is currently unavailable.", flusher)
					return
				}
			}

			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()

		case <-r.Context().Done():
			// The defer already logs disconnection
			return
		}
	}

}

func (app *App) StressTest(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("client connected to stress test stream", "remote_addr", r.RemoteAddr)
	defer app.logger.Info("client disconnected from stress test stream", "remote_addr", r.RemoteAddr)

	flusher, ok := w.(http.Flusher)
	if !ok {
		app.logger.Error("stress test streaming unsupported: http.Flusher not implemented", "remote_addr", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrorResponse{"Unexpected error occured while running tests. Please try again later."})
		return
	}

	w.Header().Add("Content-Type", "text/event-stream")
	w.Header().Add("Cache-Control", "no-cache")
	w.Header().Add("Connection", "keep-alive")

	if testServer, testApp, err := StartTestServer(app.cfg); err != nil {
		app.logger.Error("failed to start test server", "error", err)
		SendSSEErrorEvent(w, "Failed to start test server. Please try again later.", flusher)
		return
	} else {
		defer testApp.shortener.Offline()
		defer testApp.perClientRateLimiter.Offline()
		defer testApp.globalRateLimiter.Offline()
		defer testServer.Shutdown(context.Background())

		serverStopUnexpected := make(chan struct{})

		go func() {
			app.logger.Info("test server started", "addr", app.cfg.TestServerAddr)
			if err := testServer.ListenAndServe(); err != http.ErrServerClosed {
				app.logger.Error("test server stopped unexpectedly", "error", err)
				close(serverStopUnexpected)
			}
		}()

		testCommand := exec.Command("./production_stress_test.sh")
		stdout, err := testCommand.StdoutPipe()
		if err != nil {
			app.logger.Error("failed to get stdout pipe for stress test command", "error", err)
			SendSSEErrorEvent(w, "Unexpected error occured while running tests. Please try again later.", flusher)
			return
		}

		testCommand.Stderr = testCommand.Stdout

		app.logger.Info("starting stress test command", "command", testCommand.Args)
		if err := testCommand.Start(); err != nil {
			app.logger.Error("failed to start stress test command", "error", err)
			SendSSEErrorEvent(w, "Unexpected error occured while running tests. Please try again later.", flusher)
			return
		}

		scanner := bufio.NewScanner(stdout)

		for scanner.Scan() {
			select {
			case <-r.Context().Done():
				app.logger.Warn("client disconnected during stress test, killing test process", "remote_addr", r.RemoteAddr)
				testCommand.Process.Kill()
				return
			case <-serverStopUnexpected:
				app.logger.Error("test server stopped unexpectedly during stress test", "remote_addr", r.RemoteAddr)
				SendSSEErrorEvent(w, "Test server stoped unexpectedly. Please try again later.", flusher)
				testCommand.Process.Kill()
				return
			default:
				jsonData, err := json.Marshal(map[string]string{"outputLine": scanner.Text()})
				if err != nil {
					app.logger.Error("failed to marshal stress test output line", "error", err)
					SendSSEErrorEvent(w, "Unexpected error occured while reading test output. Please try again later.", flusher)
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", jsonData)
				flusher.Flush()
			}
		}

		if err := scanner.Err(); err != nil {
			app.logger.Error("error while scanning stress test output", "error", err)
			SendSSEErrorEvent(w, "Unexpected error occured while reading test output. Please try again later.", flusher)
			return
		}

		if err := testCommand.Wait(); err != nil {
			app.logger.Error("stress test command failed", "error", err)
			SendSSEErrorEvent(w, "Unexpected error occured while running tests. Please try again later.", flusher)
			return
		} else {
			app.logger.Info("stress test completed successfully", "remote_addr", r.RemoteAddr)
			fmt.Fprintf(w, "event: done\n")
			fmt.Fprintf(w, "data: {\"outputLine\": \"Tests completed successfully.\"}\n\n")
			flusher.Flush()
		}
	}
}
