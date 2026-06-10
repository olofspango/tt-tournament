# Table Tennis Tournament Tracker

A simple Go web application for managing a table tennis tournament.

## Features

- Add and remove players via `/admin`
- Smart match scheduling: all pool matches are arranged into a single play
  queue that maximises rest between each player's games (nobody plays
  back-to-back) and keeps everyone's game count balanced through the evening
- Automatic pool standings for two pools
- Top 2 in each pool advance to semi-finals (created once all pool matches
  are played), winners meet in the final
- Live score admin (`/admin-live`) with big +/− buttons, first-server
  selection, winner banner and a "Start next match" button that advances the
  queue
- Live score display (`/live-score`) for a wall screen/iPad: huge score,
  serve indicator, match point alert, winner banner and "up next" preview
- Dashboard (`/`) with standings, live match and the upcoming match queue
- Ad-hoc play mode (`/play`) for casual lunch-break sets, separate from the
  tournament
- Live updates everywhere via WebSockets
- SQLite storage and Docker-ready deployment

## Run using Docker

```bash
docker compose up --build
```

Open:

- Dashboard (standings iPad): <http://localhost:8080/>
- Live score (score iPad): <http://localhost:8080/live-score>
- Admin: <http://localhost:8080/admin>
- Live score admin: <http://localhost:8080/admin-live>

## Running a tournament

1. Add all players in `/admin` **before** starting (roster changes rebuild
   the schedule and remove affected matches).
2. Open `/` on one iPad and `/live-score` on the other.
3. On `/admin-live`: tap the first server, use +/− to score points. A game is
   won at 11 points with a 2-point margin.
4. When a game ends, press **Start next match** — the queue advances
   automatically and both displays follow.
5. After the last pool game the semi-finals appear (pool winner vs the other
   pool's runner-up), then the final.

## Ad-hoc play (lunch-break mode)

Open `/play` when a few of you just want to rotate 1v1 sets without a
tournament:

1. Add the players who are in today (the roster is per session and separate
   from the tournament roster).
2. Tap two names — the app pre-selects a suggested pairing based on who has
   rested the longest — and press **Start set**.
3. Score with the +/− buttons; a set finishes automatically at 11 points
   with a 2-point margin. **Undo** reopens the last set if a tap went wrong.
4. The session tally shows set wins per player and head-to-head results
   between each pair.
5. **New session** archives the current tally and starts fresh (optionally
   keeping the same players).

## Notes

- The app stores data in `./data/tournament.db`; `DATABASE_PATH` and `PORT`
  environment variables override the defaults
- No authorization is implemented; the admin page is publicly accessible
- Pool splitting is deterministic: first half of the roster (by ID) is
  Pool A, second half is Pool B
- Pool standings tiebreakers: points, then point difference, then points for
