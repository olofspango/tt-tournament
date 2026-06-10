package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

// Ad-hoc play sessions: a lightweight mode for casual 1v1 sets (e.g. lunch
// breaks) that is fully separate from the tournament tables, so using it
// never disturbs a tournament schedule.

type Session struct {
	ID        int    `json:"id"`
	StartedAt string `json:"started_at"`
}

type SessionGameRecord struct {
	ID          int
	SessionID   int
	Player1ID   int
	Player2ID   int
	Score1      int
	Score2      int
	Finished    bool
	FirstServer int
}

type SessionGameView struct {
	ID          int    `json:"id"`
	Player1     Player `json:"player1"`
	Player2     Player `json:"player2"`
	Score1      int    `json:"score1"`
	Score2      int    `json:"score2"`
	Finished    bool   `json:"finished"`
	FirstServer int    `json:"first_server"`
}

type SessionStanding struct {
	Player       Player `json:"player"`
	Played       int    `json:"played"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	ScoreFor     int    `json:"score_for"`
	ScoreAgainst int    `json:"score_against"`
	Diff         int    `json:"diff"`
}

type HeadToHead struct {
	Player1 Player `json:"player1"`
	Player2 Player `json:"player2"`
	Wins1   int    `json:"wins1"`
	Wins2   int    `json:"wins2"`
}

type PairSuggestion struct {
	Player1ID int `json:"player1_id"`
	Player2ID int `json:"player2_id"`
}

type SessionState struct {
	Session     *Session          `json:"session"`
	Players     []Player          `json:"players"`
	CurrentGame *SessionGameView  `json:"current_game"`
	Games       []SessionGameView `json:"games"`
	Standings   []SessionStanding `json:"standings"`
	HeadToHead  []HeadToHead      `json:"head_to_head"`
	Suggestion  *PairSuggestion   `json:"suggestion"`
}

func initSessionTables(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS session_players (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    UNIQUE(session_id, name),
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS session_games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    player1_id INTEGER NOT NULL,
    player2_id INTEGER NOT NULL,
    score1 INTEGER NOT NULL DEFAULT 0,
    score2 INTEGER NOT NULL DEFAULT 0,
    finished INTEGER NOT NULL DEFAULT 0,
    first_server INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY(player1_id) REFERENCES session_players(id),
    FOREIGN KEY(player2_id) REFERENCES session_players(id)
);
`)
	if err != nil {
		return fmt.Errorf("create session schema: %w", err)
	}
	return nil
}

func getActiveSession(db *sql.DB) (*Session, error) {
	var s Session
	row := db.QueryRow(`SELECT id, started_at FROM sessions ORDER BY id DESC LIMIT 1`)
	switch err := row.Scan(&s.ID, &s.StartedAt); err {
	case sql.ErrNoRows:
		return nil, nil
	case nil:
		return &s, nil
	default:
		return nil, err
	}
}

func createSession(db *sql.DB) (*Session, error) {
	if _, err := db.Exec(`INSERT INTO sessions DEFAULT VALUES`); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return getActiveSession(db)
}

