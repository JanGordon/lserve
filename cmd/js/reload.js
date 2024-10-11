(() => {
  // reload.ts
  var ws = new WebSocket(`ws://${location.host}/ws`);
  ws.addEventListener("open", (e) => {
    console.log("connected to ws for hot reload");
  });
  ws.addEventListener("message", (e) => {
    console.log("recieved command to reload");
    location.reload();
  });
})();
