document.addEventListener('DOMContentLoaded', () => {
  const usernameInput = document.getElementById('username');
  const messageInput = document.getElementById('message');
  const emailInput = document.getElementById('email');
  const sendBtn = document.getElementById('send-btn');
  const messages = document.getElementById('all-messages');

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
    document.querySelector('#chatbox-container').insertBefore(pinControls, document.getElementById('all-messages'));
  }

  const pinInput = document.getElementById('pin-input');
  const joinBtn = document.getElementById('join-pin');
  const createBtn = document.getElementById('create-pin');

  function append(text) {
    const d = document.createElement('div');
    d.textContent = text;
    messages.appendChild(d);
    messages.scrollTop = messages.scrollHeight;
  }

  let ws = null;

  function connectToPin(pin) {
    if (!pin) return;
    if (ws) ws.close();
    ws = new WebSocket('ws://' + window.location.host + '/room?pin=' + encodeURIComponent(pin));

    ws.addEventListener('open', () => append('Connected to room ' + pin));
    ws.addEventListener('message', (ev) => append(ev.data));
    ws.addEventListener('close', () => append('Disconnected'));
    ws.addEventListener('error', (err) => { console.error(err); append('WebSocket error'); });
  }

  joinBtn.addEventListener('click', () => {
    const p = pinInput.value.trim();
    if (!p) { append('Enter a PIN to join'); return; }
    connectToPin(p);
  });

  createBtn.addEventListener('click', () => {
    const p = Math.floor(1000 + Math.random() * 9000).toString();
    pinInput.value = p;
    append('Created room ' + p);
    connectToPin(p);
  });

  sendBtn.addEventListener('click', () => {
    if (!ws || ws.readyState !== WebSocket.OPEN) { append('Not connected to a room'); return; }
    const user = usernameInput.value.trim() || 'anon';
    const msg = messageInput.value.trim();
    if (!msg) return;
    ws.send(user + ': ' + msg);
    messageInput.value = '';
  });
});