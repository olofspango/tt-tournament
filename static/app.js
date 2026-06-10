const pageType = document.body.dataset.page;
const wsUrl = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`;
let socket;

function refresh() {
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
}

function createWebSocket() {
    socket = new WebSocket(wsUrl);
    socket.addEventListener('open', refresh);
    socket.addEventListener('message', refresh);
    socket.addEventListener('close', () => {
        setTimeout(createWebSocket, 1500);
    });
}

/* ===== helpers ===== */

function esc(text) {
    const div = document.createElement('div');
    div.textContent = text == null ? '' : String(text);
    return div.innerHTML;
}

function stageLabel(match) {
    if (match.stage === 'pool') {
        return `Pool ${match.pool} · Game ${match.play_order}`;
    }
    if (match.stage === 'semi') return 'Semi-final';
    if (match.stage === 'final') return 'Final';
    return match.stage;
}

// Table tennis serving: switch every 2 points, every point from 10-10.
function currentServerOf(match) {
    if (!match || !match.first_server || match.finished) return 0;
    const total = match.score1 + match.score2;
    const firstServes = (match.score1 >= 10 && match.score2 >= 10)
        ? total % 2 === 0
        : Math.floor(total / 2) % 2 === 0;
    return firstServes ? match.first_server : (match.first_server === 1 ? 2 : 1);
}

function matchPointFor(match) {
    if (!match || match.finished) return 0;
    if (match.score1 >= 10 && match.score1 - match.score2 >= 1) return 1;
    if (match.score2 >= 10 && match.score2 - match.score1 >= 1) return 2;
    return 0;
}

function winnerOf(match) {
    if (!match || !match.finished) return null;
    if (match.score1 === match.score2) return null;
    return match.score1 > match.score2 ? match.player1 : match.player2;
}

/* ===== dashboard ===== */

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
    renderUpNext(data.up_next || []);
}

function renderDashboardCurrentGame(match) {
    const container = document.getElementById('dashboard-current-match');
    if (!container) return;
    if (!match) {
        container.innerHTML = '<div class="hero-match-empty">No active match selected yet.</div>';
        return;
    }
    const winner = winnerOf(match);
    const mp = matchPointFor(match);
    let status = match.finished ? 'Finished' : 'Live';
    if (winner) status = `🏆 ${esc(winner.name)} wins`;
    else if (mp) status = `🔥 Match point — ${esc(mp === 1 ? match.player1.name : match.player2.name)}`;
    container.innerHTML = `
      <div class="match-tag">${match.finished ? '' : '<span class="live-dot"></span>'}${match.finished ? 'Last result' : 'Live match'}</div>
      <div class="match-score-grid">
        <div class="player-summary">
          <span class="player-name">${esc(match.player1.name)}</span>
          <strong class="hero-score">${match.score1}</strong>
        </div>
        <div class="vs-badge">vs</div>
        <div class="player-summary">
          <span class="player-name">${esc(match.player2.name)}</span>
          <strong class="hero-score">${match.score2}</strong>
        </div>
      </div>
      <div class="hero-match-meta">${stageLabel(match)} · ${status}</div>
    `;
}

function renderUpNext(matches) {
    const container = document.getElementById('up-next');
    if (!container) return;
    if (!matches.length) {
        container.innerHTML = '<p class="empty">No more matches scheduled.</p>';
        return;
    }
    container.innerHTML = matches.map((m, idx) => `
      <div class="up-next-item${idx === 0 ? ' up-next-first' : ''}">
        <span class="up-next-order">${idx === 0 ? 'NEXT' : '+' + idx}</span>
        <span class="up-next-players">${esc(m.player1.name)} <em>vs</em> ${esc(m.player2.name)}</span>
        <span class="up-next-meta">${stageLabel(m)}</span>
      </div>
    `).join('');
}

function renderPool(poolName, standings) {
    const tableBody = document.querySelector(`#pool-${poolName.toLowerCase()}-table tbody`);
    if (!tableBody) return;
    tableBody.innerHTML = '';
    standings.forEach((row) => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
      <td>${esc(row.player.name)}</td>
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

function knockoutMatchCard(match) {
    const winner = winnerOf(match);
    return `
      <strong>${stageLabel(match)}</strong>
      <div>${esc(match.player1.name)} <span class="score">${match.score1}</span> vs <span class="score">${match.score2}</span> ${esc(match.player2.name)}</div>
      ${winner ? `<div class="match-winner">🏆 ${esc(winner.name)}</div>` : ''}
    `;
}

function renderSemifinals(matches) {
    const container = document.getElementById('semifinals');
    if (!container) return;
    container.innerHTML = '';
    if (matches.length === 0) {
        container.innerHTML = '<p class="empty">Semi-finals appear when all pool matches are played. The pool winner meets the runner-up of the other pool.</p>';
        return;
    }
    matches.forEach((match) => {
        const card = document.createElement('div');
        card.className = 'match-card';
        card.innerHTML = knockoutMatchCard(match);
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
    card.innerHTML = knockoutMatchCard(match);
    container.appendChild(card);
}

/* ===== admin-live ===== */

async function loadCurrentGame() {
    const response = await fetch('/api/current-game');
    if (!response.ok) {
        return;
    }
    const data = await response.json();
    renderCurrentGame(data.match, data.up_next || []);
}

function renderCurrentGame(match, upNext) {
    const container = document.getElementById('current-match');
    if (!container) return;
    container.innerHTML = '';
    if (!match) {
        container.innerHTML = '<p class="empty">No current live match available.</p>';
        return;
    }
    const winner = winnerOf(match);
    const server = currentServerOf(match);
    const mp = matchPointFor(match);
    const next = upNext.length ? upNext[0] : null;
    const card = document.createElement('div');
    card.className = 'current-match-card';
    card.innerHTML = `
      <div class="match-meta">${stageLabel(match)}</div>
      <div class="match-title">${esc(match.player1.name)} vs ${esc(match.player2.name)}</div>
      ${winner ? `<div class="winner-banner">🏆 ${esc(winner.name)} wins ${Math.max(match.score1, match.score2)}–${Math.min(match.score1, match.score2)}</div>` : ''}
      <div class="score-board">
        <div class="player-score${server === 1 ? ' serving' : ''}">
          <span>${server === 1 ? '🏓 ' : ''}${esc(match.player1.name)}</span>
          <strong id="player1-score">${match.score1}</strong>
          <div class="score-controls">
            <button id="player1-decrement" class="secondary">−</button>
            <button id="player1-increment" ${match.finished ? 'disabled' : ''}>+</button>
          </div>
        </div>
        <div class="player-score${server === 2 ? ' serving' : ''}">
          <span>${server === 2 ? '🏓 ' : ''}${esc(match.player2.name)}</span>
          <strong id="player2-score">${match.score2}</strong>
          <div class="score-controls">
            <button id="player2-decrement" class="secondary">−</button>
            <button id="player2-increment" ${match.finished ? 'disabled' : ''}>+</button>
          </div>
        </div>
      </div>
      ${!winner ? `<p class="match-status">${mp ? '🔥 Match point — ' + esc(mp === 1 ? match.player1.name : match.player2.name) : 'Live'}</p>` : ''}
      <div class="serve-select">
        <span>First server:</span>
        <button id="first-server-1" class="${match.first_server === 1 ? '' : 'secondary'}">${esc(match.player1.name)}</button>
        <button id="first-server-2" class="${match.first_server === 2 ? '' : 'secondary'}">${esc(match.player2.name)}</button>
      </div>
      <div class="next-match-row">
        ${next ? `<div class="up-next-inline">Up next: <strong>${esc(next.player1.name)} vs ${esc(next.player2.name)}</strong> (${stageLabel(next)})</div>` : '<div class="up-next-inline">No more matches in the queue.</div>'}
        ${next ? `<button id="start-next-match" class="${winner ? '' : 'secondary'}">▶ Start next match</button>` : ''}
      </div>
    `;
    container.appendChild(card);

    document.getElementById('player1-increment').addEventListener('click', () => updateCurrentGameScore(1, 0));
    document.getElementById('player1-decrement').addEventListener('click', () => updateCurrentGameScore(-1, 0));
    document.getElementById('player2-increment').addEventListener('click', () => updateCurrentGameScore(0, 1));
    document.getElementById('player2-decrement').addEventListener('click', () => updateCurrentGameScore(0, -1));
    document.getElementById('first-server-1').addEventListener('click', () => setFirstServer(1));
    document.getElementById('first-server-2').addEventListener('click', () => setFirstServer(2));
    const nextButton = document.getElementById('start-next-match');
    if (nextButton) {
        nextButton.addEventListener('click', startNextMatch);
    }
}

async function updateCurrentGameScore(delta1, delta2) {
    const response = await fetch('/api/current-game/score', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ delta1: delta1, delta2: delta2 }),
    });
    const feedback = document.getElementById('current-match-feedback');
    if (response.ok) {
        feedback.textContent = '';
        loadCurrentGame();
        return;
    }
    const text = await response.text();
    feedback.textContent = `Unable to update score: ${text}`;
    feedback.className = 'feedback error';
}

