package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
)

var hub *Hub

func main() {
	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		databasePath = "./data/tournament.db"
	}

	if err := os.MkdirAll(path.Dir(databasePath), 0755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=1", databasePath))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := initDB(db); err != nil {
		log.Fatalf("initialize database: %v", err)
	}

	if err := rebuildMatches(db); err != nil {
		log.Fatalf("rebuild matches: %v", err)
	}

	hub = NewHub()
	go hub.Run()

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, "static/index.html")
	})
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, "static/admin.html")
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWS(w, r)
	})
	mux.HandleFunc("/api/players", func(w http.ResponseWriter, r *http.Request) {
		handlePlayers(w, r, db)
	})
	mux.HandleFunc("/api/player", func(w http.ResponseWriter, r *http.Request) {
		handleAddPlayer(w, r, db)
	})
	mux.HandleFunc("/api/matches", func(w http.ResponseWriter, r *http.Request) {
		handleMatches(w, r, db)
	})
	mux.HandleFunc("/api/match", func(w http.ResponseWriter, r *http.Request) {
		handleMatchUpdate(w, r, db)
	})
	mux.HandleFunc("/api/reset", func(w http.ResponseWriter, r *http.Request) {
		handleReset(w, r, db)
	})
	mux.HandleFunc("/api/standings", func(w http.ResponseWriter, r *http.Request) {
		handleStandings(w, r, db)
	})
	mux.HandleFunc("/api/current-game", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentGame(w, r, db)
	})
	mux.HandleFunc("/api/current-game/select", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentGameSelect(w, r, db)
	})
	mux.HandleFunc("/api/current-game/score", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentGameScore(w, r, db)
	})
	mux.HandleFunc("/api/current-game/next", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentGameNext(w, r, db)
	})
	mux.HandleFunc("/api/current-game/server", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentGameServer(w, r, db)
	})
	mux.HandleFunc("/play", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, "static/play.html")
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		handleSession(w, r, db)
	})
	mux.HandleFunc("/api/session/new", func(w http.ResponseWriter, r *http.Request) {
		handleSessionNew(w, r, db)
	})
	mux.HandleFunc("/api/session/player", func(w http.ResponseWriter, r *http.Request) {
		handleSessionPlayer(w, r, db)
	})
	mux.HandleFunc("/api/session/game", func(w http.ResponseWriter, r *http.Request) {
		handleSessionGame(w, r, db)
	})
	mux.HandleFunc("/api/session/game/reopen", func(w http.ResponseWriter, r *http.Request) {
		handleSessionGameReopen(w, r, db)
	})
	mux.HandleFunc("/api/session/score", func(w http.ResponseWriter, r *http.Request) {
		handleSessionScore(w, r, db)
	})
	mux.HandleFunc("/api/session/server", func(w http.ResponseWriter, r *http.Request) {
		handleSessionServer(w, r, db)
	})
	mux.HandleFunc("/live-score", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, "static/live-score.html")
	})
	mux.HandleFunc("/admin-live", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, "static/admin-live.html")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Printf("tt-tracker listening on %s, database=%s", server.Addr, databasePath)
	log.Fatal(server.ListenAndServe())
}

func serveFile(w http.ResponseWriter, r *http.Request, filePath string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFile(w, r, filePath)
}

func handlePlayers(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	players, err := getPlayers(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, players)
}

