const pageType = document.body.dataset.page;
const wsUrl = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`;
let socket;

function createWebSocket() {
    socket = new WebSocket(wsUrl);
    socket.addEventListener('message', () => {
        if (pageType === 'dashboard') {
            loadStandings();
            loadDashboardCurrentGame();
        } else if (pageType === 'admin') {
            loadAdmin();
        } else if (pageType === 'live-score') {
            loadLiveScore();
        } else if (pageType === 'admin-live') {
            loadCurrentGame();
        }
    });
    socket.addEventListener('close', () => {
        setTimeout(createWebSocket, 1500);
    });
}

async function loadStandings() {
    const response = await fetch('/api/standings');
    if (!response.ok) {
        return;
    }
    const data = await response.json();
    renderPool('A', data.pools.A || []);
    renderPool('B', data.pools.B || []);
    renderSemifinals(data.semifinals || []);
    renderFinal(data.final);
}

async function loadDashboardCurrentGame() {
    const response = await fetch('/api/current-game');
    if (!response.ok) {
        return;
    }
    const data = await response.json();
    renderDashboardCurrentGame(data.match);
}

function renderDashboardCurrentGame(match) {
    const container = document.getElementById('dashboard-current-match');
    if (!container) return;
    if (!match) {
        container.innerHTML = '<div class="hero-match-empty">No active match selected yet.</div>';
        return;
    }
    container.innerHTML = `
      <div class="match-tag">Current live match</div>
      <div class="match-score-grid">
        <div class="player-summary">
          <span class="player-name">${match.player1.name}</span>
          <strong class="hero-score">${match.score1}</strong>
        </div>
        <div class="vs-badge">vs</div>
        <div class="player-summary">
          <span class="player-name">${match.player2.name}</span>
          <strong class="hero-score">${match.score2}</strong>
        </div>
      </div>
      <div class="hero-match-meta">${match.stage.toUpperCase()} ${match.pool ? match.pool + ' ' : ''}${match.round ? 'Round ' + match.round : ''} · ${match.finished ? 'Finished' : 'Live'}</div>
    `;
}

function renderPool(poolName, standings) {
    const tableBody = document.querySelector(`#pool-${poolName.toLowerCase()}-table tbody`);
    if (!tableBody) return;
    tableBody.innerHTML = '';
    standings.forEach((row) => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
      <td>${row.player.name}</td>
      <td>${row.points}</td>
      <td>${row.wins}</td>
      <td>${row.losses}</td>
      <td>${row.score_for}</td>
      <td>${row.score_against}</td>
      <td>${row.diff}</td>
    `;
        tableBody.appendChild(tr);
    });
}

function renderSemifinals(matches) {
    const container = document.getElementById('semifinals');
    if (!container) return;
    container.innerHTML = '';
    if (matches.length === 0) {
        container.innerHTML = '<p class="empty">Semi-finals will appear after the top two players in each pool are determined.</p>';
        return;
    }
    matches.forEach((match) => {
        const card = document.createElement('div');
        card.className = 'match-card';
        card.innerHTML = `
      <strong>${match.stage.toUpperCase()}</strong>
      <div>${match.player1.name} <span class="score">${match.score1}</span> vs <span class="score">${match.score2}</span> ${match.player2.name}</div>
    `;
        container.appendChild(card);
    });
}

function renderFinal(match) {
    const container = document.getElementById('final');
    if (!container) return;
    container.innerHTML = '';
    if (!match) {
        container.innerHTML = '<p class="empty">Final will appear once the semi-finals are complete.</p>';
        return;
    }
    const card = document.createElement('div');
    card.className = 'match-card';
    card.innerHTML = `
      <strong>${match.stage.toUpperCase()}</strong>
      <div>${match.player1.name} <span class="score">${match.score1}</span> vs <span class="score">${match.score2}</span> ${match.player2.name}</div>
    `;
    container.appendChild(card);
}

async function loadCurrentGame() {
    const response = await fetch('/api/current-game');
    if (!response.ok) {
        return;
    }
    const data = await response.json();
    renderCurrentGame(data.match);
}

async function loadLiveScore() {
    const response = await fetch('/api/current-game');
    if (!response.ok) {
        return;
    }
    const data = await response.json();
    renderLiveScore(data.match);
}

function renderCurrentGame(match) {
    const container = document.getElementById('current-match');
    if (!container) return;
    container.innerHTML = '';
    if (!match) {
        container.innerHTML = '<p class="empty">No current live match available.</p>';
        return;
    }
    const card = document.createElement('div');
    card.className = 'current-match-card';
    card.innerHTML = `
      <div class="match-meta">${match.stage.toUpperCase()} ${match.pool || ''} ${match.round ? 'Round ' + match.round : ''}</div>
      <div class="match-title">${match.player1.name} vs ${match.player2.name}</div>
      <div class="score-board">
        <div class="player-score">
          <span>${match.player1.name}</span>
          <strong id="player1-score">${match.score1}</strong>
          <div class="score-controls">
            <button id="player1-decrement">-</button>
            <button id="player1-increment">+</button>
          </div>
        </div>
        <div class="player-score">
          <span>${match.player2.name}</span>
          <strong id="player2-score">${match.score2}</strong>
          <div class="score-controls">
            <button id="player2-decrement">-</button>
            <button id="player2-increment">+</button>
          </div>
        </div>
      </div>
      <p class="match-status">${match.finished ? 'Finished' : 'Live'}</p>
    `;
    container.appendChild(card);

    document.getElementById('player1-increment').addEventListener('click', () => updateCurrentGameScore(1, 0));
    document.getElementById('player1-decrement').addEventListener('click', () => updateCurrentGameScore(-1, 0));
    document.getElementById('player2-increment').addEventListener('click', () => updateCurrentGameScore(0, 1));
    document.getElementById('player2-decrement').addEventListener('click', () => updateCurrentGameScore(0, -1));
}

function renderLiveScore(match) {
    const container = document.getElementById('live-current-match');
    if (!container) return;
    container.innerHTML = '';
    if (!match) {
        container.innerHTML = '<p class="empty">No current live match available.</p>';
        return;
    }
    const card = document.createElement('div');
    card.className = 'live-score-display';
    card.innerHTML = `
      <div class="live-score-title">${match.player1.name} - ${match.player2.name}</div>
      <div class="live-score-score">${match.score1} - ${match.score2}</div>
    `;
    container.appendChild(card);
}

async function updateCurrentGameScore(delta1, delta2) {
    const response = await fetch('/api/current-game/score', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ delta1: delta1, delta2: delta2 }),
    });
    const feedback = document.getElementById('current-match-feedback');
    if (response.ok) {
        feedback.textContent = 'Score updated.';
        feedback.className = 'feedback success';
        setTimeout(() => { feedback.textContent = ''; }, 2000);
        loadCurrentGame();
        return;
    }
    const text = await response.text();
    feedback.textContent = `Unable to update score: ${text}`;
    feedback.className = 'feedback error';
}

async function loadAdmin() {
    await Promise.all([fetchPlayers(), fetchMatches()]);
}

async function fetchPlayers() {
    const response = await fetch('/api/players');
    if (!response.ok) {
        return;
    }
    const players = await response.json();
    const tbody = document.querySelector('#players-table tbody');
    tbody.innerHTML = '';
    players.forEach((player) => {
        const tr = document.createElement('tr');
        tr.innerHTML = `<td>${player.id}</td><td>${player.name}</td>`;
        tbody.appendChild(tr);
    });
}

async function fetchMatches() {
    const response = await fetch('/api/matches');
    if (!response.ok) {
        return;
    }
    const matches = await response.json();
    const tbody = document.querySelector('#matches-table tbody');
    tbody.innerHTML = '';
    matches.forEach((match) => {
        const tr = document.createElement('tr');
        tr.dataset.matchId = match.id;
        tr.innerHTML = `
      <td>${match.id}</td>
      <td>${match.stage}</td>
      <td>${match.pool || '-'}</td>
      <td>${match.round || '-'}</td>
      <td>${match.player1.name}</td>
      <td>
        <input type="number" min="0" value="${match.score1}" data-field="score1" class="score-input" />
        :
        <input type="number" min="0" value="${match.score2}" data-field="score2" class="score-input" />
      </td>
      <td>${match.player2.name}</td>
      <td>
        <button class="select-current-match" data-match-id="${match.id}">${match.current ? 'Current' : 'Select'}</button>
      </td>
    `;
        tbody.appendChild(tr);
    });
    document.querySelectorAll('.select-current-match').forEach((button) => {
        button.addEventListener('click', async (event) => {
            const matchId = Number(event.currentTarget.dataset.matchId);
            await selectCurrentMatch(matchId);
        });
    });
}

async function saveAllScores() {
    const rows = Array.from(document.querySelectorAll('#matches-table tbody tr'));
    const matches = rows.map((row) => {
        const id = Number(row.dataset.matchId);
        const score1 = Number(row.querySelector('[data-field="score1"]').value);
        const score2 = Number(row.querySelector('[data-field="score2"]').value);
        return { id, score1, score2 };
    });

    const response = await fetch('/api/matches', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ matches }),
    });
    const feedback = document.getElementById('match-feedback');
    if (response.ok) {
        feedback.textContent = 'Scores saved successfully.';
        feedback.className = 'feedback success';
        setTimeout(() => { feedback.textContent = ''; }, 2500);
        loadAdmin();
        return;
    }
    const text = await response.text();
    feedback.textContent = `Failed to save scores: ${text}`;
    feedback.className = 'feedback error';
}

async function resetTournament() {
    const confirmed = window.confirm('Resetting will remove all players and scores. Continue?');
    if (!confirmed) {
        return;
    }
    const response = await fetch('/api/reset', {
        method: 'POST',
    });
    const feedback = document.getElementById('match-feedback');
    if (response.ok) {
        feedback.textContent = 'Tournament reset successfully.';
        feedback.className = 'feedback success';
        setTimeout(() => { feedback.textContent = ''; }, 2500);
        loadAdmin();
        return;
    }
    const text = await response.text();
    feedback.textContent = `Failed to reset tournament: ${text}`;
    feedback.className = 'feedback error';
}

async function selectCurrentMatch(matchId) {
    const response = await fetch('/api/current-game/select', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ match_id: matchId }),
    });
    const feedback = document.getElementById('match-feedback');
    if (response.ok) {
        feedback.textContent = 'Current match updated.';
        feedback.className = 'feedback success';
        setTimeout(() => { feedback.textContent = ''; }, 2500);
        loadAdmin();
        return;
    }
    const text = await response.text();
    feedback.textContent = `Failed to select current match: ${text}`;
    feedback.className = 'feedback error';
}

async function addPlayer(event) {
    event.preventDefault();
    const nameInput = document.getElementById('player-name');
    const name = nameInput.value.trim();
    const feedback = document.getElementById('player-feedback');
    if (!name) {
        feedback.textContent = 'Enter a player name.';
        feedback.className = 'feedback error';
        return;
    }
    const response = await fetch('/api/player', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
    });
    if (response.ok) {
        feedback.textContent = 'Player added successfully.';
        feedback.className = 'feedback success';
        nameInput.value = '';
        loadAdmin();
        return;
    }
    const text = await response.text();
    feedback.textContent = `Unable to add player: ${text}`;
    feedback.className = 'feedback error';
}

window.addEventListener('DOMContentLoaded', () => {
    createWebSocket();
    if (pageType === 'dashboard') {
        loadStandings();
        loadDashboardCurrentGame();
    }
    if (pageType === 'admin') {
        loadAdmin();
        document.getElementById('player-form').addEventListener('submit', addPlayer);
        document.getElementById('save-all-scores').addEventListener('click', saveAllScores);
        document.getElementById('reset-tournament').addEventListener('click', resetTournament);
    }
    if (pageType === 'admin-live') {
        loadCurrentGame();
    }
    if (pageType === 'live-score') {
        loadLiveScore();
    }
});
