# Table Tennis Tournament Tracker

A simple Go web application for managing a table tennis tournament.

## Features

- Add players via `/admin`
- Update match scores from the admin panel
- Automatic pool standings for two pools
- Top 2 in each pool advance to semi-finals
- Live dashboard updates using WebSockets
- SQLite storage and Docker-ready deployment

## Run using Docker

```bash
docker compose up --build
```

Open:

- Dashboard: <http://localhost:8080/>
- Admin: <http://localhost:8080/admin>

## Notes

- The app stores data in `./data/tournament.db`
- No authorization is implemented; the admin page is publicly accessible
- Pool splitting is deterministic and will evenly divide players into two groups