async function setFirstServer(server) {
    const response = await fetch('/api/current-game/server', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ server }),
    });
    if (response.ok) {
        loadCurrentGame();
        return;
    }
    const feedback = document.getElementById('current-match-feedback');
    feedback.textContent = `Unable to set first server: ${await response.text()}`;
    feedback.className = 'feedback error';
}

async function startNextMatch() {
    const response = await fetch('/api/current-game/next', { method: 'POST' });
    if (response.ok) {
        loadCurrentGame();
        return;
    }
    const feedback = document.getElementById('current-match-feedback');
    feedback.textContent = `Unable to start next match: ${await response.text()}`;
    feedback.className = 'feedback error';
}

/* ===== live score display ===== */

async function loadLiveScore() {
    const response = await fetch('/api/current-game');
    if (!response.ok) {
        return;
    }
    const data = await response.json();
    renderLiveScore(data.match, data.up_next || []);
}

function renderLiveScore(match, upNext) {
    const container = document.getElementById('live-current-match');
    if (!container) return;
    container.innerHTML = '';
    if (!match) {
        container.innerHTML = '<p class="empty">No current live match available.</p>';
        return;
    }
    const winner = winnerOf(match);
    const server = currentServerOf(match);
    const mp = matchPointFor(match);
    const next = upNext.length ? upNext[0] : null;
    const card = document.createElement('div');
    card.className = 'live-score-display';
    card.innerHTML = `
      <div class="live-score-meta">${stageLabel(match)}</div>
      <div class="live-players">
        <div class="live-player">
          <div class="live-player-name">${server === 1 ? '<span class="serve-ball">🏓</span> ' : ''}${esc(match.player1.name)}</div>
          <div class="live-player-score">${match.score1}</div>
        </div>
        <div class="live-divider">–</div>
        <div class="live-player">
          <div class="live-player-name">${server === 2 ? '<span class="serve-ball">🏓</span> ' : ''}${esc(match.player2.name)}</div>
          <div class="live-player-score">${match.score2}</div>
        </div>
      </div>
      ${winner ? `<div class="live-winner">🏆 ${esc(winner.name)} wins!</div>` : ''}
      ${!winner && mp ? `<div class="live-matchpoint">MATCH POINT — ${esc(mp === 1 ? match.player1.name : match.player2.name)}</div>` : ''}
      ${next ? `<div class="live-upnext">Up next: ${esc(next.player1.name)} vs ${esc(next.player2.name)}</div>` : ''}
    `;
    container.appendChild(card);
}

