const ws = new WebSocket(`ws://${location.host}/ws`)

ws.addEventListener("open", (e)=>{
    console.log("connected to ws for hot reload")
})

ws.addEventListener("message", (e)=>{
    if (e.data == "reload") {
        console.log("recieved command to reload")
        location.reload()
    } else if (e.data == "ping") {
        ws.send("pong")
    }
   
})