func getSessionPlayers(db *sql.DB, sessionID int) ([]Player, error) {
	rows, err := db.Query(`SELECT id, name FROM session_players WHERE session_id = ? ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	players := []Player{}
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

func addSessionPlayer(db *sql.DB, sessionID int, name string) error {
	_, err := db.Exec(`INSERT INTO session_players (session_id, name) VALUES (?, ?)`, sessionID, name)
	if err != nil {
		return fmt.Errorf("add player: %w", err)
	}
	return nil
}

func getSessionGames(db *sql.DB, sessionID int) ([]SessionGameRecord, error) {
	rows, err := db.Query(`SELECT id, session_id, player1_id, player2_id, score1, score2, finished, first_server FROM session_games WHERE session_id = ? ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	games := []SessionGameRecord{}
	for rows.Next() {
		var g SessionGameRecord
		var finished int
		if err := rows.Scan(&g.ID, &g.SessionID, &g.Player1ID, &g.Player2ID, &g.Score1, &g.Score2, &finished, &g.FirstServer); err != nil {
			return nil, err
		}
		g.Finished = finished != 0
		games = append(games, g)
	}
	return games, rows.Err()
}

func (g SessionGameRecord) toView(players map[int]Player) (SessionGameView, bool) {
	p1, ok1 := players[g.Player1ID]
	p2, ok2 := players[g.Player2ID]
	if !ok1 || !ok2 {
		return SessionGameView{}, false
	}
	return SessionGameView{
		ID:          g.ID,
		Player1:     p1,
		Player2:     p2,
		Score1:      g.Score1,
		Score2:      g.Score2,
		Finished:    g.Finished,
		FirstServer: g.FirstServer,
	}, true
}

func computeSessionStandings(players []Player, games []SessionGameRecord) []SessionStanding {
	byID := map[int]*SessionStanding{}
	for _, p := range players {
		byID[p.ID] = &SessionStanding{Player: p}
	}
	for _, g := range games {
		if !g.Finished {
			continue
		}
		s1, ok1 := byID[g.Player1ID]
		s2, ok2 := byID[g.Player2ID]
		if !ok1 || !ok2 {
			continue
		}
		s1.Played++
		s2.Played++
		s1.ScoreFor += g.Score1
		s1.ScoreAgainst += g.Score2
		s2.ScoreFor += g.Score2
		s2.ScoreAgainst += g.Score1
		if g.Score1 > g.Score2 {
			s1.Wins++
			s2.Losses++
		} else if g.Score2 > g.Score1 {
			s2.Wins++
			s1.Losses++
		}
	}
	standings := make([]SessionStanding, 0, len(players))
	for _, p := range players {
		s := byID[p.ID]
		s.Diff = s.ScoreFor - s.ScoreAgainst
		standings = append(standings, *s)
	}
	sort.SliceStable(standings, func(i, j int) bool {
		if standings[i].Wins != standings[j].Wins {
			return standings[i].Wins > standings[j].Wins
		}
		if standings[i].Diff != standings[j].Diff {
			return standings[i].Diff > standings[j].Diff
		}
		return standings[i].Player.Name < standings[j].Player.Name
	})
	return standings
}

func computeHeadToHead(players []Player, games []SessionGameRecord) []HeadToHead {
	playerMap := playerMapOf(players)
	type pairKey struct{ low, high int }
	pairs := map[pairKey]*HeadToHead{}
	order := []pairKey{}
	for _, g := range games {
		if !g.Finished || g.Score1 == g.Score2 {
			continue
		}
		_, ok1 := playerMap[g.Player1ID]
		_, ok2 := playerMap[g.Player2ID]
		if !ok1 || !ok2 {
			continue
		}
		key := pairKey{low: g.Player1ID, high: g.Player2ID}
		if key.low > key.high {
			key.low, key.high = key.high, key.low
		}
		entry, exists := pairs[key]
		if !exists {
			low, high := playerMap[key.low], playerMap[key.high]
			entry = &HeadToHead{Player1: low, Player2: high}
			pairs[key] = entry
			order = append(order, key)
		}
		winnerID := g.Player1ID
		if g.Score2 > g.Score1 {
			winnerID = g.Player2ID
		}
		if winnerID == entry.Player1.ID {
			entry.Wins1++
		} else {
			entry.Wins2++
		}
	}
	result := make([]HeadToHead, 0, len(order))
	for _, key := range order {
		result = append(result, *pairs[key])
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Player1.Name != result[j].Player1.Name {
			return result[i].Player1.Name < result[j].Player1.Name
		}
		return result[i].Player2.Name < result[j].Player2.Name
	})
	return result
}

