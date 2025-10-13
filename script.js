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
const database = firebase.database().ref()
const messagesDiv = document.getElementById("all-messages");
const username = document.getElementById("username");
const message = document.getElementById("message");
const submitButton = document.getElementById("send-btn");
const email = document.getElementById("email");
const time = document.getElementById("time");
const date = document.getElementById("date");


submitButton.onclick = updateDB;

function updateDB(event) {
  event.preventDefault();

  if (username.value == "") {
    alert("Please insert a username!");
    return;
  }

  if (message.value == "") {
    alert("Please insert a message!");
    return;
  }

  if (email.value == "") {
    alert("Please insert an email!");
    return;
  }

  let datesString = '' + date.getMonth() + "/" + getDay() + "/" + getFullYear();
  let timeString = '' + date.getHours(); + '/' + date.getMinutes(); + '/' + date.getSeconds();
  const date = new Date();

  let data = {
    "username": username.value,
    "message": message.value,
    "email": email.value,
    "time": timeString,
    "date": datesString

  };

  console.log(data);
  database.push(data);

  message.value = "";
  username.value = "";
  email.value = "";
}

database.on('child_added', addMessageToBoard);


function addMessageToBoard(rowData) {
  let data = rowData.val();
  console.log(data)

  let singleMessage = makeSingleMessageHTML(data.username, data.message, data.email, data.date, data.time);
  messagesDiv.appendChild(singleMessage);


}
function makeSingleMessageHTML(usernameTxt, messageTxt, emailTxt, dateTxt, timeTxt) {
  let parentDiv = document.createElement("div");
  parentDiv.className = 'single-message';

  let usernameP = document.createElement("p");
  usernameP.textContent = usernameTxt;
  usernameP.className = 'single-message-username';

  let emailP = document.createElement("p");
  emailP.textContent = emailTxt;
  emailP.className = 'single-message-email';

  let messageP = document.createElement("p");
  messageP.textContent = messageTxt;

  parentDiv.append(usernameP, emailP, messageP);


  let date = document.createElement("p");
  date.innerText = dateTxt;
  parentDiv.append(date);

  let time = document.createElement("p");
  time.innerText = timeTxt;
  parentDiv.append(time);

  return parentDiv;
}




