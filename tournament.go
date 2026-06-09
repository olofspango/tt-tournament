package main

import (
	"database/sql"
	"sort"
)

type Player struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type MatchRecord struct {
	ID        int    `json:"id"`
	Key       string `json:"key"`
	Player1ID int    `json:"player1_id"`
	Player2ID int    `json:"player2_id"`
	Pool      string `json:"pool"`
	Stage     string `json:"stage"`
	Round     int    `json:"round"`
	Score1    int    `json:"score1"`
	Score2    int    `json:"score2"`
	Finished  bool   `json:"finished"`
	Current   bool   `json:"current"`
}

type MatchView struct {
	ID       int    `json:"id"`
	Player1  Player `json:"player1"`
	Player2  Player `json:"player2"`
	Pool     string `json:"pool"`
	Stage    string `json:"stage"`
	Round    int    `json:"round"`
	Score1   int    `json:"score1"`
	Score2   int    `json:"score2"`
	Finished bool   `json:"finished"`
	Current  bool   `json:"current"`
}

type Standing struct {
	Player       Player `json:"player"`
	Pool         string `json:"pool"`
	Points       int    `json:"points"`
	Played       int    `json:"played"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	ScoreFor     int    `json:"score_for"`
	ScoreAgainst int    `json:"score_against"`
	Diff         int    `json:"diff"`
}

type StandingsResponse struct {
	Pools      map[string][]Standing `json:"pools"`
	Semifinals []MatchView           `json:"semifinals"`
	Final      *MatchView            `json:"final,omitempty"`
}

type RoundPair struct {
	Player1ID int
	Player2ID int
	Round     int
}

func assignPools(players []Player) ([]Player, []Player) {
	sorted := make([]Player, len(players))
	copy(sorted, players)
	// Keep the players in insertion order, which is ordered by ID.
	mid := len(sorted) / 2
	if len(sorted)%2 != 0 {
		mid++
	}
	return sorted[:mid], sorted[mid:]
}

func schedulePoolMatches(pool []Player) []RoundPair {
	ids := make([]int, 0, len(pool))
	for _, player := range pool {
		ids = append(ids, player.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	// If odd number of players, add a dummy placeholder to create an even schedule.
	dummy := -1
	odd := len(ids)%2 != 0
	if odd {
		ids = append(ids, dummy)
	}

	n := len(ids)
	rounds := n - 1
	matches := []RoundPair{}
	for round := 1; round <= rounds; round++ {
		for i := 0; i < n/2; i++ {
			p1 := ids[i]
			p2 := ids[n-1-i]
			if p1 == dummy || p2 == dummy {
				continue
			}
			matches = append(matches, RoundPair{Player1ID: p1, Player2ID: p2, Round: round})
		}
		// rotate all but the first element clockwise
		temp := ids[n-1]
		copy(ids[2:], ids[1:n-1])
		ids[1] = temp
	}
	return matches
}

func computePoolStandings(players []Player, matches []MatchRecord) (map[string][]Standing, error) {
	poolMap := map[string][]Player{}
	a, b := assignPools(players)
	if len(a) > 0 {
		poolMap["A"] = a
	}
	if len(b) > 0 {
		poolMap["B"] = b
	}

	standingByPlayer := map[int]Standing{}
	for _, poolName := range []string{"A", "B"} {
		for _, player := range poolMap[poolName] {
			standingByPlayer[player.ID] = Standing{Player: player, Pool: poolName}
		}
	}

	for _, match := range matches {
		if match.Stage != "pool" {
			continue
		}
		p1, ok1 := standingByPlayer[match.Player1ID]
		p2, ok2 := standingByPlayer[match.Player2ID]
		if !ok1 || !ok2 {
			continue
		}
		p1.Played++
		p2.Played++
		p1.ScoreFor += match.Score1
		p1.ScoreAgainst += match.Score2
		p2.ScoreFor += match.Score2
		p2.ScoreAgainst += match.Score1
		if match.Score1 > match.Score2 {
			p1.Wins++
			p2.Losses++
			p1.Points++
		} else if match.Score2 > match.Score1 {
			p2.Wins++
			p1.Losses++
			p2.Points++
		}
		standingByPlayer[p1.Player.ID] = p1
		standingByPlayer[p2.Player.ID] = p2
	}

	for id, standing := range standingByPlayer {
		standing.Diff = standing.ScoreFor - standing.ScoreAgainst
		standingByPlayer[id] = standing
	}

	pools := map[string][]Standing{}
	for _, poolName := range []string{"A", "B"} {
		if players, ok := poolMap[poolName]; ok {
			list := make([]Standing, 0, len(players))
			for _, player := range players {
				list = append(list, standingByPlayer[player.ID])
			}
			sortStandings(list)
			pools[poolName] = list
		}
	}

	return pools, nil
}

func sortStandings(list []Standing) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Points != list[j].Points {
			return list[i].Points > list[j].Points
		}
		if list[i].Diff != list[j].Diff {
			return list[i].Diff > list[j].Diff
		}
		if list[i].ScoreFor != list[j].ScoreFor {
			return list[i].ScoreFor > list[j].ScoreFor
		}
		return list[i].Player.Name < list[j].Player.Name
	})
}

func getStandings(db *sql.DB) (StandingsResponse, error) {
	players, err := getPlayers(db)
	if err != nil {
		return StandingsResponse{}, err
	}
	matches, err := getMatches(db)
	if err != nil {
		return StandingsResponse{}, err
	}
	pools, err := computePoolStandings(players, matches)
	if err != nil {
		return StandingsResponse{}, err
	}

	semifinals := []MatchView{}
	playerMap := map[int]Player{}
	for _, p := range players {
		playerMap[p.ID] = p
	}
	var finalMatch *MatchView
	for _, match := range matches {
		if match.Stage != "semi" {
			continue
		}
		if p1, ok1 := playerMap[match.Player1ID]; ok1 {
			if p2, ok2 := playerMap[match.Player2ID]; ok2 {
				semifinals = append(semifinals, MatchView{
					ID:       match.ID,
					Player1:  p1,
					Player2:  p2,
					Pool:     match.Pool,
					Stage:    match.Stage,
					Round:    match.Round,
					Score1:   match.Score1,
					Score2:   match.Score2,
					Finished: match.Finished,
				})
			}
		}
	}
	for _, match := range matches {
		if match.Stage != "final" {
			continue
		}
		if p1, ok1 := playerMap[match.Player1ID]; ok1 {
			if p2, ok2 := playerMap[match.Player2ID]; ok2 {
				finalMatch = &MatchView{
					ID:       match.ID,
					Player1:  p1,
					Player2:  p2,
					Pool:     match.Pool,
					Stage:    match.Stage,
					Round:    match.Round,
					Score1:   match.Score1,
					Score2:   match.Score2,
					Finished: match.Finished,
				}
				break
			}
		}
	}

	return StandingsResponse{Pools: pools, Semifinals: semifinals, Final: finalMatch}, nil
}

func matchStagePriority(stage string) int {
	switch stage {
	case "final":
		return 1
	case "semi":
		return 2
	case "pool":
		return 3
	default:
		return 4
	}
}

func getCurrentGame(db *sql.DB) (*MatchView, error) {
	players, err := getPlayers(db)
	if err != nil {
		return nil, err
	}
	matches, err := getMatches(db)
	if err != nil {
		return nil, err
	}
	playerMap := map[int]Player{}
	for _, p := range players {
		playerMap[p.ID] = p
	}

	for _, match := range matches {
		if match.Current {
			if p1, ok1 := playerMap[match.Player1ID]; ok1 {
				if p2, ok2 := playerMap[match.Player2ID]; ok2 {
					return &MatchView{
						ID:       match.ID,
						Player1:  p1,
						Player2:  p2,
						Pool:     match.Pool,
						Stage:    match.Stage,
						Round:    match.Round,
						Score1:   match.Score1,
						Score2:   match.Score2,
						Finished: match.Finished,
						Current:  true,
					}, nil
				}
			}
		}
	}

	unfinished := []MatchRecord{}
	for _, match := range matches {
		if !match.Finished {
			unfinished = append(unfinished, match)
		}
	}
	if len(unfinished) == 0 {
		return nil, nil
	}
	sort.SliceStable(unfinished, func(i, j int) bool {
		iPri := matchStagePriority(unfinished[i].Stage)
		jPri := matchStagePriority(unfinished[j].Stage)
		if iPri != jPri {
			return iPri < jPri
		}
		if unfinished[i].Round != unfinished[j].Round {
			return unfinished[i].Round < unfinished[j].Round
		}
		return unfinished[i].Key < unfinished[j].Key
	})
	match := unfinished[0]
	p1, ok1 := playerMap[match.Player1ID]
	p2, ok2 := playerMap[match.Player2ID]
	if !ok1 || !ok2 {
		return nil, nil
	}
	return &MatchView{
		ID:       match.ID,
		Player1:  p1,
		Player2:  p2,
		Pool:     match.Pool,
		Stage:    match.Stage,
		Round:    match.Round,
		Score1:   match.Score1,
		Score2:   match.Score2,
		Finished: match.Finished,
		Current:  false,
	}, nil
}
