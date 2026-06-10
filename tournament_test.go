package main

import (
	"fmt"
	"testing"
)

func makePlayers(ids ...int) []Player {
	players := make([]Player, 0, len(ids))
	for _, id := range ids {
		players = append(players, Player{ID: id, Name: fmt.Sprintf("p%d", id)})
	}
	return players
}

func scheduledFromPools(poolA, poolB []Player) []ScheduledMatch {
	scheduled := []ScheduledMatch{}
	for _, pair := range schedulePoolMatches(poolA) {
		scheduled = append(scheduled, ScheduledMatch{
			Key:       fmt.Sprintf("pool-A-%d-%d", pair.Player1ID, pair.Player2ID),
			Player1ID: pair.Player1ID,
			Player2ID: pair.Player2ID,
			Pool:      "A",
			Round:     pair.Round,
		})
	}
	for _, pair := range schedulePoolMatches(poolB) {
		scheduled = append(scheduled, ScheduledMatch{
			Key:       fmt.Sprintf("pool-B-%d-%d", pair.Player1ID, pair.Player2ID),
			Player1ID: pair.Player1ID,
			Player2ID: pair.Player2ID,
			Pool:      "B",
			Round:     pair.Round,
		})
	}
	return scheduled
}

func TestComputePlayOrderVariousSizes(t *testing.T) {
	for _, total := range []int{8, 9, 10} {
		t.Run(fmt.Sprintf("%d_players", total), func(t *testing.T) {
			ids := []int{}
			for i := 1; i <= total; i++ {
				ids = append(ids, i)
			}
			poolA, poolB := assignPools(makePlayers(ids...))
			scheduled := scheduledFromPools(poolA, poolB)
			ordered := computePlayOrder(scheduled)

			if len(ordered) != len(scheduled) {
				t.Fatalf("expected %d matches in order, got %d", len(scheduled), len(ordered))
			}

			// Every scheduled match appears exactly once.
			seen := map[string]bool{}
			for _, m := range ordered {
				if seen[m.Key] {
					t.Fatalf("match %s scheduled twice", m.Key)
				}
				seen[m.Key] = true
			}

			// Nobody plays two matches back-to-back (always avoidable for
			// these pool sizes).
			for i := 1; i < len(ordered); i++ {
				prev := ordered[i-1]
				cur := ordered[i]
				for _, p := range []int{cur.Player1ID, cur.Player2ID} {
					if p == prev.Player1ID || p == prev.Player2ID {
						t.Errorf("player %d plays back-to-back at slots %d and %d", p, i, i+1)
					}
				}
			}

			// Playtime stays balanced as the evening progresses: after each
			// slot, within a pool no player should be more than 2 games ahead
			// of another.
			counts := map[int]int{}
			for _, p := range append(append([]Player{}, poolA...), poolB...) {
				counts[p.ID] = 0
			}
			poolOf := map[int]string{}
			for _, p := range poolA {
				poolOf[p.ID] = "A"
			}
			for _, p := range poolB {
				poolOf[p.ID] = "B"
			}
			for slot, m := range ordered {
				counts[m.Player1ID]++
				counts[m.Player2ID]++
				for _, pool := range []string{"A", "B"} {
					min, max := 1<<30, 0
					for id, c := range counts {
						if poolOf[id] != pool {
							continue
						}
						if c < min {
							min = c
						}
						if c > max {
							max = c
						}
					}
					if max-min > 2 {
						t.Errorf("after slot %d pool %s game counts are unbalanced (min=%d max=%d)", slot+1, pool, min, max)
					}
				}
			}
		})
	}
}

func TestComputePlayOrderDeterministic(t *testing.T) {
	poolA, poolB := assignPools(makePlayers(1, 2, 3, 4, 5, 6, 7, 8, 9))
	scheduled := scheduledFromPools(poolA, poolB)
	first := computePlayOrder(scheduled)
	second := computePlayOrder(scheduled)
	for i := range first {
		if first[i].Key != second[i].Key {
			t.Fatalf("ordering not deterministic at slot %d: %s vs %s", i, first[i].Key, second[i].Key)
		}
	}
}

func TestIsFinished(t *testing.T) {
	cases := []struct {
		s1, s2 int
		want   bool
	}{
		{0, 0, false},
		{11, 9, true},
		{11, 10, false},
		{12, 10, true},
		{10, 11, false},
		{9, 11, true},
		{15, 13, true},
		{11, 11, false},
	}
	for _, c := range cases {
		if got := isFinished(c.s1, c.s2); got != c.want {
			t.Errorf("isFinished(%d, %d) = %v, want %v", c.s1, c.s2, got, c.want)
		}
	}
}

func TestSchedulePoolMatchesIsFullRoundRobin(t *testing.T) {
	for _, size := range []int{3, 4, 5} {
		ids := []int{}
		for i := 1; i <= size; i++ {
			ids = append(ids, i)
		}
		matches := schedulePoolMatches(makePlayers(ids...))
		want := size * (size - 1) / 2
		if len(matches) != want {
			t.Errorf("pool of %d: expected %d matches, got %d", size, want, len(matches))
		}
		seen := map[string]bool{}
		for _, m := range matches {
			a, b := m.Player1ID, m.Player2ID
			if a > b {
				a, b = b, a
			}
			key := fmt.Sprintf("%d-%d", a, b)
			if seen[key] {
				t.Errorf("pool of %d: pairing %s appears twice", size, key)
			}
			seen[key] = true
		}
	}
}
