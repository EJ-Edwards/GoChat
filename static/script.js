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




