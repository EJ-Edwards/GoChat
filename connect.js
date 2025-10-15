// File: static/connect.js
document.addEventListener('DOMContentLoaded', () => {
  const usernameInput = document.getElementById('username');
  const messageInput = document.getElementById('message');
  const sendBtn = document.getElementById('send-btn');
  const messages = document.getElementById('all-messages');

  // create PIN controls if missing
  let pinControls = document.getElementById('pin-controls');
  if (!pinControls) {
    pinControls = document.createElement('div');
    pinControls.id = 'pin-controls';
    pinControls.innerHTML = `
      <label>Room PIN:</label>
      <input id="pin-input" type="text" placeholder="1234" />
      <button id="join-pin">Join</button>
      <button id="create-pin">Create</button>
    `;
    const container = document.querySelector('#chatbox-container');
    // insert before messages
    container.insertBefore(pinControls, messages);
  }

  const pinInput = document.getElementById('pin-input');
  const joinBtn = document.getElementById('join-pin');
  const createBtn = document.getElementById('create-pin');

  let ws = null;
  let currentPin = null;
  let reconnectTimeout = null;

  function append(text, type = 'normal') {
    const div = document.createElement('div');
    div.textContent = text;
    if (type === 'system') div.style.fontStyle = 'italic';
    messages.appendChild(div);
    messages.scrollTop = messages.scrollHeight;
  }

  // base URL uses current host so it works on Render and local
  function getWsBaseUrl() {
    if (window.location.hostname.includes('localhost')) {
      return 'ws://localhost:8080';
    }
    // use same host and scheme (wss when https)
    const scheme = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
    return scheme + window.location.host;
  }

  function connectToPin(pin) {
    if (!pin) return;

    // if already connected to same pin, skip
    if (ws && ws.readyState === WebSocket.OPEN && currentPin === pin) {
      append(`Already connected to room ${pin}`, 'system');
      return;
    }

    // close previous socket if exists
    if (ws) {
      try { ws.close(); } catch (e) { /* ignore */ }
    }
    if (reconnectTimeout) clearTimeout(reconnectTimeout);

    currentPin = pin;
    const url = getWsBaseUrl() + '/ws?pin=' + encodeURIComponent(pin);
    ws = new WebSocket(url);

    ws.addEventListener('open', () => append(`Connected to room ${pin}`, 'system'));
    ws.addEventListener('message', (ev) => append(ev.data));
    ws.addEventListener('close', () => {
      append(`Disconnected from room ${pin}`, 'system');
      ws = null;
      // auto-reconnect only if still on same pin
      if (currentPin === pin) {
        reconnectTimeout = setTimeout(() => {
          append('Reconnecting...', 'system');
          connectToPin(pin);
        }, 3000);
      }
    });
    ws.addEventListener('error', (err) => {
      console.error('WebSocket error:', err);
      append('WebSocket error', 'system');
    });
  }

  joinBtn.addEventListener('click', () => {
    const pin = pinInput.value.trim();
    if (!pin) {
      append('Enter a PIN to join', 'system');
      return;
    }
    connectToPin(pin);
  });

  createBtn.addEventListener('click', () => {
    const pin = Math.floor(1000 + Math.random() * 9000).toString();
    pinInput.value = pin;
    append(`Created room ${pin}`, 'system');
    connectToPin(pin);
  });

  sendBtn.addEventListener('click', () => {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      append('Not connected to a room', 'system');
      return;
    }
    const user = usernameInput.value.trim() || 'anon';
    const msg = messageInput.value.trim();
    if (!msg) return;
    ws.send(`${user}: ${msg}`);
    messageInput.value = '';
  });

  messageInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') sendBtn.click();
  });
});
