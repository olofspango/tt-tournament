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
	mux.HandleFunc("/live-score", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, "static/live-score.html")
	})
	mux.HandleFunc("/admin-live", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, "static/admin-live.html")
	})

	server := &http.Server{
		Addr:         ":8080",
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
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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
}

func handleMatches(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	switch r.Method {
	case http.MethodGet:
		matchRows, err := getMatches(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		matches := make([]MatchView, 0, len(matchRows))
		players, _ := getPlayers(db)
		playerMap := map[int]Player{}
		for _, player := range players {
			playerMap[player.ID] = player
		}
		for _, row := range matchRows {
			matches = append(matches, MatchView{
				ID:       row.ID,
				Player1:  playerMap[row.Player1ID],
				Player2:  playerMap[row.Player2ID],
				Pool:     row.Pool,
				Stage:    row.Stage,
				Round:    row.Round,
				Score1:   row.Score1,
				Score2:   row.Score2,
				Finished: row.Finished,
				Current:  row.Current,
			})
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
	writeJSON(w, struct {
		Match *MatchView `json:"match"`
	}{Match: match})
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
