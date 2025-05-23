const wait = Math.floor(Math.random()*Number.parseInt(process.argv[2]))
const start = Date.now()/1000
setInterval(()=>{
    console.log(Math.floor(wait - Date.now()/1000+start))

},1000)
setTimeout(process.exit, wait*1000)