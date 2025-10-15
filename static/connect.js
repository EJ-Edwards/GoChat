// File: static/connect.js
document.addEventListener('DOMContentLoaded', () => {
  const usernameInput = document.getElementById('username');
  const messageInput = document.getElementById('message');
  const sendBtn = document.getElementById('send-btn');
  const messages = document.getElementById('all-messages');
  const title = document.getElementById('title2');

  if (!usernameInput || !messageInput || !sendBtn || !messages) {
    console.error("❌ Chat elements missing — check your HTML IDs!");
    return;
  }

  // Prevent form submit reloads
  const form = document.querySelector('#typing-container form');
  if (form) form.addEventListener('submit', e => e.preventDefault());

  // Inject PIN controls if missing
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
  let heartbeatInterval = null;
  let retryCount = 0;
  const maxRetries = 5;

  // Append message helpers
  function append(text, type = 'normal') {
    const div = document.createElement('div');
    div.className = type === 'system' ? 'system-msg' : 'user-msg';
    div.textContent = text;
    messages.appendChild(div);
    messages.scrollTop = messages.scrollHeight;
  }

  // Prefer building URL from current origin to avoid cross-origin surprises
  function getWsUrl(pin) {
  const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const host = window.location.host; // e.g. yourapp.onrender.com
  return `${scheme}://${host}/ws?pin=${encodeURIComponent(pin)}`;
}

  function clearTimers() {
    if (reconnectTimeout) { clearTimeout(reconnectTimeout); reconnectTimeout = null; }
    if (heartbeatInterval) { clearInterval(heartbeatInterval); heartbeatInterval = null; }
  }

  function startHeartbeat() {
    clearInterval(heartbeatInterval);
    heartbeatInterval = setInterval(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping', ts: Date.now() }));
      }
    }, 30000); // 30s heartbeat from client keeps proxies happy
  }

  // Graceful close
  function closeSocket() {
    if (ws) {
      try { ws.close(1000, 'client navigating away'); } catch (_) {}
      ws = null;
    }
    clearTimers();
  }

  // Connect to a chat room by PIN
  function connectToPin(pin) {
    if (!pin) return;

    if (ws && ws.readyState === WebSocket.OPEN && currentPin === pin) {
      append(`Already connected to room ${pin}`, 'system');
      return;
    }

    closeSocket();
    currentPin = pin;

    const url = getWsUrl(pin);
    console.log(`🌐 Connecting to: ${url}`);
    ws = new WebSocket(url);

    ws.addEventListener('open', () => {
      retryCount = 0;
      append(`✅ Connected to room ${pin}`, 'system');
      if (title) title.textContent = `Room ${pin}`;
      startHeartbeat();
    });

    ws.addEventListener('message', (ev) => {
      // Try to parse JSON; fallback to raw text
      try {
        const data = JSON.parse(ev.data);
        switch (data.type) {
          case 'pong':
            // Ignore heartbeat acks
            return;
          case 'system':
            append(data.msg || ev.data, 'system');
            return;
          case 'chat':
            append(`${data.user || 'anon'}: ${data.msg ?? ''}`);
            return;
          default:
            append(ev.data);
        }
      } catch {
        append(ev.data);
      }
    });

    ws.addEventListener('close', (e) => {
      clearInterval(heartbeatInterval);
      append(`⚠️ Disconnected from room ${pin} (code ${e.code})`, 'system');
      console.log(`WebSocket closed: code=${e.code}, reason=${e.reason}`);
      ws = null;

      // Reconnect on abnormal closure
      if (retryCount < maxRetries && e.code !== 1000) {
        retryCount++;
        const backoffMs = Math.min(3000 * retryCount, 15000);
        append(`Reconnecting... (attempt ${retryCount}/${maxRetries})`, 'system');
        reconnectTimeout = setTimeout(() => connectToPin(pin), backoffMs);
      } else if (retryCount >= maxRetries) {
        append('❌ Max reconnection attempts reached.', 'system');
      }
    });

    ws.addEventListener('error', (err) => {
      console.error('WebSocket error:', err);
      append('❌ WebSocket error — check console.', 'system');
    });
  }

  // Button Events
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

    // Send structured JSON for future extensibility
    ws.send(JSON.stringify({ type: 'chat', user, msg }));
    messageInput.value = '';
  });

  messageInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') sendBtn.click();
  });

  // Clean up if the page unloads
  window.addEventListener('beforeunload', closeSocket);
});