// suggestNextPair proposes who should play the next set: prefer the players
// who have rested the longest since their previous game, breaking ties
// towards the pair with fewer games played and fewer mutual meetings — the
// usual "rotate every set" lunch-break pattern.
func suggestNextPair(players []Player, games []SessionGameRecord) *PairSuggestion {
	if len(players) < 2 {
		return nil
	}
	const never = 1 << 30
	lastPlayed := map[int]int{}
	gamesPlayed := map[int]int{}
	meetings := map[[2]int]int{}
	slot := 0
	for _, g := range games {
		slot++
		lastPlayed[g.Player1ID] = slot
		lastPlayed[g.Player2ID] = slot
		gamesPlayed[g.Player1ID]++
		gamesPlayed[g.Player2ID]++
		key := [2]int{g.Player1ID, g.Player2ID}
		if key[0] > key[1] {
			key[0], key[1] = key[1], key[0]
		}
		meetings[key]++
	}
	next := slot + 1

	var best *PairSuggestion
	bestRest, bestSum, bestMeet := -1, 0, 0
	for i := 0; i < len(players); i++ {
		for j := i + 1; j < len(players); j++ {
			p1, p2 := players[i].ID, players[j].ID
			rest := never
			for _, p := range []int{p1, p2} {
				if last, ok := lastPlayed[p]; ok && next-last < rest {
					rest = next - last
				}
			}
			sum := gamesPlayed[p1] + gamesPlayed[p2]
			key := [2]int{p1, p2}
			meet := meetings[key]
			better := false
			switch {
			case best == nil:
				better = true
			case rest != bestRest:
				better = rest > bestRest
			case sum != bestSum:
				better = sum < bestSum
			default:
				better = meet < bestMeet
			}
			if better {
				best = &PairSuggestion{Player1ID: p1, Player2ID: p2}
				bestRest, bestSum, bestMeet = rest, sum, meet
			}
		}
	}
	return best
}

func getSessionState(db *sql.DB) (SessionState, error) {
	state := SessionState{
		Players:    []Player{},
		Games:      []SessionGameView{},
		Standings:  []SessionStanding{},
		HeadToHead: []HeadToHead{},
	}
	session, err := getActiveSession(db)
	if err != nil {
		return state, err
	}
	if session == nil {
		return state, nil
	}
	state.Session = session

	players, err := getSessionPlayers(db, session.ID)
	if err != nil {
		return state, err
	}
	state.Players = players

	games, err := getSessionGames(db, session.ID)
	if err != nil {
		return state, err
	}
	playerMap := playerMapOf(players)
	for _, g := range games {
		view, ok := g.toView(playerMap)
		if !ok {
			continue
		}
		if !g.Finished {
			current := view
			state.CurrentGame = &current
			continue
		}
		state.Games = append(state.Games, view)
	}
	// Newest finished set first in the log.
	for i, j := 0, len(state.Games)-1; i < j; i, j = i+1, j-1 {
		state.Games[i], state.Games[j] = state.Games[j], state.Games[i]
	}

	state.Standings = computeSessionStandings(players, games)
	state.HeadToHead = computeHeadToHead(players, games)
	if state.CurrentGame == nil {
		state.Suggestion = suggestNextPair(players, games)
	}
	return state, nil
}

func getCurrentSessionGame(db *sql.DB, sessionID int) (*SessionGameRecord, error) {
	var g SessionGameRecord
	var finished int
	row := db.QueryRow(`SELECT id, session_id, player1_id, player2_id, score1, score2, finished, first_server FROM session_games WHERE session_id = ? AND finished = 0 ORDER BY id DESC LIMIT 1`, sessionID)
	switch err := row.Scan(&g.ID, &g.SessionID, &g.Player1ID, &g.Player2ID, &g.Score1, &g.Score2, &finished, &g.FirstServer); err {
	case sql.ErrNoRows:
		return nil, nil
	case nil:
		g.Finished = finished != 0
		return &g, nil
	default:
		return nil, err
	}
}

/* ===== HTTP handlers ===== */

func handleSession(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state, err := getSessionState(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, state)
}

