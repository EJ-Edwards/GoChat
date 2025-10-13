document.addEventListener('DOMContentLoaded', () => {
  const usernameInput = document.getElementById('username');
  const messageInput = document.getElementById('message');
  const sendBtn = document.getElementById('send-btn');
  const messages = document.getElementById('all-messages');

  // Add PIN controls if not present
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
    document.querySelector('#chatbox-container').insertBefore(pinControls, messages);
  }

  const pinInput = document.getElementById('pin-input');
  const joinBtn = document.getElementById('join-pin');
  const createBtn = document.getElementById('create-pin');

  let ws = null;
  let currentPin = null;

  function append(text, type = 'normal') {
    const d = document.createElement('div');
    d.textContent = text;
    if (type === 'system') d.style.fontStyle = 'italic';
    messages.appendChild(d);
    messages.scrollTop = messages.scrollHeight;
  }

  function connectToPin(pin) {
    if (!pin) return;

    if (ws) ws.close(); // close old connection
    currentPin = pin;

    ws = new WebSocket('ws://localhost:5500/room?pin=' + encodeURIComponent(pin));

    ws.addEventListener('open', () => append('Connected to room ' + pin, 'system'));

    ws.addEventListener('message', (ev) => append(ev.data));

    ws.addEventListener('close', () => {
      append('Disconnected from room ' + pin, 'system');
      // optional: auto-reconnect after 2 seconds
      setTimeout(() => {
        append('Reconnecting...', 'system');
        connectToPin(pin);
      }, 2000);
    });

    ws.addEventListener('error', (err) => {
      console.error(err);
      append('WebSocket error', 'system');
    });
  }

  joinBtn.addEventListener('click', () => {
    const p = pinInput.value.trim();
    if (!p) { append('Enter a PIN to join', 'system'); return; }
    connectToPin(p);
  });

  createBtn.addEventListener('click', () => {
    const p = Math.floor(1000 + Math.random() * 9000).toString();
    pinInput.value = p;
    append('Created room ' + p, 'system');
    connectToPin(p);
  });

  sendBtn.addEventListener('click', () => {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      append('Not connected to a room', 'system');
      return;
    }

    const user = usernameInput.value.trim() || 'anon';
    const msg = messageInput.value.trim();
    if (!msg) return;

    ws.send(user + ': ' + msg);
    messageInput.value = '';
  });

  // Optional: send message on Enter key
  messageInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') sendBtn.click();
  });
});