func handleAddPlayer(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	switch r.Method {
	case http.MethodPost:
		var payload struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if payload.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if _, err := addPlayer(db, payload.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		broadcastUpdate()
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		var payload struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if payload.ID <= 0 {
			http.Error(w, "invalid player id", http.StatusBadRequest)
			return
		}
		if err := deletePlayer(db, payload.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		broadcastUpdate()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleMatches(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	switch r.Method {
	case http.MethodGet:
		matchRows, err := getMatches(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		players, err := getPlayers(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		playerMap := playerMapOf(players)
		matches := make([]MatchView, 0, len(matchRows))
		for _, row := range matchRows {
			if view, ok := row.toView(playerMap); ok {
				matches = append(matches, view)
			}
		}
		writeJSON(w, matches)
	case http.MethodPost:
		handleBatchMatchUpdate(w, r, db)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleBatchMatchUpdate(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var payload struct {
		Matches []struct {
			ID     int `json:"id"`
			Score1 int `json:"score1"`
			Score2 int `json:"score2"`
		} `json:"matches"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	for _, m := range payload.Matches {
		if m.ID <= 0 || m.Score1 < 0 || m.Score2 < 0 {
			http.Error(w, "invalid match or score", http.StatusBadRequest)
			return
		}
		if err := updateMatchScore(db, m.ID, m.Score1, m.Score2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := updateSemifinalMatches(db); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

func handleMatchUpdate(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ID     int `json:"id"`
		Score1 int `json:"score1"`
		Score2 int `json:"score2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if payload.ID <= 0 || payload.Score1 < 0 || payload.Score2 < 0 {
		http.Error(w, "invalid match or score", http.StatusBadRequest)
		return
	}
	if err := updateMatchScore(db, payload.ID, payload.Score1, payload.Score2); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := updateSemifinalMatches(db); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

func handleStandings(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	standings, err := getStandings(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, standings)
}

func handleCurrentGame(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	match, err := getCurrentGame(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	excludeID := 0
	if match != nil {
		excludeID = match.ID
	}
	upNext, err := getUpNext(db, 4, excludeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, struct {
		Match  *MatchView  `json:"match"`
		UpNext []MatchView `json:"up_next"`
	}{Match: match, UpNext: upNext})
}

// handleCurrentGameNext advances the live display to the next unfinished
// match in the play sequence.
func handleCurrentGameNext(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	current, err := getCurrentGame(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	excludeID := 0
	if current != nil {
		excludeID = current.ID
	}
	upNext, err := getUpNext(db, 1, excludeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(upNext) == 0 {
		if err := clearCurrentMatch(db); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if err := setCurrentMatch(db, upNext[0].ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

// handleCurrentGameServer records which player serves first in the current
// match so displays can show whose serve it is.
func handleCurrentGameServer(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Server int `json:"server"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if payload.Server != 1 && payload.Server != 2 {
		http.Error(w, "server must be 1 or 2", http.StatusBadRequest)
		return
	}
	match, err := getCurrentGame(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if match == nil {
		http.Error(w, "no active match", http.StatusBadRequest)
		return
	}
	if err := setFirstServer(db, match.ID, payload.Server); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

func handleCurrentGameSelect(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		MatchID int `json:"match_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if payload.MatchID <= 0 {
		http.Error(w, "invalid match id", http.StatusBadRequest)
		return
	}
	if err := setCurrentMatch(db, payload.MatchID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

func handleCurrentGameScore(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Delta1 int `json:"delta1"`
		Delta2 int `json:"delta2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	match, err := getCurrentGame(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if match == nil {
		http.Error(w, "no active match", http.StatusBadRequest)
		return
	}
	newScore1 := match.Score1 + payload.Delta1
	newScore2 := match.Score2 + payload.Delta2
	if newScore1 < 0 || newScore2 < 0 {
		http.Error(w, "invalid score", http.StatusBadRequest)
		return
	}
	if err := updateMatchScore(db, match.ID, newScore1, newScore2); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := updateSemifinalMatches(db); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	updatedMatch, err := getCurrentGame(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	writeJSON(w, struct {
		Match *MatchView `json:"match"`
	}{Match: updatedMatch})
}

func handleReset(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := resetTournament(db); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	hub.register <- conn

	go func() {
		defer func() {
			hub.unregister <- conn
			conn.Close()
		}()
		for {
			if _, _, err := conn.NextReader(); err != nil {
				break
			}
		}
	}()
}

func broadcastUpdate() {
	message := []byte(`{"type":"update"}`)
	hub.broadcast <- message
}

func handleError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