func handleSessionNew(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Optionally carry over the roster so a returning group can skip re-typing
	// names.
	var payload struct {
		KeepPlayers bool `json:"keep_players"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&payload)
	}
	previous, err := getActiveSession(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session, err := createSession(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if payload.KeepPlayers && previous != nil {
		players, err := getSessionPlayers(db, previous.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, p := range players {
			if err := addSessionPlayer(db, session.ID, p.Name); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusCreated)
}

func handleSessionPlayer(w http.ResponseWriter, r *http.Request, db *sql.DB) {
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
	session, err := getActiveSession(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session == nil {
		if session, err = createSession(db); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := addSessionPlayer(db, session.ID, payload.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusCreated)
}

func handleSessionGame(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	switch r.Method {
	case http.MethodPost:
		handleSessionGameStart(w, r, db)
	case http.MethodDelete:
		handleSessionGameDelete(w, r, db)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleSessionGameStart(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var payload struct {
		Player1ID int `json:"player1_id"`
		Player2ID int `json:"player2_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if payload.Player1ID <= 0 || payload.Player2ID <= 0 || payload.Player1ID == payload.Player2ID {
		http.Error(w, "pick two different players", http.StatusBadRequest)
		return
	}
	session, err := getActiveSession(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session == nil {
		http.Error(w, "no active session", http.StatusBadRequest)
		return
	}
	players, err := getSessionPlayers(db, session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	playerMap := playerMapOf(players)
	if _, ok := playerMap[payload.Player1ID]; !ok {
		http.Error(w, "unknown player", http.StatusBadRequest)
		return
	}
	if _, ok := playerMap[payload.Player2ID]; !ok {
		http.Error(w, "unknown player", http.StatusBadRequest)
		return
	}
	current, err := getCurrentSessionGame(db, session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if current != nil {
		http.Error(w, "a set is already in progress", http.StatusConflict)
		return
	}
	if _, err := db.Exec(`INSERT INTO session_games (session_id, player1_id, player2_id) VALUES (?, ?, ?)`, session.ID, payload.Player1ID, payload.Player2ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusCreated)
}

func handleSessionGameDelete(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var payload struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	session, err := getActiveSession(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session == nil {
		http.Error(w, "no active session", http.StatusBadRequest)
		return
	}
	res, err := db.Exec(`DELETE FROM session_games WHERE id = ? AND session_id = ?`, payload.ID, session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rows == 0 {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

func handleSessionScore(w http.ResponseWriter, r *http.Request, db *sql.DB) {
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
	session, err := getActiveSession(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session == nil {
		http.Error(w, "no active session", http.StatusBadRequest)
		return
	}
	game, err := getCurrentSessionGame(db, session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.Error(w, "no set in progress", http.StatusBadRequest)
		return
	}
	score1 := game.Score1 + payload.Delta1
	score2 := game.Score2 + payload.Delta2
	if score1 < 0 || score2 < 0 {
		http.Error(w, "invalid score", http.StatusBadRequest)
		return
	}
	finished := 0
	if isFinished(score1, score2) {
		finished = 1
	}
	if _, err := db.Exec(`UPDATE session_games SET score1 = ?, score2 = ?, finished = ? WHERE id = ?`, score1, score2, finished, game.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

// handleSessionGameReopen flips the most recently finished set back to "in
// progress" so a mis-tapped final point can be corrected.
func handleSessionGameReopen(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, err := getActiveSession(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session == nil {
		http.Error(w, "no active session", http.StatusBadRequest)
		return
	}
	current, err := getCurrentSessionGame(db, session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if current != nil {
		http.Error(w, "a set is already in progress", http.StatusConflict)
		return
	}
	res, err := db.Exec(`UPDATE session_games SET finished = 0 WHERE id = (SELECT id FROM session_games WHERE session_id = ? AND finished = 1 ORDER BY id DESC LIMIT 1)`, session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rows == 0 {
		http.Error(w, "no finished set to reopen", http.StatusBadRequest)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}

func handleSessionServer(w http.ResponseWriter, r *http.Request, db *sql.DB) {
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
	session, err := getActiveSession(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session == nil {
		http.Error(w, "no active session", http.StatusBadRequest)
		return
	}
	game, err := getCurrentSessionGame(db, session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if game == nil {
		http.Error(w, "no set in progress", http.StatusBadRequest)
		return
	}
	if _, err := db.Exec(`UPDATE session_games SET first_server = ? WHERE id = ?`, payload.Server, game.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	broadcastUpdate()
	w.WriteHeader(http.StatusNoContent)
}
