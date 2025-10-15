// File: static/connect.js
document.addEventListener('DOMContentLoaded', () => {
  const usernameInput = document.getElementById('username');
  const messageInput = document.getElementById('message');
  const sendBtn = document.getElementById('send-btn');
  const messages = document.getElementById('all-messages');
  const title = document.getElementById('title2');

  // --- Safety check: ensure elements exist ---
  if (!usernameInput || !messageInput || !sendBtn || !messages) {
    console.error("❌ Chat elements missing — check your HTML IDs!");
    return;
  }

  // --- Create PIN controls if missing ---
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
    container.insertBefore(pinControls, messages);
  }

  const pinInput = document.getElementById('pin-input');
  const joinBtn = document.getElementById('join-pin');
  const createBtn = document.getElementById('create-pin');

  let ws = null;
  let currentPin = null;
  let reconnectTimeout = null;
  let retryCount = 0;
  const maxRetries = 5;

  // --- Helper: Append message to chat ---
  function append(text, type = 'normal') {
    const div = document.createElement('div');
    div.className = type === 'system' ? 'system-msg' : 'user-msg';
    div.textContent = text;
    messages.appendChild(div);
    messages.scrollTop = messages.scrollHeight;
  }

  // --- Helper: Get WebSocket base URL ---
  function getWsBaseUrl() {
    if (window.location.hostname.includes('localhost')) {
      return 'ws://localhost:8080';
    }
    // For Render or any HTTPS host → use wss
    return 'wss://gochat-tz6u.onrender.com';
  }

  // --- Connect to a chat room (by PIN) ---
  function connectToPin(pin) {
    if (!pin) return;

    // If already connected to same room, skip
    if (ws && ws.readyState === WebSocket.OPEN && currentPin === pin) {
      append(`Already connected to room ${pin}`, 'system');
      return;
    }

    // Close existing socket before reconnecting
    if (ws) {
      try { ws.close(); } catch (e) {}
    }
    if (reconnectTimeout) clearTimeout(reconnectTimeout);

    currentPin = pin;
    const url = `${getWsBaseUrl()}/ws?pin=${encodeURIComponent(pin)}`;
    console.log(`🌐 Connecting to: ${url}`);
    ws = new WebSocket(url);

    ws.addEventListener('open', () => {
      retryCount = 0;
      append(`✅ Connected to room ${pin}`, 'system');
      if (title) title.textContent = `Room ${pin}`;
      console.log("Connected successfully!");
    });

    ws.addEventListener('message', (ev) => append(ev.data));

    ws.addEventListener('close', (e) => {
      append(`⚠️ Disconnected from room ${pin}`, 'system');
      console.log(`WebSocket closed: code=${e.code}, reason=${e.reason}`);
      ws = null;
      if (retryCount < maxRetries) {
        retryCount++;
        append(`Reconnecting... (attempt ${retryCount}/${maxRetries})`, 'system');
        reconnectTimeout = setTimeout(() => connectToPin(pin), 3000);
      } else {
        append('❌ Max reconnection attempts reached.', 'system');
      }
    });

    ws.addEventListener('error', (err) => {
      console.error('WebSocket error:', err);
      append('❌ WebSocket error — check console.', 'system');
    });
  }

  // --- Button Events ---
  joinBtn.addEventListener('click', () => {
    const pin = pinInput.value.trim();
    if (!pin) {
      append('Enter a PIN to join a room.', 'system');
      return;
    }
    append(`Joining room ${pin}...`, 'system');
    connectToPin(pin);
  });

  createBtn.addEventListener('click', () => {
    const pin = Math.floor(1000 + Math.random() * 9000).toString();
    pinInput.value = pin;
    append(`🆕 Created room ${pin}`, 'system');
    connectToPin(pin);
  });

  sendBtn.addEventListener('click', () => {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      append('Not connected to any room.', 'system');
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
