package main

import (
	"database/sql"
	"fmt"
	"sort"
)

func initDB(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS players (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS matches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL UNIQUE,
    player1_id INTEGER NOT NULL,
    player2_id INTEGER NOT NULL,
    pool TEXT,
    stage TEXT NOT NULL,
    round INTEGER NOT NULL DEFAULT 0,
    score1 INTEGER NOT NULL DEFAULT 0,
    score2 INTEGER NOT NULL DEFAULT 0,
    finished INTEGER NOT NULL DEFAULT 0,
    current_match INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY(player1_id) REFERENCES players(id),
    FOREIGN KEY(player2_id) REFERENCES players(id)
);
`)
	if err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return ensureMatchColumns(db)
}

func ensureMatchColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(matches)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	roundExists := false
	currentExists := false
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == "round" {
			roundExists = true
		}
		if name == "current_match" {
			currentExists = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !roundExists {
		if _, err := db.Exec(`ALTER TABLE matches ADD COLUMN round INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !currentExists {
		if _, err := db.Exec(`ALTER TABLE matches ADD COLUMN current_match INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return nil
}

func getPlayers(db *sql.DB) ([]Player, error) {
	rows, err := db.Query(`SELECT id, name FROM players ORDER BY id ASC`)
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

func addPlayer(db *sql.DB, name string) (Player, error) {
	res, err := db.Exec(`INSERT INTO players (name) VALUES (?)`, name)
	if err != nil {
		return Player{}, fmt.Errorf("insert player: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Player{}, fmt.Errorf("retrieve player id: %w", err)
	}

	player := Player{ID: int(id), Name: name}
	if err := rebuildMatches(db); err != nil {
		return player, fmt.Errorf("rebuild matches after add player: %w", err)
	}
	if err := updateSemifinalMatches(db); err != nil {
		return player, fmt.Errorf("update semifinal matches after add player: %w", err)
	}
	return player, nil
}

func getMatches(db *sql.DB) ([]MatchRecord, error) {
	rows, err := db.Query(`SELECT id, key, player1_id, player2_id, pool, stage, round, score1, score2, finished, current_match FROM matches ORDER BY current_match DESC, CASE stage WHEN 'pool' THEN 1 WHEN 'semi' THEN 2 WHEN 'final' THEN 3 ELSE 4 END, pool, round, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := []MatchRecord{}
	for rows.Next() {
		var match MatchRecord
		var finishedValue int
		var currentValue int
		if err := rows.Scan(&match.ID, &match.Key, &match.Player1ID, &match.Player2ID, &match.Pool, &match.Stage, &match.Round, &match.Score1, &match.Score2, &finishedValue, &currentValue); err != nil {
			return nil, err
		}
		match.Finished = finishedValue != 0
		match.Current = currentValue != 0
		matches = append(matches, match)
	}
	return matches, rows.Err()
}

func updateMatchScore(db *sql.DB, matchID, score1, score2 int) error {
	finished := 0
	if isFinished(score1, score2) {
		finished = 1
	}
	_, err := db.Exec(`UPDATE matches SET score1 = ?, score2 = ?, finished = ? WHERE id = ?`, score1, score2, finished, matchID)
	if err != nil {
		return fmt.Errorf("update match score: %w", err)
	}
	return nil
}

func setCurrentMatch(db *sql.DB, matchID int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE matches SET current_match = 0`); err != nil {
		tx.Rollback()
		return err
	}
	res, err := tx.Exec(`UPDATE matches SET current_match = 1 WHERE id = ?`, matchID)
	if err != nil {
		tx.Rollback()
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		tx.Rollback()
		return err
	}
	if rows == 0 {
		tx.Rollback()
		return fmt.Errorf("match not found")
	}
	return tx.Commit()
}

func clearCurrentMatch(db *sql.DB) error {
	_, err := db.Exec(`UPDATE matches SET current_match = 0 WHERE current_match = 1`)
	return err
}

func ensureMatch(db *sql.DB, key string, player1ID, player2ID int, pool, stage string, round int) error {
	var existingID int
	var existingP1 int
	var existingP2 int
	var existingRound int
	row := db.QueryRow(`SELECT id, player1_id, player2_id, round FROM matches WHERE key = ?`, key)
	switch err := row.Scan(&existingID, &existingP1, &existingP2, &existingRound); err {
	case sql.ErrNoRows:
		_, err := db.Exec(`INSERT INTO matches (key, player1_id, player2_id, pool, stage, round) VALUES (?, ?, ?, ?, ?, ?)`, key, player1ID, player2ID, pool, stage, round)
		return err
	case nil:
		if existingP1 != player1ID || existingP2 != player2ID || existingRound != round {
			_, err := db.Exec(`UPDATE matches SET player1_id = ?, player2_id = ?, pool = ?, stage = ?, round = ?, score1 = 0, score2 = 0, finished = 0, current_match = 0 WHERE id = ?`, player1ID, player2ID, pool, stage, round, existingID)
			return err
		}
		return nil
	default:
		return err
	}
}

func rebuildMatches(db *sql.DB) error {
	players, err := getPlayers(db)
	if err != nil {
		return err
	}
	poolA, poolB := assignPools(players)

	if err := ensurePoolMatches(db, poolA, "A"); err != nil {
		return err
	}
	if err := ensurePoolMatches(db, poolB, "B"); err != nil {
		return err
	}

	if len(poolA) >= 2 && len(poolB) >= 2 {
		standings, err := computePoolStandings(players, []MatchRecord{})
		if err != nil {
			return err
		}
		topA := topN(standings["A"], 2)
		topB := topN(standings["B"], 2)
		if len(topA) == 2 && len(topB) == 2 {
			if err := ensureMatch(db, "semi-A1B2", topA[0].Player.ID, topB[1].Player.ID, "", "semi", 0); err != nil {
				return err
			}
			if err := ensureMatch(db, "semi-B1A2", topB[0].Player.ID, topA[1].Player.ID, "", "semi", 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateSemifinalMatches(db *sql.DB) error {
	players, err := getPlayers(db)
	if err != nil {
		return err
	}
	matches, err := getMatches(db)
	if err != nil {
		return err
	}
	standingPools, err := computePoolStandings(players, matches)
	if err != nil {
		return err
	}
	topA := topN(standingPools["A"], 2)
	topB := topN(standingPools["B"], 2)
	if len(topA) != 2 || len(topB) != 2 {
		return nil
	}
	if err := ensureMatch(db, "semi-A1B2", topA[0].Player.ID, topB[1].Player.ID, "", "semi", 0); err != nil {
		return err
	}
	if err := ensureMatch(db, "semi-B1A2", topB[0].Player.ID, topA[1].Player.ID, "", "semi", 0); err != nil {
		return err
	}
	return updateFinalMatch(db)
}

func updateFinalMatch(db *sql.DB) error {
	matches, err := getMatches(db)
	if err != nil {
		return err
	}
	type semiWinner struct {
		playerID int
		key      string
	}
	winners := []semiWinner{}
	for _, match := range matches {
		if match.Stage != "semi" || !match.Finished {
			continue
		}
		var winnerID int
		if match.Score1 > match.Score2 {
			winnerID = match.Player1ID
		} else if match.Score2 > match.Score1 {
			winnerID = match.Player2ID
		} else {
			continue
		}
		winners = append(winners, semiWinner{playerID: winnerID, key: match.Key})
	}
	if len(winners) != 2 {
		if _, err := db.Exec(`DELETE FROM matches WHERE key = ?`, "final"); err != nil {
			return err
		}
		return nil
	}
	sort.Slice(winners, func(i, j int) bool {
		return winners[i].key < winners[j].key
	})
	return ensureMatch(db, "final", winners[0].playerID, winners[1].playerID, "", "final", 0)
}

func ensurePoolMatches(db *sql.DB, pool []Player, poolName string) error {
	for _, match := range schedulePoolMatches(pool) {
		key := fmt.Sprintf("pool-%s-%d-%d", poolName, match.Player1ID, match.Player2ID)
		if err := ensureMatch(db, key, match.Player1ID, match.Player2ID, poolName, "pool", match.Round); err != nil {
			return err
		}
	}
	return nil
}

func resetTournament(db *sql.DB) error {
	if _, err := db.Exec(`DELETE FROM matches`); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM players`); err != nil {
		return err
	}
	return nil
}

func isFinished(score1, score2 int) bool {
	delta := score1 - score2
	if delta < 0 {
		delta = -delta
	}
	if score1 >= 11 || score2 >= 11 {
		return delta >= 2
	}
	return false
}

func topN(list []Standing, n int) []Standing {
	if len(list) < n {
		return list
	}
	return list[:n]
}