/* ===== admin ===== */

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
        tr.innerHTML = `
      <td>${player.id}</td>
      <td>${esc(player.name)}</td>
      <td><button class="delete-player secondary danger" data-player-id="${player.id}" data-player-name="${esc(player.name)}">Remove</button></td>
    `;
        tbody.appendChild(tr);
    });
    document.querySelectorAll('.delete-player').forEach((button) => {
        button.addEventListener('click', async (event) => {
            const id = Number(event.currentTarget.dataset.playerId);
            const name = event.currentTarget.dataset.playerName;
            await removePlayer(id, name);
        });
    });
}

async function removePlayer(id, name) {
    const confirmed = window.confirm(`Remove ${name}? Their matches will be deleted and the schedule rebuilt.`);
    if (!confirmed) {
        return;
    }
    const response = await fetch('/api/player', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id }),
    });
    const feedback = document.getElementById('player-feedback');
    if (response.ok) {
        feedback.textContent = `${name} removed.`;
        feedback.className = 'feedback success';
        setTimeout(() => { feedback.textContent = ''; }, 2500);
        loadAdmin();
        return;
    }
    feedback.textContent = `Unable to remove player: ${await response.text()}`;
    feedback.className = 'feedback error';
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
        const winner = winnerOf(match);
        const tr = document.createElement('tr');
        tr.dataset.matchId = match.id;
        if (match.current) tr.classList.add('current-row');
        if (match.finished) tr.classList.add('finished-row');
        tr.innerHTML = `
      <td>${match.play_order || '-'}</td>
      <td>${match.stage === 'pool' ? 'Pool ' + match.pool : (match.stage === 'semi' ? 'Semi' : 'Final')}</td>
      <td>${esc(match.player1.name)}</td>
      <td class="score-cell">
        <input type="number" min="0" value="${match.score1}" data-field="score1" class="score-input" />
        :
        <input type="number" min="0" value="${match.score2}" data-field="score2" class="score-input" />
      </td>
      <td>${esc(match.player2.name)}</td>
      <td>${match.current ? '🔴 Live' : (winner ? '✓ ' + esc(winner.name) : '')}</td>
      <td>
        <button class="select-current-match ${match.current ? '' : 'secondary'}" data-match-id="${match.id}">${match.current ? 'Current' : 'Select'}</button>
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
    refresh();
    if (pageType === 'admin') {
        document.getElementById('player-form').addEventListener('submit', addPlayer);
        document.getElementById('save-all-scores').addEventListener('click', saveAllScores);
        document.getElementById('reset-tournament').addEventListener('click', resetTournament);
    }
});
